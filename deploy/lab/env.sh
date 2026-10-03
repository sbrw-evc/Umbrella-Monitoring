#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

log() { printf '\033[36m==>\033[0m %s\n' "$*"; }
rnd() { openssl rand -hex "${1:-16}"; }
pw() { printf 'Lab%s7' "$(openssl rand -hex 12)"; }

umask 077
[ -f .env ] || { log "Writing .env with random secrets"; : > .env; }
add_env() {
  grep -q "^$1=" .env && return 0
  printf '%s=%s\n' "$1" "$2" >> .env
  log "  .env: $1"
}
HOST_IP=${PUBLIC_HOST:-$(hostname -I 2>/dev/null | awk '{print $1}')}
add_env PUBLIC_HOST "${HOST_IP:-localhost}"
add_env LAB_HOST "${LAB_HOST:-lab-host-1}"
add_env UMBRELLA_PORT "${UMBRELLA_PORT:-8080}"
add_env ZABBIX_WEB_PORT "${ZABBIX_WEB_PORT:-8081}"
add_env GRAFANA_PORT "${GRAFANA_PORT:-3000}"
add_env PROMETHEUS_PORT "${PROMETHEUS_PORT:-9090}"
add_env OSD_PORT "${OSD_PORT:-5601}"
add_env NETBOX_PORT "${NETBOX_PORT:-8000}"
add_env OPENBAO_PORT "${OPENBAO_PORT:-8200}"
add_env TZ "${TZ:-Europe/Moscow}"
add_env PG_SLOW_MS "${PG_SLOW_MS:-20}"
add_env UMBRELLA_ADMIN_USER "${UMBRELLA_ADMIN_USER:-admin}"
add_env UMBRELLA_ADMIN_PASSWORD "$(pw)"
add_env LAB_OWNER_PASSWORD "$(pw)"
add_env UMBRELLA_GRAFANA_TOKEN "umb_$(rnd 24)"
add_env UMBRELLA_METRICS_TOKEN "$(rnd)"
add_env PROMETHEUS_WEBHOOK_TOKEN "$(rnd)"
add_env UMBRELLA_DB_PASSWORD "$(rnd)"
add_env ZABBIX_DB_PASSWORD "$(rnd)"
add_env ZABBIX_ADMIN_PASSWORD "$(rnd)"
add_env GRAFANA_ADMIN_PASSWORD "$(rnd)"
add_env NETBOX_DB_PASSWORD "$(rnd)"
add_env NETBOX_REDIS_PASSWORD "$(rnd)"
add_env NETBOX_SECRET_KEY "$(rnd 32)"
add_env NETBOX_ADMIN_PASSWORD "$(pw)"
add_env NETBOX_API_TOKEN "$(rnd 20)"
add_env UMBRELLA_PD_ROUTING_KEY "${UMBRELLA_PD_ROUTING_KEY:-}"
add_env UMBRELLA_PD_API_TOKEN "${UMBRELLA_PD_API_TOKEN:-}"
add_env UMBRELLA_PD_REGION "${UMBRELLA_PD_REGION:-us}"
add_env TEAMS_WEBHOOK_URL "${TEAMS_WEBHOOK_URL:-}"
add_env ZOOM_WEBHOOK_URL "${ZOOM_WEBHOOK_URL:-}"
add_env ZOOM_VERIFICATION_TOKEN "${ZOOM_VERIFICATION_TOKEN:-}"
chmod 600 .env
