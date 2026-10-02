#!/usr/bin/env bash
# One-command lab install on a clean Debian/Ubuntu server, run as root:
#
#   curl -fsSL https://raw.githubusercontent.com/sbrw-evc/Umbrella-Monitoring/feature/mvp-app/deploy/zabbix-lab/install.sh | bash
#
# or from a checkout: sudo deploy/zabbix-lab/install.sh
#
# Installs Docker, fetches the repository to /opt/umbrella-monitoring, writes
# .env with random secrets, starts Umbrella, Zabbix 7.0 (server, web, agent 2),
# Prometheus + Alertmanager + node-exporter, OpenSearch + Dashboards, Grafana
# and the notification sink, connects the sources to Umbrella (configure.sh)
# and runs the MVP test (smoke-test.sh --all). Running it again updates the
# code and restarts; the secrets in .env are kept, missing ones are added.
#
# Settings (environment): UMB_REPO, UMB_BRANCH, UMB_DIR, PUBLIC_HOST,
# UMBRELLA_PORT, ZABBIX_WEB_PORT, GRAFANA_PORT, PROMETHEUS_PORT, OSD_PORT,
# UMBRELLA_DEMO, UMBRELLA_PD_ROUTING_KEY, SKIP_TEST=1.
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

# Use the checkout this script lives in, otherwise clone or update UMB_DIR.
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
  LAB="$UMB_DIR/deploy/zabbix-lab"
fi
cd "$LAB"

# OpenSearch wants vm.max_map_count >= 262144 (kept across reboots).
if [ "$(sysctl -n vm.max_map_count 2>/dev/null || echo 0)" -lt 262144 ]; then
  log "sysctl vm.max_map_count=262144 (OpenSearch)"
  sysctl -q -w vm.max_map_count=262144 || log "  could not set it; OpenSearch may refuse to start"
fi
if [ -d /etc/sysctl.d ] && [ ! -f /etc/sysctl.d/99-umbrella-opensearch.conf ]; then
  echo 'vm.max_map_count = 262144' > /etc/sysctl.d/99-umbrella-opensearch.conf
fi

rnd() { openssl rand -hex 16; }
# Umbrella password policy: 10+ characters, letters and digits.
pw() { printf 'Lab%s7' "$(openssl rand -hex 12)"; }
umask 077
if [ ! -f .env ]; then
  log "Writing .env with random secrets"
  HOST_IP=${PUBLIC_HOST:-$(hostname -I 2>/dev/null | awk '{print $1}')}
  cat > .env <<ENV
PUBLIC_HOST=${HOST_IP:-localhost}
UMBRELLA_PORT=${UMBRELLA_PORT:-8080}
ZABBIX_WEB_PORT=${ZABBIX_WEB_PORT:-8081}
UMBRELLA_DEMO=${UMBRELLA_DEMO:-false}
UMBRELLA_PD_ROUTING_KEY=${UMBRELLA_PD_ROUTING_KEY:-}
ZABBIX_DB_PASSWORD=$(rnd)
ZABBIX_ADMIN_PASSWORD=$(rnd)
ZABBIX_WEBHOOK_TOKEN=$(rnd)
SMOKE_WEBHOOK_TOKEN=$(rnd)
TZ=${TZ:-Europe/Moscow}
ENV
fi
# Settings added after the first version of the lab: appended when missing,
# so an existing .env keeps its passwords.
add_env() { grep -q "^$1=" .env || { printf '%s=%s\n' "$1" "$2" >> .env; log "  .env: added $1"; }; }
add_env GRAFANA_PORT "${GRAFANA_PORT:-3000}"
add_env PROMETHEUS_PORT "${PROMETHEUS_PORT:-9090}"
add_env OSD_PORT "${OSD_PORT:-5601}"
add_env UMBRELLA_ADMIN_USER "${UMBRELLA_ADMIN_USER:-admin}"
add_env UMBRELLA_ADMIN_PASSWORD "$(pw)"
add_env LAB_OWNER_PASSWORD "$(pw)"
add_env GRAFANA_ADMIN_PASSWORD "$(rnd)"
add_env UMBRELLA_GRAFANA_TOKEN "umb_$(openssl rand -hex 24)"
add_env UMBRELLA_METRICS_TOKEN "$(rnd)"
add_env PROMETHEUS_WEBHOOK_TOKEN "$(rnd)"
add_env LAB_ZOOM_TOKEN "$(rnd)"
umask 022
chmod 600 .env
set -a; . ./.env; set +a

log "Building and starting containers (first build takes a few minutes)"
docker compose up -d --build
docker compose ps

log "Connecting the lab sources to Umbrella"
./configure.sh

TEST=0
if [ "${SKIP_TEST:-0}" != 1 ]; then
  log "Testing MVP functions"
  ./smoke-test.sh --all || TEST=$?
fi

cat <<MSG

Umbrella:    http://$PUBLIC_HOST:$UMBRELLA_PORT   login ${UMBRELLA_ADMIN_USER:-admin}, password UMBRELLA_ADMIN_PASSWORD
             owner of lab-shop: lab-owner, password LAB_OWNER_PASSWORD
Grafana:     http://$PUBLIC_HOST:$GRAFANA_PORT   login admin, password GRAFANA_ADMIN_PASSWORD
Zabbix:      http://$PUBLIC_HOST:$ZABBIX_WEB_PORT   login Admin, password ZABBIX_ADMIN_PASSWORD
Prometheus:  http://$PUBLIC_HOST:$PROMETHEUS_PORT   (no login)
OpenSearch Dashboards:  http://$PUBLIC_HOST:$OSD_PORT   (no login)
All passwords and tokens are in $LAB/.env (readable by root only).
Re-run the test:       $LAB/smoke-test.sh --all
After a restart of the umbrella container (data is in memory):  $LAB/configure.sh
Prometheus and OpenSearch Dashboards have no login: open the ports only to your own addresses.
MSG
exit $TEST
