#!/usr/bin/env bash
# One-command lab install on a clean Debian/Ubuntu server, run as root:
#
#   curl -fsSL https://raw.githubusercontent.com/sbrw-evc/Umbrella-Monitoring/feature/mvp-app/deploy/zabbix-lab/install.sh | bash
#
# or from a checkout: sudo deploy/zabbix-lab/install.sh
#
# Installs Docker, fetches the repository to /opt/umbrella-monitoring, writes
# .env with random secrets, starts Umbrella + Zabbix 7.0 (server, web, agent 2),
# connects Zabbix to Umbrella (configure.sh) and runs the MVP test
# (smoke-test.sh --zabbix). Running it again updates the code and restarts.
#
# Settings (environment): UMB_REPO, UMB_BRANCH, UMB_DIR, PUBLIC_HOST,
# UMBRELLA_PORT, ZABBIX_WEB_PORT, UMBRELLA_DEMO, UMBRELLA_PD_ROUTING_KEY.
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

if [ ! -f .env ]; then
  log "Writing .env with random secrets"
  rnd() { openssl rand -hex 16; }
  HOST_IP=${PUBLIC_HOST:-$(hostname -I 2>/dev/null | awk '{print $1}')}
  umask 077
  cat > .env <<ENV
PUBLIC_HOST=${HOST_IP:-localhost}
UMBRELLA_PORT=${UMBRELLA_PORT:-8080}
ZABBIX_WEB_PORT=${ZABBIX_WEB_PORT:-8081}
UMBRELLA_DEMO=${UMBRELLA_DEMO:-true}
UMBRELLA_PD_ROUTING_KEY=${UMBRELLA_PD_ROUTING_KEY:-}
ZABBIX_DB_PASSWORD=$(rnd)
ZABBIX_ADMIN_PASSWORD=$(rnd)
ZABBIX_WEBHOOK_TOKEN=$(rnd)
SMOKE_WEBHOOK_TOKEN=$(rnd)
TZ=${TZ:-Europe/Moscow}
ENV
  umask 022
fi
chmod 600 .env
set -a; . ./.env; set +a

log "Building and starting containers (first build takes a few minutes)"
docker compose up -d --build
docker compose ps

log "Connecting Zabbix to Umbrella"
./configure.sh

log "Testing MVP functions"
TEST=0
./smoke-test.sh --zabbix || TEST=$?

cat <<MSG

Umbrella:  http://$PUBLIC_HOST:$UMBRELLA_PORT
Zabbix:    http://$PUBLIC_HOST:$ZABBIX_WEB_PORT   login Admin, password: ZABBIX_ADMIN_PASSWORD in $LAB/.env
Re-run the test:       $LAB/smoke-test.sh --zabbix
After a restart of the umbrella container (data is in memory):  $LAB/configure.sh
The lab API has no login: open the ports only to your own addresses.
MSG
exit $TEST
