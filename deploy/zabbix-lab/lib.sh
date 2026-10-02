# Shared helpers for configure.sh and smoke-test.sh. Needs curl and jq.

UMB_URL=${UMB_URL:-http://localhost:${UMBRELLA_PORT:-8080}}
ZBX_URL=${ZBX_URL:-http://localhost:${ZABBIX_WEB_PORT:-8081}}
PROM_URL=${PROM_URL:-http://localhost:${PROMETHEUS_PORT:-9090}}
GRAFANA_URL=${GRAFANA_URL:-http://localhost:${GRAFANA_PORT:-3000}}
OSD_URL=${OSD_URL:-http://localhost:${OSD_PORT:-5601}}
# Bearer token of the signed-in Umbrella user (umb_login) or an API token.
UMB_TOKEN=${UMB_TOKEN:-}
# The last HTTP code, kept in a file so it survives $(umb ...) subshells.
UMB_STATUS_FILE=$(mktemp)
trap 'rm -f "$UMB_STATUS_FILE"' EXIT
umb_status() { cat "$UMB_STATUS_FILE"; }

log() { printf '\033[36m==>\033[0m %s\n' "$*"; }
die() { printf '\033[31mERROR:\033[0m %s\n' "$*" >&2; exit 1; }

# http METHOD URL [JSON] [curl args...] prints the body; umb_status prints the HTTP code.
http() {
  local out m=$1 u=$2 body=${3:-}
  shift 2; [ $# -gt 0 ] && shift
  out=$(curl -sS -o /dev/stdout -w '\n%{http_code}' -X "$m" "$u" -H 'Content-Type: application/json' \
    ${body:+--data-binary "$body"} "$@") || { printf '0' >"$UMB_STATUS_FILE"; return 1; }
  printf '%s' "${out##*$'\n'}" >"$UMB_STATUS_FILE"
  printf '%s' "${out%$'\n'*}"
}

# umb METHOD PATH [JSON] calls the Umbrella API as $UMB_TOKEN.
umb() { http "$1" "$UMB_URL$2" "${3:-}" ${UMB_TOKEN:+-H "Authorization: Bearer $UMB_TOKEN"}; }

# umb_as TOKEN METHOD PATH [JSON] calls the Umbrella API with another token.
umb_as() { local t=$1; shift; http "$1" "$UMB_URL$2" "${3:-}" ${t:+-H "Authorization: Bearer $t"}; }

# umb_token USER PASSWORD prints a bearer session token (POST /api/auth/token).
umb_token() {
  http POST "$UMB_URL/api/auth/token" "$(jq -nc --arg u "$1" --arg p "$2" '{username:$u,password:$p}')" | jq -r '.token // empty'
}

# umb_login USER PASSWORD signs in and keeps the token in UMB_TOKEN.
umb_login() {
  local resp
  resp=$(http POST "$UMB_URL/api/auth/token" "$(jq -nc --arg u "$1" --arg p "$2" '{username:$u,password:$p}')") || return 1
  [ "$(umb_status)" = 200 ] || return 1
  [ "$(jq -r .must_change_password <<<"$resp")" = true ] &&
    die "Umbrella asks $1 to change the password: sign in to the web UI, change it and put it into UMBRELLA_ADMIN_PASSWORD in .env"
  UMB_TOKEN=$(jq -r .token <<<"$resp")
  [ -n "$UMB_TOKEN" ] && [ "$UMB_TOKEN" != null ]
}

# ingest CONNECTOR_ID_OR_SLUG TOKEN JSON prints the body; umb_status prints the HTTP code.
ingest() { http POST "$UMB_URL/api/ingest/$1" "$3" ${2:+-H "X-Umbrella-Token: $2"}; }

wait_http() { # wait_http URL SECONDS
  local i
  for ((i = 0; i < $2; i += 3)); do
    curl -fsS -o /dev/null "$1" 2>/dev/null && return 0
    sleep 3
  done
  return 1
}

# OpenSearch is reachable only inside the lab network; scripts go through the
# OpenSearch Dashboards console proxy instead. os_api METHOD PATH [JSON]
os_api() {
  http POST "$OSD_URL/api/console/proxy?path=$(jq -rn --arg v "$2" '$v|@uri')&method=$1" "${3:-}" -H 'osd-xsrf: true'
}
# osd METHOD PATH [JSON] calls the OpenSearch Dashboards API.
osd() { http "$1" "$OSD_URL$2" "${3:-}" -H 'osd-xsrf: true'; }

# grafana METHOD PATH [JSON] calls the Grafana API as admin (basic auth).
grafana() { http "$1" "$GRAFANA_URL$2" "${3:-}" -u "admin:${GRAFANA_ADMIN_PASSWORD:-}"; }

# Zabbix JSON-RPC. ZBX_TOKEN is set by zbx_login.
zbx() { # zbx METHOD PARAMS_JSON prints .result, fails on .error
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

zbx_login() { # zbx_login USER PASSWORD
  ZBX_TOKEN=
  ZBX_TOKEN=$(zbx user.login "$(jq -nc --arg u "$1" --arg p "$2" '{username:$u,password:$p}')" 2>/dev/null | jq -r .) || return 1
  [ -n "$ZBX_TOKEN" ] && [ "$ZBX_TOKEN" != null ]
}

# connector_id NAME_OR_SLUG prints the id of the Umbrella connector with that slug or name.
connector_id() {
  umb GET /api/connectors | jq -r --arg n "$1" '([.items[] | select(.slug == $n)] + [.items[] | select(.name == $n)])[0].id // empty'
}
ci_id() { umb GET "/api/cis?q=$(jq -rn --arg v "$1" '$v|@uri')" | jq -r --arg n "$1" '[.items[] | select(.name == $n)][0].id // empty'; }
user_id() { umb GET /api/users | jq -r --arg n "$1" '[.items[] | select(.username == $n)][0].id // empty'; }

# ensure_ci NAME TYPE TEAM [PARENT_ID] prints the CI id, creating it when missing.
# PARENT depends on the new CI (business service -> IT service -> host).
ensure_ci() {
  local id
  id=$(ci_id "$1")
  if [ -z "$id" ]; then
    id=$(umb POST /api/cis "$(jq -nc --arg n "$1" --arg t "$2" --arg team "$3" --arg p "${4:-}" \
      '{name:$n,type:$t,team:$team,parent:$p,description:"Umbrella lab",identities:[{kind:"hostname",value:$n}]}')" | jq -r .id)
  fi
  printf '%s' "$id"
}

# ensure_connector NAME SLUG TEAM TEMPLATE GRAPH_JSON prints the id of a
# published, running connector with that graph. It is found by slug, then by
# name (connectors made by older versions of this script had no slug).
ensure_connector() {
  local id
  id=$(connector_id "$2")
  [ -n "$id" ] || id=$(connector_id "$1")
  if [ -z "$id" ]; then
    id=$(umb POST /api/connectors "$(jq -nc --arg n "$1" --arg s "$2" --arg t "$3" --arg tpl "$4" '{name:$n,slug:$s,team:$t,template:$tpl}')" | jq -r .id)
    [ "$(umb_status)" = 201 ] || die "connector $1: create answered $(umb_status)"
  fi
  umb PUT "/api/connectors/$id" "$(jq -nc --arg n "$1" --arg s "$2" --argjson g "$5" '{name:$n,slug:$s,draft:$g}')" >/dev/null
  [ "$(umb_status)" = 200 ] || die "connector $1: update answered $(umb_status)"
  umb POST "/api/connectors/$id/publish" >/dev/null
  [ "$(umb_status)" = 200 ] || die "connector $1: publish answered $(umb_status)"
  umb POST "/api/connectors/$id/start" >/dev/null
  printf '%s' "$id"
}

# ensure_webhook_connector NAME TEAM GRAPH_JSON: a connector without a slug
# (the smoke test makes these).
ensure_webhook_connector() {
  local id
  id=$(connector_id "$1")
  if [ -z "$id" ]; then
    id=$(umb POST /api/connectors "$(jq -nc --arg n "$1" --arg t "$2" '{name:$n,team:$t,template:"webhook-json"}')" | jq -r .id)
  fi
  umb PUT "/api/connectors/$id" "$(jq -nc --argjson g "$3" '{draft:$g}')" >/dev/null
  umb POST "/api/connectors/$id/publish" >/dev/null
  [ "$(umb_status)" = 200 ] || die "connector $1: publish answered $(umb_status)"
  umb POST "/api/connectors/$id/start" >/dev/null
  printf '%s' "$id"
}

# ensure_user USERNAME JSON creates the user or updates name, roles and
# services. The password in JSON is used only on create, so a user who
# changed it keeps the new one.
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

# ensure_channel NAME JSON creates or updates a notification channel.
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

# webhook_graph SECRET_REF SEVERITY_FIELD MAPPING LABELS TITLE CI SIGNAL METHOD STATUS EXTERNAL_ID VALUE [ITEMS_PATH]
webhook_graph() {
  jq -nc --arg secret "$1" --arg sevf "$2" --arg map "$3" --arg labels "$4" --arg title "$5" --arg ci "$6" \
    --arg signal "$7" --arg method "$8" --arg status "$9" --arg ext "${10}" --arg value "${11}" --arg items "${12:-}" '{
    nodes: [
      {id:"n1",kind:"trigger.webhook",x:40,y:140,config:{auth:"token",secret_ref:$secret}},
      {id:"n2",kind:"parse.json",x:280,y:140,config:(if $items == "" then {} else {items:$items} end)},
      {id:"n3",kind:"map.severity",x:520,y:140,config:{field:$sevf,mapping:$map}},
      {id:"n4",kind:"enrich.labels",x:760,y:140,config:{labels:$labels}},
      {id:"n5",kind:"map.event",x:40,y:320,config:{title:$title,ci:$ci,signal:$signal,method:$method,severity:"${_severity}",status:$status,external_id:$ext,value:$value}},
      {id:"n6",kind:"out.event",x:300,y:320,config:{}},
      {id:"n7",kind:"ack.response",x:560,y:320,config:{mode:"http_2xx"}}
    ],
    edges: [
      {id:"e1",source:"n1",target:"n2"},{id:"e2",source:"n2",target:"n3"},{id:"e3",source:"n3",target:"n4"},
      {id:"e4",source:"n4",target:"n5"},{id:"e5",source:"n5",target:"n6"},{id:"e6",source:"n6",target:"n7"}
    ]}'
}

# pull_graph INTERVAL URL METHOD BODY ITEMS SEVERITY_FIELD MAPPING LABELS TITLE CI SIGNAL METHOD STATUS EXTERNAL_ID VALUE
# trigger.schedule -> fetch.http -> parse.json -> map.severity -> enrich.labels -> map.event -> out.event -> ack.response
pull_graph() {
  jq -nc --arg iv "$1" --arg url "$2" --arg hm "$3" --arg body "$4" --arg items "$5" --arg sevf "$6" --arg map "$7" \
    --arg labels "$8" --arg title "$9" --arg ci "${10}" --arg signal "${11}" --arg method "${12}" --arg status "${13}" \
    --arg ext "${14}" --arg value "${15}" '{
    nodes: [
      {id:"n1",kind:"trigger.schedule",x:40,y:140,config:{interval:$iv}},
      {id:"n2",kind:"fetch.http",x:280,y:140,config:{url:$url,method:$hm,body:$body}},
      {id:"n3",kind:"parse.json",x:520,y:140,config:{items:$items}},
      {id:"n4",kind:"map.severity",x:760,y:140,config:{field:$sevf,mapping:$map}},
      {id:"n5",kind:"enrich.labels",x:40,y:320,config:{labels:$labels}},
      {id:"n6",kind:"map.event",x:280,y:320,config:{title:$title,ci:$ci,signal:$signal,method:$method,severity:"${_severity}",status:$status,external_id:$ext,value:$value}},
      {id:"n7",kind:"out.event",x:520,y:320,config:{}},
      {id:"n8",kind:"ack.response",x:760,y:320,config:{mode:"cursor"}}
    ],
    edges: [
      {id:"e1",source:"n1",target:"n2"},{id:"e2",source:"n2",target:"n3"},{id:"e3",source:"n3",target:"n4"},
      {id:"e4",source:"n4",target:"n5"},{id:"e5",source:"n5",target:"n6"},{id:"e6",source:"n6",target:"n7"},
      {id:"e7",source:"n7",target:"n8"}
    ]}'
}
