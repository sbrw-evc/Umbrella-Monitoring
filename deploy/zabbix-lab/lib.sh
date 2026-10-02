# Shared helpers for configure.sh and smoke-test.sh. Needs curl and jq.

UMB_URL=${UMB_URL:-http://localhost:${UMBRELLA_PORT:-8080}}
ZBX_URL=${ZBX_URL:-http://localhost:${ZABBIX_WEB_PORT:-8081}}
UMB_USER=${UMB_USER:-lab-setup}
# The last HTTP code, kept in a file so it survives $(umb ...) subshells.
UMB_STATUS_FILE=$(mktemp)
trap 'rm -f "$UMB_STATUS_FILE"' EXIT
umb_status() { cat "$UMB_STATUS_FILE"; }

log() { printf '\033[36m==>\033[0m %s\n' "$*"; }
die() { printf '\033[31mERROR:\033[0m %s\n' "$*" >&2; exit 1; }

# umb METHOD PATH [JSON] prints the body; umb_status prints the HTTP code.
umb() {
  local out
  out=$(curl -sS -o /dev/stdout -w '\n%{http_code}' -X "$1" "$UMB_URL$2" \
    -H 'Content-Type: application/json' -H "X-Umbrella-User: $UMB_USER" ${3:+--data-binary "$3"}) || return 1
  printf '%s' "${out##*$'\n'}" >"$UMB_STATUS_FILE"
  printf '%s' "${out%$'\n'*}"
}

# ingest CONNECTOR_ID TOKEN JSON prints the body; umb_status prints the HTTP code.
ingest() {
  local out
  out=$(curl -sS -o /dev/stdout -w '\n%{http_code}' -X POST "$UMB_URL/api/ingest/$1" \
    -H 'Content-Type: application/json' ${2:+-H "X-Umbrella-Token: $2"} --data-binary "$3") || return 1
  printf '%s' "${out##*$'\n'}" >"$UMB_STATUS_FILE"
  printf '%s' "${out%$'\n'*}"
}

wait_http() { # wait_http URL SECONDS
  local i
  for ((i = 0; i < $2; i += 3)); do
    curl -fsS -o /dev/null "$1" 2>/dev/null && return 0
    sleep 3
  done
  return 1
}

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

# connector_id NAME prints the id of the Umbrella connector with that name.
connector_id() { umb GET /api/connectors | jq -r --arg n "$1" '[.items[] | select(.name == $n)][0].id // empty'; }
ci_id() { umb GET "/api/cis?q=$(jq -rn --arg v "$1" '$v|@uri')" | jq -r --arg n "$1" '[.items[] | select(.name == $n)][0].id // empty'; }

# ensure_ci NAME TYPE TEAM [PARENT_ID] prints the CI id, creating it when missing.
ensure_ci() {
  local id
  id=$(ci_id "$1")
  if [ -z "$id" ]; then
    id=$(umb POST /api/cis "$(jq -nc --arg n "$1" --arg t "$2" --arg team "$3" --arg p "${4:-}" \
      '{name:$n,type:$t,team:$team,parent:$p,description:"Umbrella lab",identities:[{kind:"hostname",value:$n}]}')" | jq -r .id)
  fi
  printf '%s' "$id"
}

# ensure_webhook_connector NAME TEAM GRAPH_JSON prints the id of a published,
# running connector with that graph.
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

# webhook_graph SECRET_REF SEVERITY_FIELD MAPPING LABELS TITLE CI SIGNAL METHOD STATUS EXTERNAL_ID VALUE
webhook_graph() {
  jq -nc --arg secret "$1" --arg sevf "$2" --arg map "$3" --arg labels "$4" --arg title "$5" --arg ci "$6" \
    --arg signal "$7" --arg method "$8" --arg status "$9" --arg ext "${10}" --arg value "${11}" '{
    nodes: [
      {id:"n1",kind:"trigger.webhook",x:40,y:140,config:{auth:"token",secret_ref:$secret}},
      {id:"n2",kind:"parse.json",x:280,y:140,config:{}},
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
