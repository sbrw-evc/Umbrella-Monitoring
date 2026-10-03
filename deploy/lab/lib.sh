# shellcheck shell=bash
UMB_URL=${UMB_URL:-http://localhost:${UMBRELLA_PORT:-8080}}
ZBX_URL=${ZBX_URL:-http://localhost:${ZABBIX_WEB_PORT:-8081}}
PROM_URL=${PROM_URL:-http://localhost:${PROMETHEUS_PORT:-9090}}
GRAFANA_URL=${GRAFANA_URL:-http://localhost:${GRAFANA_PORT:-3000}}
OSD_URL=${OSD_URL:-http://localhost:${OSD_PORT:-5601}}
NETBOX_URL=${NETBOX_URL:-http://localhost:${NETBOX_PORT:-8000}}
LAB_HOST=${LAB_HOST:-lab-host-1}
LAB_TEAM=${LAB_TEAM:-monitoring}
UMB_TOKEN=${UMB_TOKEN:-}
UMB_STATUS_FILE=$(mktemp)
trap 'rm -f "$UMB_STATUS_FILE"' EXIT
umb_status() { cat "$UMB_STATUS_FILE"; }

log() { printf '\033[36m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[33mWARN:\033[0m %s\n' "$*" >&2; }
die() { printf '\033[31mERROR:\033[0m %s\n' "$*" >&2; exit 1; }

http() {
  local out m=$1 u=$2 body=${3:-}
  shift 2; [ $# -gt 0 ] && shift
  out=$(curl -sS -o /dev/stdout -w '\n%{http_code}' -X "$m" "$u" -H 'Content-Type: application/json' \
    ${body:+--data-binary "$body"} "$@") || { printf '0' >"$UMB_STATUS_FILE"; return 1; }
  printf '%s' "${out##*$'\n'}" >"$UMB_STATUS_FILE"
  printf '%s' "${out%$'\n'*}"
}

umb() { http "$1" "$UMB_URL$2" "${3:-}" ${UMB_TOKEN:+-H "Authorization: Bearer $UMB_TOKEN"}; }
umb_as() { local t=$1; shift; http "$1" "$UMB_URL$2" "${3:-}" ${t:+-H "Authorization: Bearer $t"}; }

umb_token() {
  http POST "$UMB_URL/api/auth/token" "$(jq -nc --arg u "$1" --arg p "$2" '{username:$u,password:$p}')" | jq -r '.token // empty'
}

umb_login() {
  local resp
  resp=$(http POST "$UMB_URL/api/auth/token" "$(jq -nc --arg u "$1" --arg p "$2" '{username:$u,password:$p}')") || return 1
  [ "$(umb_status)" = 200 ] || return 1
  [ "$(jq -r .must_change_password <<<"$resp")" = true ] &&
    die "Umbrella asks $1 to change the password: change it in the web UI and in OpenBao (umbrella/bootstrap) and put it into UMBRELLA_ADMIN_PASSWORD in .env"
  UMB_TOKEN=$(jq -r .token <<<"$resp")
  [ -n "$UMB_TOKEN" ] && [ "$UMB_TOKEN" != null ]
}

wait_http() {
  local i
  for ((i = 0; i < $2; i += 3)); do
    curl -fsS -o /dev/null "$1" 2>/dev/null && return 0
    sleep 3
  done
  return 1
}

os_api() {
  http POST "$OSD_URL/api/console/proxy?path=$(jq -rn --arg v "$2" '$v|@uri')&method=$1" "${3:-}" -H 'osd-xsrf: true'
}
osd() { http "$1" "$OSD_URL$2" "${3:-}" -H 'osd-xsrf: true'; }

grafana() { http "$1" "$GRAFANA_URL$2" "${3:-}" -u "admin:${GRAFANA_ADMIN_PASSWORD:-}"; }

nb() { http "$1" "$NETBOX_URL$2" "${3:-}" -H "Authorization: Token ${NETBOX_API_TOKEN:-}" -H 'Accept: application/json'; }

nb_ensure() {
  local path=$1 query=$2 body=$3 id
  id=$(nb GET "$path?$query" | jq -r '.results[0].id // empty')
  if [ -z "$id" ]; then
    id=$(nb POST "$path" "$body" | jq -r '.id // empty')
    [ -n "$id" ] || die "NetBox $path: create answered $(umb_status)"
  else
    nb PATCH "$path$id/" "$body" >/dev/null
  fi
  printf '%s' "$id"
}

zbx() {
  local resp
  resp=$(curl -sS -X POST "$ZBX_URL/api_jsonrpc.php" -H 'Content-Type: application/json-rpc' \
    ${ZBX_TOKEN:+-H "Authorization: Bearer $ZBX_TOKEN"} \
    --data-binary "$(jq -nc --arg m "$1" --argjson p "$2" '{jsonrpc:"2.0",method:$m,params:$p,id:1}')") || return 1
  if [ "$(jq -r 'has("error")' <<<"$resp")" = true ]; then
    printf 'Zabbix %s: %s\n' "$1" "$(jq -c .error <<<"$resp")" >&2
    return 1
  fi
  jq -c .result <<<"$resp"
}

zbx_login() {
  ZBX_TOKEN=
  ZBX_TOKEN=$(zbx user.login "$(jq -nc --arg u "$1" --arg p "$2" '{username:$u,password:$p}')" 2>/dev/null | jq -r .) || return 1
  [ -n "$ZBX_TOKEN" ] && [ "$ZBX_TOKEN" != null ]
}

ci_id() { umb GET "/api/cis?q=$(jq -rn --arg v "$1" '$v|@uri')" | jq -r --arg n "$1" '[.items[] | select(.name == $n)][0].id // empty'; }
user_id() { umb GET /api/users | jq -r --arg n "$1" '[.items[] | select(.username == $n)][0].id // empty'; }

ensure_ci() {
  local id
  id=$(ci_id "$1")
  if [ -z "$id" ]; then
    id=$(umb POST /api/cis "$(jq -nc --arg n "$1" --arg t "$2" --arg team "$3" --arg d "${4:-}" '{name:$n,type:$t,team:$team,description:$d}')" | jq -r '.id // empty')
    [ -n "$id" ] || die "CI $1: create answered $(umb_status)"
  fi
  printf '%s' "$id"
}

ensure_relation() {
  umb POST /api/relations "$(jq -nc --arg f "$1" --arg t "$2" --arg ty "${3:-depends_on}" '{from:$f,to:$t,type:$ty}')" >/dev/null || true
}

ensure_user() {
  local id
  id=$(user_id "$1")
  if [ -z "$id" ]; then
    id=$(umb POST /api/users "$2" | jq -r '.id // empty')
    [ "$(umb_status)" = 201 ] || die "user $1: create answered $(umb_status)"
  else
    umb PUT "/api/users/$id" "$(jq -c 'del(.password, .username, .must_change_password, .service)' <<<"$2")" >/dev/null
    [ "$(umb_status)" = 200 ] || die "user $1: update answered $(umb_status)"
  fi
  printf '%s' "$id"
}

ensure_channel() {
  local id
  id=$(umb GET /api/channels | jq -r --arg n "$1" '[.items[] | select(.name == $n)][0].id // empty')
  if [ -z "$id" ]; then
    id=$(umb POST /api/channels "$2" | jq -r '.id // empty')
    [ "$(umb_status)" = 201 ] || die "channel $1: create answered $(umb_status)"
  else
    umb PUT "/api/channels/$id" "$2" >/dev/null
    [ "$(umb_status)" = 200 ] || die "channel $1: update answered $(umb_status)"
  fi
  printf '%s' "$id"
}

integration_id() { umb GET /api/integrations | jq -r --arg n "$1" '[.items[] | select(.name == $n)][0].id // empty'; }

ensure_integration() {
  local id resp body=$2
  id=$(integration_id "$1")
  if [ -z "$id" ]; then
    resp=$(umb POST /api/integrations "$body")
    [ "$(umb_status)" = 201 ] || die "integration $1: create answered $(umb_status): $(jq -r '.error // empty' <<<"$resp")"
    id=$(jq -r .id <<<"$resp")
  else
    resp=$(umb PUT "/api/integrations/$id" "$(jq -c 'del(.slug)' <<<"$body")")
    [ "$(umb_status)" = 200 ] || die "integration $1: update answered $(umb_status): $(jq -r '.error // empty' <<<"$resp")"
  fi
  printf '%s' "$id"
}

integration_action() {
  local resp
  resp=$(umb POST "/api/integrations/$1/$2")
  if [ "$(jq -r .ok <<<"$resp")" = true ]; then
    log "  $2: $(jq -r .message <<<"$resp")"
  else
    warn "$1 $2: $(jq -r '.message // .error' <<<"$resp")"
  fi
}

ensure_rule() {
  local id resp
  id=$(umb GET /api/rules | jq -r --arg n "$1" '[.items[] | select(.name == $n)][0].id // empty')
  if [ -z "$id" ]; then
    resp=$(umb POST /api/rules "$2")
    [ "$(umb_status)" = 201 ] || die "rule $1: create answered $(umb_status): $(jq -r '.error // empty' <<<"$resp")"
    id=$(jq -r .id <<<"$resp")
  else
    resp=$(umb PUT "/api/rules/$id" "$2")
    [ "$(umb_status)" = 200 ] || die "rule $1: update answered $(umb_status): $(jq -r '.error // empty' <<<"$resp")"
  fi
  printf '%s' "$id"
}

compose() { docker compose --project-directory "$LAB_DIR" "$@"; }
