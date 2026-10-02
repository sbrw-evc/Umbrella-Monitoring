#!/usr/bin/env bash
# Checks the Umbrella MVP functions through its API. --zabbix adds the real
# chain Zabbix trigger -> webhook -> Umbrella incident -> recovery; --stack adds
# Prometheus -> Alertmanager -> Umbrella, OpenSearch -> Umbrella, Teams/Zoom
# notifications (the lab sink), Grafana and OpenSearch Dashboards.
#
#   ./smoke-test.sh             # Umbrella only (UMB_URL, default http://localhost:8080)
#   ./smoke-test.sh --zabbix    # plus Zabbix end-to-end (needs configure.sh done)
#   ./smoke-test.sh --stack     # plus Prometheus, OpenSearch, Teams/Zoom, Grafana
#   ./smoke-test.sh --all       # everything (install.sh runs this)
#
# Signs in as UMBRELLA_ADMIN_USER from .env, creates the service account
# smoke-test (roles monitoring + auditor) and works with its API token. Every
# run uses its own CI names, signals and users, so it can be repeated.
set -uo pipefail
cd "$(dirname "$0")"
[ -f .env ] && set -a && . ./.env && set +a
. ./lib.sh

ZABBIX_E2E=${ZABBIX_E2E:-0}
STACK_E2E=${STACK_E2E:-0}
for a in "$@"; do
  case $a in
    --zabbix) ZABBIX_E2E=1 ;;
    --stack) STACK_E2E=1 ;;
    --all) ZABBIX_E2E=1 STACK_E2E=1 ;;
    *) die "unknown option $a (use --zabbix, --stack, --all)" ;;
  esac
done
: "${SMOKE_WEBHOOK_TOKEN:?set in .env (Umbrella reads it as UMB_SECRET_LAB_SMOKE)}"
: "${UMBRELLA_ADMIN_PASSWORD:?set in .env}"
UMB_ADMIN=${UMBRELLA_ADMIN_USER:-admin}
TOKEN=$SMOKE_WEBHOOK_TOKEN
RUN=$(date +%H%M%S)$RANDOM
PASS=0 FAIL=0
FAILED=()
# Passwords of the users this run creates (deleted at the end).
RUN_PW="Smk$(openssl rand -hex 8 2>/dev/null || printf '%s' "$RUN$RANDOM")9"

ok()   { PASS=$((PASS + 1)); printf '  \033[32mPASS\033[0m %s\n' "$1"; }
bad()  { FAIL=$((FAIL + 1)); FAILED+=("$1"); printf '  \033[31mFAIL\033[0m %s\n' "$1"; [ -n "${2:-}" ] && printf '       %s\n' "${2:0:600}"; }
check() { # check NAME CONDITION_EXIT_CODE [DETAIL]
  if [ "$2" = 0 ]; then ok "$1"; else bad "$1" "${3:-}"; fi
}
section() { printf '\n\033[1m%s\033[0m\n' "$*"; }
jqt() { local j=$1 f=$2; shift 2; jq -e "$@" "$f" <<<"$j" >/dev/null 2>&1; echo $?; } # jqt JSON FILTER [jq args] -> 0 when truthy
is() { [ "$1" = "$2" ]; echo $?; } # is ACTUAL EXPECTED -> 0 when equal

# incident CI_ID_OR_EMPTY SIGNAL prints the newest incident with that signal.
incident() {
  umb GET "/api/incidents?view=all&limit=1000${1:+&ci=$1}" |
    jq -c --arg s "$2" '[.items[] | select(.signal == $s)] | sort_by(.first_seen) | last // empty'
}
# wait_incident CI SIGNAL JQ_FILTER SECONDS prints the incident once the filter holds.
wait_incident() {
  local i a
  for ((i = 0; i <= $4; i += 2)); do
    a=$(incident "$1" "$2")
    if [ -n "$a" ] && jq -e "$3" <<<"$a" >/dev/null 2>&1; then printf '%s' "$a"; return 0; fi
    sleep 2
  done
  printf '%s' "$a"
  return 1
}
event() { # event ID HOST SIGNAL SEVERITY STATUS [TITLE] [METHOD]
  jq -nc --arg id "$1" --arg h "$2" --arg s "$3" --arg sev "$4" --arg st "$5" --arg t "${6:-Smoke $3 on $2}" --arg m "${7:-red}" \
    '{id:$id,host:$h,signal:$s,severity:$sev,status:$st,title:$t,method:$m,value:"42"}'
}
# Unix time of an RFC 3339 timestamp from the API (fractions dropped).
TS='sub("\\.[0-9]+"; "") | fromdateiso8601'

# Admin session and the smoke-test service account with a fresh API token.
ADMIN_TOKEN=
ADMIN_RESP=$(http POST "$UMB_URL/api/auth/token" "$(jq -nc --arg u "$UMB_ADMIN" --arg p "$UMBRELLA_ADMIN_PASSWORD" '{username:$u,password:$p}')")
ADMIN_STATUS=$(umb_status)
[ "$ADMIN_STATUS" = 200 ] && ADMIN_TOKEN=$(jq -r .token <<<"$ADMIN_RESP")
[ -n "$ADMIN_TOKEN" ] || die "cannot sign in to Umbrella at $UMB_URL as $UMB_ADMIN (status $ADMIN_STATUS): check UMBRELLA_ADMIN_PASSWORD in .env"
UMB_TOKEN=$ADMIN_TOKEN
SMOKE_UID=$(ensure_user smoke-test '{"username":"smoke-test","name":"Smoke test","service":true,"roles":["monitoring","auditor"],"business_services":[]}')
SMOKE_TOK_RESP=$(umb POST "/api/users/$SMOKE_UID/tokens" "$(jq -nc --arg n "smoke-$RUN" '{name:$n,days:1}')")
SMOKE_TOKEN=$(jq -r '.token // empty' <<<"$SMOKE_TOK_RESP")
SMOKE_TOKEN_ID=$(jq -r '.item.id // empty' <<<"$SMOKE_TOK_RESP")
[ -n "$SMOKE_TOKEN" ] || die "cannot create an API token for smoke-test: $SMOKE_TOK_RESP"
UMB_TOKEN=$SMOKE_TOKEN

section "1. Service is up"
umb GET /healthz >/dev/null; check "GET /healthz answers 200" $([ "$(umb_status)" = 200 ]; echo $?)
META=$(umb GET /api/meta)
check "GET /api/meta: version and PagerDuty mode" "$(jqt "$META" '.version and .pd_mode')" "$META"
SELF=$(umb GET /api/selfcheck)
check "GET /api/selfcheck: store, bus, PagerDuty status" "$(jqt "$SELF" '.store and .bus and .pagerduty.mode')" "$SELF"
BLOCKS=$(umb GET /api/blocks)
check "GET /api/blocks: block palette for the builder" "$(jqt "$BLOCKS" '[.items[].kind] | index("map.event") and index("trigger.webhook")')"
HTML=$(curl -sS "$UMB_URL/heatmap")
check "Web UI is served (SPA route /heatmap)" $(grep -qi '<div id="root"' <<<"$HTML"; echo $?)

section "2. Sign-in, roles and API tokens"
R=$(umb_as "" GET /api/incidents)
check "API without sign-in is refused (401 unauthorized)" $([ "$(umb_status)" = 401 ] && jq -e '.code == "unauthorized"' <<<"$R" >/dev/null; echo $?) "status $(umb_status) $R"
check "Sign-in POST /api/auth/token as $UMB_ADMIN (200, bearer token)" "$(jqt "$ADMIN_RESP" '(.token | length) > 0 and .must_change_password == false and .expires_in > 0')" "status $ADMIN_STATUS"
ME=$(umb_as "$ADMIN_TOKEN" GET /api/auth/me)
check "GET /api/auth/me: administrator with users.admin" "$(jqt "$ME" '(.user.roles | index("admin")) != null and (.permissions | index("users.admin")) != null and .all_services == true')" "$ME"
ROLES=$(umb_as "$ADMIN_TOKEN" GET /api/roles)
check "Built-in roles: admin, monitoring, oncall, owner, viewer, auditor, reader" "$(jqt "$ROLES" '[.items[].id] as $r | ["admin","monitoring","oncall","owner","viewer","auditor","reader"] | all(. as $x | $r | index($x) != null)')" "$ROLES"
ME=$(umb GET /api/auth/me)
check "API token of the service account smoke-test works (monitoring + auditor)" "$(jqt "$ME" '.user.username == "smoke-test" and .user.service == true and (.permissions | index("connectors.edit")) != null and (.permissions | index("users.admin")) == null')" "$ME"
umb GET /api/users >/dev/null
check "Service account without users.admin: GET /api/users is refused (403)" "$(is "$(umb_status)" 403)" "status $(umb_status)"
http POST "$UMB_URL/api/auth/token" '{"username":"smoke-test","password":"anything-1"}' >/dev/null
check "Service account cannot sign in with a password (401)" "$(is "$(umb_status)" 401)" "status $(umb_status)"
if [ -n "${UMBRELLA_GRAFANA_TOKEN:-}" ]; then
  USERS=$(umb_as "$ADMIN_TOKEN" GET /api/users)
  check "Service account grafana (role reader) from UMBRELLA_GRAFANA_TOKEN" "$(jqt "$USERS" 'any(.items[]; .username == "grafana" and .service == true and .roles == ["reader"] and .tokens >= 1)')" "$USERS"
  R=$(umb_as "$UMBRELLA_GRAFANA_TOKEN" GET '/api/incidents?view=open')
  check "Grafana token reads incidents (200)" $([ "$(umb_status)" = 200 ] && jq -e '.items' <<<"$R" >/dev/null; echo $?) "status $(umb_status)"
  R=$(umb_as "$UMBRELLA_GRAFANA_TOKEN" POST /api/incidents/bulk '{"ids":[],"action":"ack"}')
  check "Grafana token cannot act on incidents (403 forbidden)" $([ "$(umb_status)" = 403 ] && jq -e '.code == "forbidden"' <<<"$R" >/dev/null; echo $?) "status $(umb_status) $R"
fi
M=$(http GET "$UMB_URL/metrics")
if [ -n "${UMBRELLA_METRICS_TOKEN:-}" ]; then
  check "GET /metrics without the metrics token is refused (401)" "$(is "$(umb_status)" 401)" "status $(umb_status)"
  M=$(http GET "$UMB_URL/metrics" "" -H "Authorization: Bearer $UMBRELLA_METRICS_TOKEN")
fi
check "GET /metrics: Prometheus text with umbrella_up and incident gauges" $([ "$(umb_status)" = 200 ] && grep -q '^umbrella_up 1' <<<"$M" && grep -q '^umbrella_incidents_active{' <<<"$M"; echo $?) "status $(umb_status)"

section "3. CMDB"
TEAM=smoke
BS=$(ensure_ci "smoke-bs-$RUN" business_service $TEAM)
SVC=$(ensure_ci "smoke-service-$RUN" it_service $TEAM "$BS")
H1=$(ensure_ci "smoke-host-1-$RUN" host $TEAM "$SVC")
H2=$(ensure_ci "smoke-host-2-$RUN" host $TEAM "$SVC")
BS2=$(ensure_ci "smoke-bs2-$RUN" business_service smoke2)
SVC2=$(ensure_ci "smoke-service2-$RUN" it_service smoke2 "$BS2")
H3=$(ensure_ci "smoke-host-3-$RUN" host smoke2 "$SVC2")
check "CIs created: business service, IT service and two hosts below it" $([ -n "$BS" ] && [ -n "$SVC" ] && [ -n "$H1" ] && [ -n "$H2" ] && [ -n "$H3" ]; echo $?)
GRAPH=$(umb GET /api/cmdb/graph)
check "CMDB graph links the service to the hosts" "$(jqt "$GRAPH" '[.edges[] | select((.from // .source) == $s and (.to // .target) == $h)] | length > 0' --arg s "$SVC" --arg h "$H1")"
check "CMDB graph: business service depends on the IT service" "$(jqt "$GRAPH" '[.edges[] | select((.from // .source) == $b and (.to // .target) == $s)] | length > 0' --arg b "$BS" --arg s "$SVC")"

section "4. Connector builder"
SEV_MAP=$'critical=critical\nerror=error\nwarning=warning\n*=info'
G=$(webhook_graph openbao://lab/smoke severity "$SEV_MAP" $'source=smoke\nrun='"$RUN" \
  '${title}' '${host}' '${signal}' '${method|red}' '${status}' '${id}' '${value}')
DRY=$(umb POST /api/connectors/none/dry-run "$(jq -nc --argjson g "$G" --arg s "$(event d1 "smoke-host-1-$RUN" dry error firing)" '{graph:$g,sample:$s}')")
check "Dry-run: 7 blocks traced, one event out" "$(jqt "$DRY" '(.trace | length) == 7 and (.events | length) == 1')" "$DRY"
check "Dry-run: severity mapped, CI and signal templated" "$(jqt "$DRY" '.events[0] | .severity == "error" and .ci == $h and .signal == "dry" and .labels.source == "smoke"' --arg h "smoke-host-1-$RUN")" "$DRY"
DRYBAD=$(umb POST /api/connectors/none/dry-run "$(jq -nc --argjson g "$G" '{graph:$g,sample:"not json {"}')")
check "Dry-run: broken payload shows a block error" "$(jqt "$DRYBAD" '(.errors | length) > 0')" "$DRYBAD"

BADC=$(umb POST /api/connectors '{"name":"smoke-invalid","template":"webhook-json"}' | jq -r .id)
umb PUT "/api/connectors/$BADC" '{"draft":{"nodes":[{"id":"n1","kind":"parse.json","x":0,"y":0,"config":{}}],"edges":[]}}' >/dev/null
umb POST "/api/connectors/$BADC/publish" >/dev/null
check "Publish rejects a graph without a trigger (422)" $([ "$(umb_status)" = 422 ]; echo $?) "status $(umb_status)"
umb DELETE "/api/connectors/$BADC" >/dev/null

C1=$(ensure_webhook_connector smoke-webhook $TEAM "$G")
C2=$(ensure_webhook_connector smoke-second-source $TEAM "$G")
CONN=$(umb GET "/api/connectors/$C1")
check "Connectors published and running" "$(jqt "$CONN" '.connector.status == "running" and .connector.version >= 1 and (.ingest_url | length) > 0')" "$CONN"

section "5. Ingest and webhook auth"
umb POST "/api/ingest/$C1" "$(event x "smoke-host-1-$RUN" sig-a warning firing)" >/dev/null
check "Ingest without token is refused (401)" $([ "$(umb_status)" = 401 ]; echo $?) "status $(umb_status)"
ingest "$C1" wrong-token "$(event x "smoke-host-1-$RUN" sig-a warning firing)" >/dev/null
check "Ingest with a wrong token is refused (401)" $([ "$(umb_status)" = 401 ]; echo $?) "status $(umb_status)"
R=$(ingest "$C1" "$TOKEN" "$(event "a1-$RUN" "smoke-host-1-$RUN" sig-a warning firing)")
check "Ingest with token is accepted (202, 1 event)" $([ "$(umb_status)" = 202 ] && jq -e '.accepted == 1' <<<"$R" >/dev/null; echo $?) "status $(umb_status) $R"
A=$(wait_incident "$H1" sig-a '.status == "open"' 10)
check "Incident opened and bound to the CMDB host" "$(jqt "$A" '.ci_id == $h and .severity == "warning" and .team == "smoke" and (.service | startswith("smoke-service"))' --arg h "$H1")" "$A"
INC=$(jq -r .id <<<"$A")
check "Method RED taken from the payload" "$(jqt "$A" '.method == "red"')" "$A"

section "6. Dedup"
ingest "$C1" "$TOKEN" "$(event "a1-$RUN" "smoke-host-1-$RUN" sig-a warning firing)" >/dev/null
A=$(incident "$H1" sig-a)
check "Retry with the same external_id is dropped (inbox dedup)" "$(jqt "$A" '.count == 1')" "$A"
ingest "$C1" "$TOKEN" "$(event "a2-$RUN" "smoke-host-1-$RUN" sig-a error firing)" >/dev/null
A=$(incident "$H1" sig-a)
check "Repeat of the same signal folds into one incident" "$(jqt "$A" '.id == $id and .count == 2' --arg id "$INC")" "$A"
check "Severity escalates to the worst seen (error)" "$(jqt "$A" '.severity == "error"')" "$A"
ingest "$C2" "$TOKEN" "$(event "b1-$RUN" "smoke-host-1-$RUN" sig-a warning firing)" >/dev/null
A=$(incident "$H1" sig-a)
check "Same CI and signal from a second source: same incident, 2 sources" "$(jqt "$A" '.id == $id and (.sources | length) == 2' --arg id "$INC")" "$A"

section "7. Incident actions"
R=$(umb POST "/api/incidents/$INC/ack")
check "Ack: status acknowledged, who acked" "$(jqt "$R" '.status == "acknowledged" and .acked_by == "smoke-test"')" "$R"
umb POST "/api/incidents/$INC/ack" >/dev/null
check "Second ack is refused (409)" $([ "$(umb_status)" = 409 ]; echo $?) "status $(umb_status)"
umb POST "/api/incidents/$INC/comment" '{"text":"smoke comment"}' >/dev/null
D=$(umb GET "/api/incidents/$INC")
check "Comment lands in the timeline" "$(jqt "$D" '[.incident.timeline[] | select(.kind == "comment" and .text == "smoke comment")] | length == 1')"
check "Incident card lists its events" "$(jqt "$D" '(.events | length) >= 3')"

section "8. Recovery from sources"
ingest "$C1" "$TOKEN" "$(event "a2-$RUN" "smoke-host-1-$RUN" sig-a error resolved)" >/dev/null
A=$(incident "$H1" sig-a)
check "One source recovered, the other still firing: stays active" "$(jqt "$A" '.status == "acknowledged"')" "$A"
ingest "$C2" "$TOKEN" "$(event "b1-$RUN" "smoke-host-1-$RUN" sig-a warning ok)" >/dev/null
A=$(incident "$H1" sig-a)
check "All sources recovered: incident resolved" "$(jqt "$A" '.status == "resolved" and .resolved_at')" "$A"
ingest "$C1" "$TOKEN" "$(event "a3-$RUN" "smoke-host-1-$RUN" sig-a warning firing)" >/dev/null
A=$(incident "$H1" sig-a)
check "Repeat within the fold window reopens the same incident" "$(jqt "$A" '.id == $id and .status == "open"' --arg id "$INC")" "$A"
R=$(umb POST "/api/incidents/$INC/resolve")
check "Manual resolve" "$(jqt "$R" '.status == "resolved"')" "$R"

section "9. Maintenance"
NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ)
END=$(date -u -d '+1 hour' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -v+1H +%Y-%m-%dT%H:%M:%SZ)
START=$(date -u -d '-1 minute' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -v-1M +%Y-%m-%dT%H:%M:%SZ)
MW=$(umb POST /api/maintenance "$(jq -nc --arg ci "$H2" --arg s "$START" --arg e "$END" '{title:"smoke maintenance",ci_id:$ci,start:$s,end:$e}')")
check "Maintenance window created" $([ "$(umb_status)" = 201 ]; echo $?) "$MW"
MW_ID=$(jq -r .id <<<"$MW")
ingest "$C1" "$TOKEN" "$(event "m1-$RUN" "smoke-host-2-$RUN" sig-m critical firing)" >/dev/null
A=$(incident "$H2" sig-m)
check "Event during maintenance: incident suppressed, not sent to PagerDuty" "$(jqt "$A" '.suppressed == true and .pd_state == "skipped"')" "$A"
umb DELETE "/api/maintenance/$MW_ID" >/dev/null
check "Maintenance window deleted" $([ "$(umb_status)" = 204 ]; echo $?)

section "10. Unknown CI and parse errors"
ingest "$C1" "$TOKEN" "$(event "u1-$RUN" "smoke-unknown-$RUN" sig-u warning firing)" >/dev/null
A=$(incident "" sig-u)
AUTO=$(umb GET "/api/cis?q=smoke-unknown-$RUN" | jq -c --arg n "smoke-unknown-$RUN" '[.items[] | select(.name == $n)][0] // empty')
if [ -n "$AUTO" ]; then
  check "Event for a CI missing in CMDB: CI added automatically (origin auto, UMBRELLA_CMDB_AUTO)" "$(jqt "$AUTO" '.origin == "auto" and .type == "host"')" "$AUTO"
  check "  ...and the incident is bound to it" "$(jqt "$A" '.ci_id == $id' --arg id "$(jq -r .id <<<"$AUTO")")" "$A"
else
  check "Event for a CI missing in CMDB opens an unbound incident (UMBRELLA_CMDB_AUTO=false)" "$(jqt "$A" '.ci_name == $n and (.ci_id // "") == ""' --arg n "smoke-unknown-$RUN")" "$A"
fi
BEFORE=$(umb GET /api/parse-errors | jq '[.items[] | select(.connector_id == "'"$C1"'")] | length')
ingest "$C1" "$TOKEN" 'definitely not json {' >/dev/null
AFTER=$(umb GET /api/parse-errors | jq '[.items[] | select(.connector_id == "'"$C1"'")] | length')
check "Broken payload goes to the parse error list" $([ "$AFTER" -gt "$BEFORE" ]; echo $?) "before $BEFORE after $AFTER"

section "11. Bulk actions"
ingest "$C1" "$TOKEN" "$(event "k1-$RUN" "smoke-host-1-$RUN" sig-k1 info firing)" >/dev/null
ingest "$C1" "$TOKEN" "$(event "k2-$RUN" "smoke-host-1-$RUN" sig-k2 info firing)" >/dev/null
K1=$(incident "$H1" sig-k1 | jq -r .id); K2=$(incident "$H1" sig-k2 | jq -r .id)
R=$(umb POST /api/incidents/bulk "$(jq -nc --arg a "$K1" --arg b "$K2" '{ids:[$a,$b],action:"ack"}')")
check "Bulk ack of two incidents" "$(jqt "$R" '.done == 2')" "$R"
R=$(umb POST /api/incidents/bulk "$(jq -nc --arg a "$K1" --arg b "$K2" '{ids:[$a,$b],action:"resolve"}')")
check "Bulk resolve of two incidents" "$(jqt "$R" '.done == 2')" "$R"

section "12. Views"
L=$(umb GET "/api/incidents?view=closed&team=smoke&q=sig-k1")
check "Incident list: filters by view, team and text" "$(jqt "$L" '[.items[].id] | index($id) != null' --arg id "$K1")" "$L"
HM=$(umb GET "/api/heatmap?hours=6&group=service&team=smoke")
check "Heatmap: 24 columns, host row with incidents" "$(jqt "$HM" '(.columns | length) == 24 and ([.groups[].rows[] | select(.ci_id == $h and .total > 0)] | length) == 1' --arg h "$H1")"
HMT=$(umb GET "/api/heatmap?hours=24&group=team")
check "Heatmap grouped by team" "$(jqt "$HMT" '[.groups[].key] | index("smoke") != null')"
EV=$(umb GET "/api/events?connector=$C1&limit=50")
check "Event log keeps raw payload and labels" "$(jqt "$EV" '[.items[] | select(.labels.run == $r and (.raw | length) > 0)] | length >= 5' --arg r "$RUN")"
CIV=$(umb GET "/api/cis/$H1")
check "CI card opens" $([ "$(umb_status)" = 200 ]; echo $?)
WS=$(curl -sS -i -N --max-time 3 -H 'Connection: Upgrade' -H 'Upgrade: websocket' -H 'Sec-WebSocket-Version: 13' \
  -H 'Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==' -H "Origin: $UMB_URL" -H "Authorization: Bearer $UMB_TOKEN" "$UMB_URL/api/ws" 2>/dev/null | head -1)
check "Live updates: WebSocket handshake (101)" $(grep -q ' 101' <<<"$WS"; echo $?) "$WS"

section "13. PagerDuty gateway"
umb POST /api/selfcheck/pd-outage '{"on":true}' >/dev/null
ingest "$C1" "$TOKEN" "$(event "p1-$RUN" "smoke-host-1-$RUN" sig-pd critical firing)" >/dev/null
sleep 3
A=$(incident "$H1" sig-pd)
SELF=$(umb GET /api/selfcheck)
check "Simulated PagerDuty outage: delivery not accepted" "$(jqt "$A" '.pd_state == "pending" or .pd_state == "failed"')" "$A"
check "Self-check shows the outage" "$(jqt "$SELF" '.pagerduty.simulated_outage == true')" "$SELF"
umb POST /api/selfcheck/pd-outage '{"on":false}' >/dev/null
ingest "$C1" "$TOKEN" "$(event "p2-$RUN" "smoke-host-1-$RUN" sig-pd2 critical firing)" >/dev/null
A=$(wait_incident "$H1" sig-pd2 '.pd_state == "accepted"' 20)
check "After the outage: new incident accepted by PagerDuty (or dry-run)" $?  "$A"
for s in sig-pd sig-pd2; do umb POST "/api/incidents/$(incident "$H1" $s | jq -r .id)/resolve" >/dev/null; done

section "14. Connector lifecycle and audit"
umb POST "/api/connectors/$C2/stop" >/dev/null
ingest "$C2" "$TOKEN" "$(event "s1-$RUN" "smoke-host-1-$RUN" sig-s info firing)" >/dev/null
check "Stopped connector refuses events (409)" $([ "$(umb_status)" = 409 ]; echo $?) "status $(umb_status)"
umb POST "/api/connectors/$C2/start" >/dev/null
AUD=$(umb GET /api/audit)
check "Audit log: publish, maintenance, acks by smoke-test" "$(jqt "$AUD" '[.items[] | select(.actor == "smoke-test") | .action] | (any(startswith("connector.publish")) and index("maintenance.create") != null and index("alert.ack") != null)')"

section "15. Roles and service scope"
UMB_TOKEN=$ADMIN_TOKEN
OWNER_ID=$(ensure_user "smoke-owner-$RUN" "$(jq -nc --arg u "smoke-owner-$RUN" --arg p "$RUN_PW" --arg b "$BS" '{username:$u,password:$p,roles:["owner"],business_services:[$b],must_change_password:false}')")
VIEWER_ID=$(ensure_user "smoke-viewer-$RUN" "$(jq -nc --arg u "smoke-viewer-$RUN" --arg p "$RUN_PW" --arg b "$BS" '{username:$u,password:$p,roles:["viewer"],business_services:[$b],must_change_password:false}')")
UMB_TOKEN=$SMOKE_TOKEN
check "Users created: owner and viewer of smoke-bs-$RUN" $([ -n "$OWNER_ID" ] && [ -n "$VIEWER_ID" ]; echo $?)
umb_as "$ADMIN_TOKEN" POST /api/users '{"username":"smoke-weak","password":"short1","roles":["viewer"]}' >/dev/null
check "Password policy: a weak password is refused (400)" "$(is "$(umb_status)" 400)" "status $(umb_status)"
ingest "$C1" "$TOKEN" "$(event "o1-$RUN" "smoke-host-1-$RUN" sig-own warning firing)" >/dev/null
ingest "$C1" "$TOKEN" "$(event "o2-$RUN" "smoke-host-3-$RUN" sig-other warning firing)" >/dev/null
MINE=$(incident "$H1" sig-own | jq -r .id); OTHER=$(incident "$H3" sig-other | jq -r .id)
OWNER_TOKEN=$(umb_token "smoke-owner-$RUN" "$RUN_PW")
VIEWER_TOKEN=$(umb_token "smoke-viewer-$RUN" "$RUN_PW")
check "Owner and viewer sign in with their passwords" $([ -n "$OWNER_TOKEN" ] && [ -n "$VIEWER_TOKEN" ]; echo $?)
http POST "$UMB_URL/api/auth/token" "$(jq -nc --arg u "smoke-viewer-$RUN" '{username:$u,password:"wrong-password-1"}')" >/dev/null
check "Wrong password is refused (401)" "$(is "$(umb_status)" 401)" "status $(umb_status)"
L=$(umb_as "$OWNER_TOKEN" GET '/api/incidents?view=open&limit=1000')
check "Owner sees the incident of its business service" "$(jqt "$L" '[.items[].id] | index($id) != null' --arg id "$MINE")" "$MINE"
check "Owner does not see the incident of another business service" "$(jqt "$L" '[.items[].id] | index($id) == null' --arg id "$OTHER")" "$OTHER"
umb_as "$OWNER_TOKEN" GET "/api/incidents/$OTHER" >/dev/null
check "Owner opening a foreign incident gets 404" "$(is "$(umb_status)" 404)" "status $(umb_status)"
CIS=$(umb_as "$OWNER_TOKEN" GET '/api/cis')
check "Owner CMDB view: own hosts only" "$(jqt "$CIS" '[.items[].id] | (index($a) != null) and (index($b) == null)' --arg a "$H1" --arg b "$H3")"
umb_as "$OWNER_TOKEN" GET /api/connectors >/dev/null
check "Owner has no access to connectors (403)" "$(is "$(umb_status)" 403)" "status $(umb_status)"
L=$(umb_as "$VIEWER_TOKEN" GET '/api/incidents?view=open&limit=1000')
check "Viewer sees the incident of its business service" "$(jqt "$L" '[.items[].id] | index($id) != null' --arg id "$MINE")"
R=$(umb_as "$VIEWER_TOKEN" POST "/api/incidents/$MINE/ack")
check "Viewer cannot acknowledge (403 forbidden)" $([ "$(umb_status)" = 403 ] && jq -e '.code == "forbidden"' <<<"$R" >/dev/null; echo $?) "status $(umb_status) $R"
R=$(umb_as "$OWNER_TOKEN" POST "/api/incidents/$MINE/ack")
check "Owner acknowledges the incident of its service" "$(jqt "$R" '.status == "acknowledged" and .acked_by == $u' --arg u "smoke-owner-$RUN")" "$R"
for i in "$MINE" "$OTHER"; do umb POST "/api/incidents/$i/resolve" >/dev/null; done

if [ "$STACK_E2E" = 1 ]; then
  : "${GRAFANA_ADMIN_PASSWORD:?set in .env}"
  LAB_NODE_CI=$(ci_id umbrella-lab-node)
  TEXTFILE=node-textfile/umbrella_lab.prom
  PROM_SIGNAL=prometheus:UmbrellaLabTestFailure

  section "16. Prometheus -> Alertmanager -> Umbrella"
  curl -fsS -o /dev/null "$PROM_URL/-/ready" 2>/dev/null
  check "Prometheus is ready at $PROM_URL" $?
  TG=$(http GET "$PROM_URL/api/v1/targets?state=active")
  check "Prometheus targets up: prometheus, node, umbrella, alertmanager" "$(jqt "$TG" '[.data.activeTargets[] | select(.health == "up") | .labels.job] as $up | ["prometheus","node","umbrella","alertmanager"] | all(. as $j | $up | index($j) != null)')" "$(jq -c '[.data.activeTargets[] | {job:.labels.job,health,lastError}]' <<<"$TG" 2>/dev/null)"
  RU=$(http GET "$PROM_URL/api/v1/rules")
  check "Lab alert rules loaded (UmbrellaLabTestFailure, HostHighLoad)" "$(jqt "$RU" '[.data.groups[].rules[].name] | index("UmbrellaLabTestFailure") != null and index("HostHighLoad") != null')"
  PC=$(umb GET /api/connectors | jq -c '[.items[] | select(.slug == "prometheus")][0] // empty')
  check "Umbrella connector prometheus is running (/api/ingest/prometheus)" "$(jqt "$PC" '.status == "running"')" "$PC"
  # Start from a resolved alert: an open one left by an aborted run would hide the new firing.
  rm -f "$TEXTFILE"
  wait_incident "$LAB_NODE_CI" "$PROM_SIGNAL" '.status == "resolved"' 150 >/dev/null || true
  T0=$(date -u +%s)
  (umask 022; printf '# smoke-test %s\numbrella_lab_test_failure{check="smoke"} 1\n' "$RUN" > "$TEXTFILE.tmp" && mv "$TEXTFILE.tmp" "$TEXTFILE")
  A=$(wait_incident "$LAB_NODE_CI" "$PROM_SIGNAL" "(.status == \"open\" or .status == \"acknowledged\") and (.last_seen | $TS) >= $T0" 180)
  check "Textfile metric -> rule fires -> Alertmanager -> Umbrella incident (error)" $? "$A"
  check "Incident from Alertmanager: CI umbrella-lab-node, title from annotations.summary" "$(jqt "$A" '.ci_name == "umbrella-lab-node" and .severity == "error" and (.title | startswith("Lab test failure"))')" "$A"
  EV=$(umb GET "/api/events?connector=$(jq -r '.id // empty' <<<"$PC")&limit=20")
  check "Event keeps the Alertmanager labels (source, alertname)" "$(jqt "$EV" 'any(.items[]; .labels.source == "prometheus" and .labels.alertname == "UmbrellaLabTestFailure")')"
  rm -f "$TEXTFILE"
  A=$(wait_incident "$LAB_NODE_CI" "$PROM_SIGNAL" '.status == "resolved"' 240)
  check "Metric removed -> alert resolved -> Umbrella incident resolved" $? "$A"

  section "17. OpenSearch -> Umbrella"
  ST=$(osd GET /api/status)
  check "OpenSearch Dashboards status green at $OSD_URL" "$(jqt "$ST" '.status.overall.state == "green"')" "$(jq -c '.status.overall' <<<"$ST" 2>/dev/null)"
  osd GET /api/saved_objects/index-pattern/lab-logs >/dev/null
  check "Dashboards index pattern lab-logs* exists" "$(is "$(umb_status)" 200)" "status $(umb_status)"
  os_api GET /lab-logs/_mapping >/dev/null
  check "OpenSearch index lab-logs exists" "$(is "$(umb_status)" 200)" "status $(umb_status)"
  OC=$(umb GET /api/connectors | jq -c '[.items[] | select(.slug == "opensearch")][0] // empty')
  check "Umbrella connector opensearch (pull) is running" "$(jqt "$OC" '.status == "running"')" "$OC"
  NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  os_api POST '/lab-logs/_doc?refresh=true' "$(jq -nc --arg t "$NOW" --arg r "$RUN" '{"@timestamp":$t,level:"error",host:"shop-api-1",
    service:"lab-shop-api",message:("Smoke " + $r + ": payment gateway timeout"),error_code:("SMOKE-" + $r)}')" >/dev/null
  check "Error entry indexed into lab-logs" "$(is "$(umb_status)" 201)" "status $(umb_status)"
  os_api POST '/lab-logs/_doc?refresh=true' "$(jq -nc --arg t "$NOW" --arg r "$RUN" '{"@timestamp":$t,level:"info",host:"shop-api-1",
    service:"lab-shop-api",message:("Smoke " + $r + ": all good"),error_code:("INFO-" + $r)}')" >/dev/null
  OS_A=$(wait_incident "" "opensearch:lab-shop-api:SMOKE-$RUN" '.status == "open"' 90)
  check "Error log -> Umbrella incident on shop-api-1 (service lab-shop-api, error)" "$(jqt "$OS_A" '.ci_name == "shop-api-1" and .service == "lab-shop-api" and .severity == "error"')" "$OS_A"
  check "Info log entry opens no incident" "$([ -z "$(incident "" "opensearch:lab-shop-api:INFO-$RUN")" ]; echo $?)"
  sleep 20
  A=$(incident "" "opensearch:lab-shop-api:SMOKE-$RUN")
  check "Repeated polls of the same entry do not add events (dedup by _id)" "$(jqt "$A" '.count == 1')" "$A"
  OS_INC=$(jq -r '.id // empty' <<<"$OS_A")
  LAB_OWNER_TOKEN=$(umb_token lab-owner "${LAB_OWNER_PASSWORD:-}")
  ME=$(umb_as "$LAB_OWNER_TOKEN" GET /api/auth/me)
  check "User lab-owner (configure.sh) signs in, bound to lab-shop" "$(jqt "$ME" '.user.roles == ["owner"] and any(.business_services[]; .name == "lab-shop")')" "$ME"
  L=$(umb_as "$LAB_OWNER_TOKEN" GET '/api/incidents?view=open&limit=1000')
  check "lab-owner sees the OpenSearch incident of lab-shop, not the smoke ones" "$(jqt "$L" '([.items[].id] | index($a) != null) and all(.items[]; (.service // "") | startswith("smoke") | not)' --arg a "$OS_INC")" "$(jq -c '[.items[] | {id,service}]' <<<"$L" 2>/dev/null)"

  section "18. Notifications: Teams and Zoom"
  CH=$(umb GET /api/channels)
  TEAMS_CH=$(jq -r '[.items[] | select(.name == "Lab Teams")][0].id // empty' <<<"$CH")
  ZOOM_CH=$(jq -r '[.items[] | select(.name == "Lab Zoom")][0].id // empty' <<<"$CH")
  check "Channels Lab Teams (teams) and Lab Zoom (zoom) are enabled" "$(jqt "$CH" '[.items[] | select(.enabled) | .type] | index("teams") != null and index("zoom") != null')" "$CH"
  R=$(umb POST "/api/channels/$TEAMS_CH/test")
  check "Teams test message delivered (200)" "$(jqt "$R" '.ok == true and .status == 200')" "$R"
  R=$(umb POST "/api/channels/$ZOOM_CH/test")
  check "Zoom test message delivered (200)" "$(jqt "$R" '.ok == true and .status == 200')" "$R"
  DL=
  for i in $(seq 1 15); do
    DL=$(umb GET /api/deliveries)
    jq -e --arg a "$OS_INC" --arg t "$TEAMS_CH" --arg z "$ZOOM_CH" '[.items[] | select(.alert_id == $a and .event == "open" and .ok)] |
      (any(.channel_id == $t) and any(.channel_id == $z))' <<<"$DL" >/dev/null 2>&1 && break
    sleep 2
  done
  check "OpenSearch incident -> open message delivered to Teams and Zoom (ok)" "$(jqt "$DL" '[.items[] | select(.alert_id == $a and .event == "open" and .ok)] |
    (any(.channel_id == $t) and any(.channel_id == $z))' --arg a "$OS_INC" --arg t "$TEAMS_CH" --arg z "$ZOOM_CH")" "$(jq -c --arg a "$OS_INC" '[.items[] | select(.alert_id == $a)]' <<<"$DL" 2>/dev/null)"
  check "Zoom channel is limited to lab-shop: no message for the smoke service incident" "$(jqt "$DL" '[.items[] | select(.alert_id == $a and .channel_id == $z)] | length == 0' --arg a "$INC" --arg z "$ZOOM_CH")"
  check "Teams channel (all services) got the smoke incident" "$(jqt "$DL" 'any(.items[]; .alert_id == $a and .channel_id == $t and .ok)' --arg a "$INC" --arg t "$TEAMS_CH")"
  umb POST "/api/incidents/$OS_INC/resolve" >/dev/null
  for i in $(seq 1 10); do
    DL=$(umb GET /api/deliveries)
    jq -e --arg a "$OS_INC" 'any(.items[]; .alert_id == $a and .event == "resolve" and .ok)' <<<"$DL" >/dev/null 2>&1 && break
    sleep 2
  done
  check "Resolve message delivered" "$(jqt "$DL" 'any(.items[]; .alert_id == $a and .event == "resolve" and .ok)' --arg a "$OS_INC")"
  M=$(http GET "$UMB_URL/metrics" "" ${UMBRELLA_METRICS_TOKEN:+-H "Authorization: Bearer $UMBRELLA_METRICS_TOKEN"})
  check "Metrics count deliveries: umbrella_notifications_total{type=\"teams\",result=\"ok\"} > 0" $(grep -E '^umbrella_notifications_total\{.*result="ok".*type="teams"\} [1-9]' <<<"$M" >/dev/null; echo $?)

  section "19. Grafana"
  H=$(http GET "$GRAFANA_URL/api/health")
  check "Grafana is up at $GRAFANA_URL (database ok)" "$(jqt "$H" '.database == "ok"')" "$H"
  D=$(grafana GET /api/dashboards/uid/umbrella-incidents)
  check "Dashboard umbrella-incidents is provisioned" "$(jqt "$D" '.meta.provisioned == true and (.dashboard.panels | length) >= 15')" "status $(umb_status)"
  check "Dashboard links to Umbrella at the public address" "$(jqt "$D" '[.dashboard.templating.list[] | select(.name == "umbrella") | .query][0] == $u' --arg u "http://${PUBLIC_HOST:-localhost}:${UMBRELLA_PORT:-8080}")" "$(jq -c '[.dashboard.templating.list[] | select(.name == "umbrella")]' <<<"$D" 2>/dev/null)"
  R=$(grafana GET /api/datasources/uid/prometheus/health)
  check "Data source Prometheus: health OK" "$(jqt "$R" '.status == "OK"')" "$R"
  Q=$(grafana GET "/api/datasources/proxy/uid/prometheus/api/v1/query?query=umbrella_up")
  check "Grafana reads umbrella_up through Prometheus" "$(jqt "$Q" '.data.result[0].value[1] == "1"')" "$Q"
  R=$(grafana GET /api/datasources/uid/umbrella-api/health)
  check "Data source Umbrella API (Infinity, token of grafana): health OK" "$(jqt "$R" '.status == "OK"')" "$R (the Infinity plugin is installed from grafana.com at start)"
fi

if [ "$ZABBIX_E2E" = 1 ]; then
  section "20. Zabbix end-to-end"
  : "${ZABBIX_ADMIN_PASSWORD:?set in .env}"
  LAB_HOST=umbrella-lab-agent
  if zbx_login Admin "$ZABBIX_ADMIN_PASSWORD"; then
    ok "Zabbix API login"
    HOSTID=$(zbx host.get "$(jq -nc --arg h "$LAB_HOST" '{filter:{host:[$h]},output:["hostid"]}')" | jq -r '.[0].hostid // empty')
    ITEM=$(zbx item.get "$(jq -nc --arg h "$HOSTID" '{hostids:[$h],filter:{key_:"umbrella.test"},output:["itemid"]}')" | jq -r '.[0].itemid // empty')
    TRG=$(zbx trigger.get "$(jq -nc --arg h "$HOSTID" '{hostids:[$h],filter:{description:"Umbrella test problem"},output:["triggerid"]}')" | jq -r '.[0].triggerid // empty')
    check "Lab host, test item and trigger exist in Zabbix" $([ -n "$ITEM" ] && [ -n "$TRG" ]; echo $?)
    AVAIL=""
    for i in $(seq 1 20); do
      AVAIL=$(zbx host.get "$(jq -nc --arg h "$HOSTID" '{hostids:[$h],selectInterfaces:["available"],output:["hostid"]}')" | jq -r '.[0].interfaces[0].available')
      [ "$AVAIL" = 1 ] && break
      sleep 6
    done
    check "Zabbix agent 2 is reachable (interface available)" $([ "$AVAIL" = 1 ]; echo $?) "available=$AVAIL"
    LAB_CI=$(ci_id "$LAB_HOST")
    # Start from OK: a problem left open by an earlier run would not fire again.
    zbx history.push "$(jq -nc --arg i "$ITEM" '[{itemid:$i,value:"0"}]')" >/dev/null
    for i in $(seq 1 30); do
      [ "$(zbx trigger.get "$(jq -nc --arg t "$TRG" '{triggerids:[$t],output:["value"]}')" | jq -r '.[0].value')" = 0 ] && break
      sleep 2
    done
    sleep 2
    zbx history.push "$(jq -nc --arg i "$ITEM" '[{itemid:$i,value:"1"}]')" >/dev/null
    A=$(wait_incident "$LAB_CI" "zabbix:$TRG" '.status == "open" or .status == "acknowledged"' 120)
    check "Zabbix problem -> Umbrella incident on the lab host (High -> error)" "$(jqt "$A" '.severity == "error" and .method == "use"')" "$A"
    ZEV=$(zbx problem.get "$(jq -nc --arg t "$TRG" '{objectids:[$t],output:["eventid"]}')" | jq -r '.[0].eventid // empty')
    if [ -n "$ZEV" ]; then
      SENT=$(zbx alert.get "$(jq -nc --arg e "$ZEV" '{eventids:[$e],output:["status","error"]}')")
      check "Zabbix marks the webhook as sent (Umbrella acked with 2xx)" "$(jqt "$SENT" 'any(.[]; .status == "1")')" "$SENT"
    fi
    zbx history.push "$(jq -nc --arg i "$ITEM" '[{itemid:$i,value:"0"}]')" >/dev/null
    A=$(wait_incident "$LAB_CI" "zabbix:$TRG" '.status == "resolved"' 120)
    check "Zabbix recovery -> Umbrella incident resolved" $? "$A"
  else
    bad "Zabbix API login" "check ZABBIX_ADMIN_PASSWORD and run ./configure.sh"
  fi
fi

# Clean up what this run created in the access model.
UMB_TOKEN=$ADMIN_TOKEN
for u in "${OWNER_ID:-}" "${VIEWER_ID:-}"; do [ -n "$u" ] && umb DELETE "/api/users/$u" >/dev/null; done
[ -n "$SMOKE_TOKEN_ID" ] && umb DELETE "/api/users/$SMOKE_UID/tokens/$SMOKE_TOKEN_ID" >/dev/null

printf '\n\033[1mResult: %d passed, %d failed\033[0m\n' "$PASS" "$FAIL"
for f in "${FAILED[@]}"; do printf '  - %s\n' "$f"; done
[ "$FAIL" = 0 ]
