#!/usr/bin/env bash
set -euo pipefail

UMB_REPO=${UMB_REPO:-https://github.com/sbrw-evc/Umbrella-Monitoring.git}
UMB_BRANCH=${UMB_BRANCH:-feature/mvp-app}
UMB_DIR=${UMB_DIR:-/opt/umbrella-monitoring}

log() { printf '\033[36m==>\033[0m %s\n' "$*"; }
die() { printf '\033[31mERROR:\033[0m %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" = 0 ] || die "run as root (sudo)"

log "Packages: git, curl, jq, openssl"
if command -v apt-get >/dev/null; then
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -qq
  apt-get install -y -qq git curl jq openssl ca-certificates >/dev/null
elif command -v dnf >/dev/null; then
  dnf install -y -q git curl jq openssl ca-certificates
else
  for c in git curl jq openssl; do command -v $c >/dev/null || die "install $c first"; done
fi

if ! command -v docker >/dev/null || ! docker compose version >/dev/null 2>&1; then
  log "Docker Engine + compose plugin (get.docker.com)"
  curl -fsSL https://get.docker.com | sh
fi
systemctl enable --now docker >/dev/null 2>&1 || true

SELF_DIR=$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" 2>/dev/null && pwd || true)
if [ -n "$SELF_DIR" ] && [ -f "$SELF_DIR/docker-compose.yml" ] && [ -d "$SELF_DIR/../../app" ]; then
  LAB="$SELF_DIR"
else
  if [ -d "$UMB_DIR/.git" ]; then
    log "Updating $UMB_DIR ($UMB_BRANCH)"
    git -C "$UMB_DIR" fetch -q origin "$UMB_BRANCH"
    git -C "$UMB_DIR" checkout -q -B "$UMB_BRANCH" FETCH_HEAD
  else
    log "Cloning $UMB_REPO ($UMB_BRANCH) to $UMB_DIR"
    git clone -q --branch "$UMB_BRANCH" "$UMB_REPO" "$UMB_DIR"
  fi
  LAB="$UMB_DIR/deploy/lab"
fi
cd "$LAB"

if [ "$(sysctl -n vm.max_map_count 2>/dev/null || echo 0)" -lt 262144 ]; then
  log "sysctl vm.max_map_count=262144 (OpenSearch)"
  sysctl -q -w vm.max_map_count=262144 || log "  could not set it; OpenSearch may refuse to start"
fi
if [ -d /etc/sysctl.d ] && [ ! -f /etc/sysctl.d/99-umbrella-opensearch.conf ]; then
  echo 'vm.max_map_count = 262144' > /etc/sysctl.d/99-umbrella-opensearch.conf
fi

. ./preflight.sh
apparmor_check "$LAB"

./env.sh

log "OpenBao: start, initialize and unseal"
umask 022
mkdir -p secrets/openbao secrets/umbrella
chmod 700 secrets secrets/openbao
chmod 755 secrets/umbrella
set -a; . ./.env; set +a
docker compose up -d openbao openbao-init
for _ in $(seq 1 100); do
  [ "$(docker inspect -f '{{.State.Health.Status}}' "$(docker compose ps -q openbao-init)" 2>/dev/null)" = healthy ] && break
  sleep 3
done
[ -s secrets/umbrella/secret_id ] || { docker compose logs openbao-init | tail -20; die "OpenBao is not ready"; }
log "  unseal keys and root token: $LAB/secrets/openbao/init.txt (root only)"

log "Building and starting containers (the first build takes several minutes)"
UMBRELLA_VERSION=$(git -C .. describe --tags --always --dirty 2>/dev/null || echo lab)
export UMBRELLA_VERSION
docker compose build --pull umbrella fluentd
docker compose up -d
docker compose ps --format 'table {{.Service}}\t{{.State}}\t{{.Status}}'

log "Connecting the lab to Umbrella"
./configure.sh

TEST=0
if [ "${SKIP_TEST:-0}" != 1 ]; then
  log "Testing the lab"
  ./smoke-test.sh --all || TEST=$?
fi

cat <<MSG

Umbrella:    http://$PUBLIC_HOST:$UMBRELLA_PORT   ${UMBRELLA_ADMIN_USER:-admin} / UMBRELLA_ADMIN_PASSWORD; lab-owner / LAB_OWNER_PASSWORD
Grafana:     http://$PUBLIC_HOST:$GRAFANA_PORT   admin / GRAFANA_ADMIN_PASSWORD
Zabbix:      http://$PUBLIC_HOST:$ZABBIX_WEB_PORT   Admin / ZABBIX_ADMIN_PASSWORD
NetBox:      http://$PUBLIC_HOST:$NETBOX_PORT   admin / NETBOX_ADMIN_PASSWORD
Prometheus:  http://$PUBLIC_HOST:$PROMETHEUS_PORT   (no login)
OpenSearch Dashboards:  http://$PUBLIC_HOST:$OSD_PORT   (no login)
OpenBao:     http://127.0.0.1:$OPENBAO_PORT   (this host only; root token in secrets/openbao/init.txt)
Passwords and tokens: $LAB/.env (root only). Update Umbrella without losing data: $LAB/update.sh
MSG
exit $TEST
