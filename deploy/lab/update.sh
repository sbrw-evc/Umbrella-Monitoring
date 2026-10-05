#!/usr/bin/env bash
set -euo pipefail
LAB_DIR=$(cd "$(dirname "$0")" && pwd)
cd "$LAB_DIR"

usage() {
  cat <<'TXT'
Usage: ./update.sh [options]

Updates Umbrella in place: data (volume umbrella-data), secrets (OpenBao) and
the other lab services are kept. Only the umbrella container is replaced.

  --no-pull     do not fetch new code (use the files as they are)
  --all         also pull newer images and apply changed configs of every service
  --configure   run configure.sh afterwards (idempotent)
  --test        run smoke-test.sh --all afterwards
  --rollback    start the previous Umbrella image again
  -h, --help    this help
TXT
}

main() {
PULL=1 ALL=0 CONFIGURE=0 TEST=0 ROLLBACK=0
for a in "$@"; do
  case "$a" in
    --no-pull) PULL=0 ;;
    --all) ALL=1 ;;
    --configure) CONFIGURE=1 ;;
    --test) TEST=1 ;;
    --rollback) ROLLBACK=1 ;;
    -h | --help) usage; exit 0 ;;
    *) usage; exit 2 ;;
  esac
done

log() { printf '\033[36m==>\033[0m %s\n' "$*"; }
die() { printf '\033[31mERROR:\033[0m %s\n' "$*" >&2; exit 1; }

[ -f .env ] || die "no .env: install the lab first (./install.sh)"
command -v docker >/dev/null || die "docker is not installed"
IMAGE=umbrella-mvp:lab
PREV=umbrella-mvp:lab-prev

running_version() {
  curl -fsSI "http://localhost:${UMBRELLA_PORT:-8080}/healthz" 2>/dev/null | awk -F': ' 'tolower($1)=="x-umbrella-version" {print $2}' | tr -d '\r'
}

wait_healthy() {
  local id state
  for _ in $(seq 1 60); do
    id=$(docker compose ps -q umbrella)
    state=$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$id" 2>/dev/null || true)
    [ "$state" = healthy ] && return 0
    sleep 3
  done
  return 1
}

start_umbrella() { docker compose up -d --no-deps --no-build umbrella; }

. ./preflight.sh
apparmor_check "$LAB_DIR"

set -a; . ./.env; set +a
BEFORE=$(running_version || true)

if [ "$ROLLBACK" = 1 ]; then
  docker image inspect "$PREV" >/dev/null 2>&1 || die "no previous image $PREV"
  log "Rolling back to the previous Umbrella image"
  docker image tag "$PREV" "$IMAGE"
  start_umbrella
  wait_healthy || die "Umbrella is not healthy after rollback: docker compose logs umbrella"
  log "Umbrella ${BEFORE:-?} -> $(running_version)"
  exit 0
fi

ROOT=$(git -C "$LAB_DIR" rev-parse --show-toplevel 2>/dev/null || true)
if [ "$PULL" = 1 ] && [ -n "$ROOT" ]; then
  OLD=$(git -C "$ROOT" rev-parse --short HEAD)
  log "Fetching code ($(git -C "$ROOT" rev-parse --abbrev-ref HEAD))"
  git -C "$ROOT" pull --ff-only -q || die "git pull failed: commit or stash local changes, or run with --no-pull"
  NEW=$(git -C "$ROOT" rev-parse --short HEAD)
  if [ "$OLD" = "$NEW" ]; then log "  already at $NEW"; else log "  $OLD -> $NEW"; git -C "$ROOT" log --oneline "$OLD..$NEW" | head -20; fi
  if [ "$OLD" != "$NEW" ] && git -C "$ROOT" diff --name-only "$OLD" "$NEW" | grep -qE '^deploy/lab/(update|preflight)\.sh$'; then
    log "  update.sh changed: running the new version"
    local args=() a
    for a in "$@"; do [ "$a" = --no-pull ] || args+=("$a"); done
    UPDATE_FROM=$OLD exec "$LAB_DIR/update.sh" --no-pull "${args[@]}"
  fi
elif [ -n "${UPDATE_FROM:-}" ] && [ -n "$ROOT" ]; then
  OLD=$UPDATE_FROM
  NEW=$(git -C "$ROOT" rev-parse --short HEAD)
fi

./env.sh
set -a; . ./.env; set +a

mkdir -p secrets/openbao secrets/umbrella
chmod 700 secrets secrets/openbao
chmod 755 secrets/umbrella
log "OpenBao"
docker compose up -d openbao openbao-init
for _ in $(seq 1 60); do
  [ "$(docker inspect -f '{{.State.Health.Status}}' "$(docker compose ps -q openbao-init)" 2>/dev/null)" = healthy ] && break
  sleep 3
done

UMBRELLA_VERSION=$(git -C "${ROOT:-..}" describe --tags --always --dirty 2>/dev/null || date +%Y%m%d%H%M)
export UMBRELLA_VERSION
if docker image inspect "$IMAGE" >/dev/null 2>&1; then
  docker image tag "$IMAGE" "$PREV"
fi
log "Building Umbrella $UMBRELLA_VERSION (the running container keeps serving)"
docker compose build --pull umbrella

if [ "$ALL" = 1 ]; then
  log "Pulling images and applying configs of every service"
  docker compose pull --ignore-buildable --quiet
  docker compose build fluentd
  docker compose up -d --remove-orphans
  if [ -n "${OLD:-}" ] && [ "$OLD" != "${NEW:-}" ]; then
    CHANGED=$(git -C "$ROOT" diff --name-only "$OLD" "$NEW")
    RESTART=()
    for pair in telegraf:telegraf prometheus:prometheus alertmanager:alertmanager node-textfile:node-exporter; do
      grep -q "^deploy/lab/${pair%%:*}/" <<<"$CHANGED" && RESTART+=("${pair#*:}")
    done
    grep -q '^deploy/grafana/' <<<"$CHANGED" && RESTART+=(grafana)
    if [ ${#RESTART[@]} -gt 0 ]; then
      log "Restarting services with changed mounted configs: ${RESTART[*]}"
      docker compose restart "${RESTART[@]}"
    fi
  fi
else
  log "Replacing the umbrella container"
  start_umbrella
fi

if ! wait_healthy; then
  docker compose logs --tail 30 umbrella >&2 || true
  if docker image inspect "$PREV" >/dev/null 2>&1; then
    log "Umbrella is not healthy: rolling back"
    docker image tag "$PREV" "$IMAGE"
    start_umbrella
    wait_healthy || true
  fi
  die "update failed; the previous version is running again"
fi
log "Umbrella ${BEFORE:-?} -> $(running_version)"

if [ "$CONFIGURE" = 1 ]; then ./configure.sh; fi
if [ "$TEST" = 1 ]; then ./smoke-test.sh --all; fi
docker image prune -f --filter "label=com.docker.compose.project=umbrella-lab" >/dev/null 2>&1 || true
}

main "$@"
exit
