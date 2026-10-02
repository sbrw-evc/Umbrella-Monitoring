#!/usr/bin/env bash
# Checks the Umbrella MVP functions through its API, and with --zabbix also
# the real chain Zabbix trigger -> webhook -> Umbrella incident -> recovery.
#
#   ./smoke-test.sh             # Umbrella only (UMB_URL, default http://localhost:8080)
#   ./smoke-test.sh --zabbix    # plus Zabbix end-to-end (needs configure.sh done)
#
# Every run uses its own CI names and signals, so it can be repeated.
set -uo pipefail
cd "$(dirname "$0")"
[ -f .env ] && set -a && . ./.env && set +a
. ./lib.sh

ZABBIX_E2E=${ZABBIX_E2E:-0}
[ "${1:-}" = --zabbix ] && ZABBIX_E2E=1
: "${SMOKE_WEBHOOK_TOKEN:?set in .env (Umbrella reads it as UMB_SECRET_LAB_SMOKE)}"
TOKEN=$SMOKE_WEBHOOK_TOKEN
RUN=$(date +%H%M%S)$RANDOM
UMB_USER=smoke-test
PASS=0 FAIL=0
FAILED=()

ok()   { PASS=$((PASS + 1)); printf '  \033[32mPASS\033[0m %s\n' "$1"; }
bad()  { FAIL=$((FAIL + 1)); FAILED+=("$1"); printf '  \033[31mFAIL\033[0m %s\n' "$1"; [ -n "${2:-}" ] && printf '       %s\n' "$2"; }
check() { # check NAME CONDITION_EXIT_CODE [DETAIL]
  if [ "$2" = 0 ]; then ok "$1"; else bad "$1" "${3:-}"; fi
}
section() { printf '\n\033[1m%s\033[0m\n' "$*"; }
jqt() { local j=$1 f=$2; shift 2; jq -e "$@" "$f" <<<"$j" >/dev/null 2>&1; echo $?; } # jqt JSON FILTER [jq args] -> 0 when truthy

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

section "2. CMDB"
TEAM=smoke
SVC=$(ensure_ci "smoke-service-$RUN" it_service $TEAM)
H1=$(ensure_ci "smoke-host-1-$RUN" host $TEAM "$SVC")
H2=$(ensure_ci "smoke-host-2-$RUN" host $TEAM "$SVC")
check "CIs created: IT service and two hosts below it" $([ -n "$SVC" ] && [ -n "$H1" ] && [ -n "$H2" ]; echo $?)
GRAPH=$(umb GET /api/cmdb/graph)
check "CMDB graph links the service to the hosts" "$(jqt "$GRAPH" '[.edges[] | select((.from // .source) == $s and (.to // .target) == $h)] | length > 0' --arg s "$SVC" --arg h "$H1")"

section "3. Connector builder"
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

section "4. Ingest and webhook auth"
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

section "5. Dedup"
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

section "6. Incident actions"
R=$(umb POST "/api/incidents/$INC/ack")
check "Ack: status acknowledged, who acked" "$(jqt "$R" '.status == "acknowledged" and .acked_by == "smoke-test"')" "$R"
umb POST "/api/incidents/$INC/ack" >/dev/null
check "Second ack is refused (409)" $([ "$(umb_status)" = 409 ]; echo $?) "status $(umb_status)"
umb POST "/api/incidents/$INC/comment" '{"text":"smoke comment"}' >/dev/null
D=$(umb GET "/api/incidents/$INC")
check "Comment lands in the timeline" "$(jqt "$D" '[.incident.timeline[] | select(.kind == "comment" and .text == "smoke comment")] | length == 1')"
check "Incident card lists its events" "$(jqt "$D" '(.events | length) >= 3')"

section "7. Recovery from sources"
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

section "8. Maintenance"
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

section "9. Unknown CI and parse errors"
ingest "$C1" "$TOKEN" "$(event "u1-$RUN" "smoke-unknown-$RUN" sig-u warning firing)" >/dev/null
A=$(incident "" sig-u)
check "Event for a CI missing in CMDB opens an unbound incident" "$(jqt "$A" '.ci_name == $n and (.ci_id // "") == ""' --arg n "smoke-unknown-$RUN")" "$A"
BEFORE=$(umb GET /api/parse-errors | jq '[.items[] | select(.connector_id == "'"$C1"'")] | length')
ingest "$C1" "$TOKEN" 'definitely not json {' >/dev/null
AFTER=$(umb GET /api/parse-errors | jq '[.items[] | select(.connector_id == "'"$C1"'")] | length')
check "Broken payload goes to the parse error list" $([ "$AFTER" -gt "$BEFORE" ]; echo $?) "before $BEFORE after $AFTER"

section "10. Bulk actions"
ingest "$C1" "$TOKEN" "$(event "k1-$RUN" "smoke-host-1-$RUN" sig-k1 info firing)" >/dev/null
ingest "$C1" "$TOKEN" "$(event "k2-$RUN" "smoke-host-1-$RUN" sig-k2 info firing)" >/dev/null
K1=$(incident "$H1" sig-k1 | jq -r .id); K2=$(incident "$H1" sig-k2 | jq -r .id)
R=$(umb POST /api/incidents/bulk "$(jq -nc --arg a "$K1" --arg b "$K2" '{ids:[$a,$b],action:"ack"}')")
check "Bulk ack of two incidents" "$(jqt "$R" '.done == 2')" "$R"
R=$(umb POST /api/incidents/bulk "$(jq -nc --arg a "$K1" --arg b "$K2" '{ids:[$a,$b],action:"resolve"}')")
check "Bulk resolve of two incidents" "$(jqt "$R" '.done == 2')" "$R"

section "11. Views"
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
  -H 'Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==' -H "Origin: $UMB_URL" "$UMB_URL/api/ws" 2>/dev/null | head -1)
check "Live updates: WebSocket handshake (101)" $(grep -q ' 101' <<<"$WS"; echo $?) "$WS"

section "12. PagerDuty gateway"
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

section "13. Connector lifecycle and audit"
umb POST "/api/connectors/$C2/stop" >/dev/null
ingest "$C2" "$TOKEN" "$(event "s1-$RUN" "smoke-host-1-$RUN" sig-s info firing)" >/dev/null
check "Stopped connector refuses events (409)" $([ "$(umb_status)" = 409 ]; echo $?) "status $(umb_status)"
umb POST "/api/connectors/$C2/start" >/dev/null
AUD=$(umb GET /api/audit)
check "Audit log: publish, maintenance, acks by smoke-test" "$(jqt "$AUD" '[.items[] | select(.actor == "smoke-test") | .action] | (any(startswith("connector.publish")) and index("maintenance.create") != null and index("alert.ack") != null)')"

if [ "$ZABBIX_E2E" = 1 ]; then
  section "14. Zabbix end-to-end"
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

printf '\n\033[1mResult: %d passed, %d failed\033[0m\n' "$PASS" "$FAIL"
for f in "${FAILED[@]}"; do printf '  - %s\n' "$f"; done
[ "$FAIL" = 0 ]
