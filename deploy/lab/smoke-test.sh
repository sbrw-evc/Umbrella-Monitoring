#!/usr/bin/env bash
set -uo pipefail
LAB_DIR=$(cd "$(dirname "$0")" && pwd)
cd "$LAB_DIR" || exit 1
if [ -f .env ] && [ -z "${IN_TOOLBOX:-}" ]; then set -a; . ./.env; set +a; fi
. ./lib.sh

usage() {
  cat <<'TXT'
Usage: ./smoke-test.sh [--zabbix] [--stack] [--restart] [--all]

  (no options)  Umbrella: OpenBao, access, integrations, CMDB, incidents, rules, scope
  --zabbix      plus Zabbix trigger -> Umbrella incident -> recovery
  --stack       plus Prometheus/Alertmanager, PostgreSQL and container logs
                through Telegraf -> Kafka -> Fluentd -> OpenSearch, Grafana, NetBox
  --restart     plus restart of the umbrella container: data and sessions survive
  --all         --zabbix --stack
TXT
}
ZABBIX_E2E=0 STACK_E2E=0 RESTART=0
for a in "$@"; do
  case $a in
    --zabbix) ZABBIX_E2E=1 ;;
    --stack) STACK_E2E=1 ;;
    --restart) RESTART=1 ;;
    --all) ZABBIX_E2E=1 STACK_E2E=1 ;;
    -h | --help) usage; exit 0 ;;
    *) usage; exit 2 ;;
  esac
done
: "${UMBRELLA_ADMIN_PASSWORD:?set in .env}"
UMB_ADMIN=${UMBRELLA_ADMIN_USER:-admin}
RUN=$(date +%H%M%S)$RANDOM
RUN_PW="Smk$(openssl rand -hex 8)9"
PASS=0 FAIL=0
FAILED=()

TABLE=umbrella_smoke_missing_$RUN
TEXTFILE=node-textfile/umbrella_lab.prom
cleanup() {
  local tok=${ADMIN_TOKEN:-} id ids conn
  [ -n "$tok" ] || return 0
  printf '\n\033[1mCleanup\033[0m\n'
  rm -f "$TEXTFILE" "$TEXTFILE.tmp"
  ids=$(umb_as "$tok" GET "/api/incidents?view=all&limit=1000" | jq -r --arg run "$RUN" '.items[]
    | select((.signal | contains($run)) or .signal == "prometheus:UmbrellaLabTestFailure"
      or (.signal | startswith("zabbix:")) and .title == "Umbrella test problem"
      or (.title | contains("umbrella_smoke_missing")) or (.signal | startswith("red.smoke.")) or (.signal | startswith("smoke."))) | .id')
  for id in $ids; do umb_as "$tok" DELETE "/api/incidents/$id" >/dev/null; done
  printf '  incidents removed: %s\n' "$(wc -w <<<"$ids" | tr -d ' ')"
  id=$(umb_as "$tok" GET /api/integrations | jq -r '[.items[] | select(.name == "Smoke webhook")][0].id // empty')
  if [ -n "$id" ]; then
    conn=$(umb_as "$tok" GET "/api/integrations/$id" | jq -r '.connector_id // empty')
    [ -z "$conn" ] || umb_as "$tok" DELETE "/api/parse-errors?connector=$conn" >/dev/null
    umb_as "$tok" DELETE "/api/integrations/$id" >/dev/null
    printf '  integration Smoke webhook, its connector, parse errors and OpenBao secrets removed\n'
  fi
  for id in $(umb_as "$tok" GET /api/rules | jq -r '.items[] | select(.name | startswith("smoke-rule-")) | .id'); do umb_as "$tok" DELETE "/api/rules/$id" >/dev/null; done
  for id in $(umb_as "$tok" GET /api/teams | jq -r '.items[] | select(.managed and (.id | startswith("smoke-"))) | .id'); do umb_as "$tok" DELETE "/api/teams/$id?detach=1" >/dev/null; done
  for id in $(umb_as "$tok" GET /api/cis | jq -r --argjson before "${AUTO_BEFORE:-[]}" '.items[]
      | select((.name | startswith("smoke-ci-")) or (.origin == "auto" and (.id as $i | $before | index($i) | not))) | .id'); do
    umb_as "$tok" DELETE "/api/cis/$id" >/dev/null
  done
  for id in $(umb_as "$tok" GET /api/maintenance | jq -r '.items[] | select(.maintenance.title == "smoke") | .maintenance.id'); do umb_as "$tok" DELETE "/api/maintenance/$id" >/dev/null; done
  for id in $(umb_as "$tok" GET /api/users | jq -r '.items[] | select(.username | startswith("smoke-")) | .id'); do umb_as "$tok" DELETE "/api/users/$id" >/dev/null; done
  if [ "$STACK_E2E" = 1 ]; then
    os_api POST "/pg-logs-*/_delete_by_query?refresh=true" '{"query":{"match_phrase":{"message":"umbrella_smoke_missing"}}}' >/dev/null
    printf '  test log lines removed from OpenSearch\n'
  fi
  printf '  users, teams, rules, CIs and maintenance windows of the test removed (the audit log keeps the record)\n'
}
trap 'cleanup; rm -f "$UMB_STATUS_FILE"' EXIT

ok() { PASS=$((PASS + 1)); printf '  \033[32mPASS\033[0m %s\n' "$1"; }
bad() { FAIL=$((FAIL + 1)); FAILED+=("$1"); printf '  \033[31mFAIL\033[0m %s\n' "$1"; [ -n "${2:-}" ] && printf '       %s\n' "${2:0:600}"; }
check() { if [ "$2" = 0 ]; then ok "$1"; else bad "$1" "${3:-}"; fi; }
section() { printf '\n\033[1m%s\033[0m\n' "$*"; }
jqt() { local j=$1 f=$2; shift 2; jq -e "$@" "$f" <<<"$j" >/dev/null 2>&1; echo $?; }
is() { [ "$1" = "$2" ]; echo $?; }
TS='sub("\\.[0-9]+"; "") | fromdateiso8601'

incident() {
  umb GET "/api/incidents?view=all&limit=1000${1:+&ci=$1}" |
    jq -c --arg s "$2" '[.items[] | select(.signal == $s)] | sort_by(.first_seen) | last // empty'
}
wait_incident() {
  local i a
  for ((i = 0; i <= $4; i += 3)); do
    a=$(incident "$1" "$2")
    if [ -n "$a" ] && jq -e "$3" <<<"$a" >/dev/null 2>&1; then printf '%s' "$a"; return 0; fi
    sleep 3
  done
  printf '%s' "$a"
  return 1
}
psql_lab() {
  if [ -n "${IN_TOOLBOX:-}" ]; then
    PGPASSWORD=$ZABBIX_DB_PASSWORD psql -h zabbix-db -U zabbix -d zabbix -qAt -c "$1"
  else
    docker compose exec -T zabbix-db psql -U zabbix -d zabbix -qAt -c "$1"
  fi
}

section "1. Service, OpenBao and storage"
H=$(curl -fsSI "$UMB_URL/healthz" 2>/dev/null | tr -d '\r')
check "GET /healthz answers with X-Umbrella-Version" $(grep -qi '^x-umbrella-version: .' <<<"$H"; echo $?) "$H"
ADMIN_TOKEN=$(umb_token "$UMB_ADMIN" "$UMBRELLA_ADMIN_PASSWORD")
check "Sign-in as $UMB_ADMIN (password is read from OpenBao at start)" $([ -n "$ADMIN_TOKEN" ]; echo $?)
UMB_TOKEN=$ADMIN_TOKEN
AUTO_BEFORE=$(umb GET /api/cis | jq -c '[.items[] | select(.origin == "auto") | .id]')
META=$(umb GET /api/meta)
check "GET /api/meta: OpenBao connected, version" "$(jqt "$META" '.openbao == true and (.version | length) > 0')" "$META"
OB=$(umb GET /api/openbao)
check "OpenBao: unsealed, AppRole token, policy umbrella, KV umbrella/ readable" "$(jqt "$OB" '.sealed == false and .token_ok and .mount_ok and .auth == "approle" and (.policies | index("umbrella")) != null')" "$OB"
SELF=$(umb GET /api/selfcheck)
check "Self-check: state is stored in PostgreSQL umbrella-db, no write errors" "$(jqt "$SELF" '.persistence.kind == "postgres" and (.persistence.where | contains("umbrella-db")) and (.persistence.error // "") == ""')" "$(jq -c .persistence <<<"$SELF")"
SS=$(http GET "$UMB_URL/api/setup/status")
check "Setup wizard is finished (status is public, no setup required)" "$(jqt "$SS" '.required == false and .admin_exists == true')" "$SS"
UD=$(http GET "$UMB_URL/api/ui-defaults")
check "Default theme and language from the wizard (light, ru)" "$(jqt "$UD" '.theme == "light" and .locale == "ru"')" "$UD"
http POST "$UMB_URL/api/setup/complete" '{"theme":"dark"}' >/dev/null
check "Setup cannot be run again (409)" "$(is "$(umb_status)" 409)"
HTML=$(curl -fsS "$UMB_URL/cis" 2>/dev/null)
check "Web UI is served (route /cis)" $(grep -qi '<div id="root"' <<<"$HTML"; echo $?)
M=$(http GET "$UMB_URL/metrics" "" ${UMBRELLA_METRICS_TOKEN:+-H "Authorization: Bearer $UMBRELLA_METRICS_TOKEN"})
check "GET /metrics with the token from OpenBao" $([ "$(umb_status)" = 200 ] && grep -q '^umbrella_up 1' <<<"$M"; echo $?) "status $(umb_status)"

section "2. Access"
R=$(umb_as "" GET /api/incidents)
check "API without sign-in is refused (401)" "$(is "$(umb_status)" 401)"
ensure_user "smoke-$RUN" "$(jq -nc --arg u "smoke-$RUN" --arg p "$RUN_PW" '{username:$u,name:"smoke",password:$p,roles:["oncall"],business_services:[],must_change_password:false}')" >/dev/null
ONCALL=$(umb_token "smoke-$RUN" "$RUN_PW")
umb_as "$ONCALL" GET /api/users >/dev/null
check "On-call user without users.admin: GET /api/users is refused (403)" "$(is "$(umb_status)" 403)"
umb_as "$ONCALL" PUT /api/pagerduty '{"enabled":true}' >/dev/null
check "On-call user cannot change PagerDuty settings (403)" "$(is "$(umb_status)" 403)"

section "3. Secrets stay in OpenBao"
INTS=$(umb GET /api/integrations)
check "Integration secrets are references openbao://umbrella/integrations/..." "$(jqt "$INTS" '[.items[] | select(.secret_ref != null) | .secret_ref | startswith("openbao://umbrella/integrations/")] | all')"
LEAK=0
for v in "$ZABBIX_ADMIN_PASSWORD" "$NETBOX_API_TOKEN" "$PROMETHEUS_WEBHOOK_TOKEN" "$UMBRELLA_ADMIN_PASSWORD"; do
  for p in /api/integrations /api/pagerduty /api/settings /api/channels /api/connectors /api/audit; do
    grep -qF -- "$v" <<<"$(umb GET "$p")" && LEAK=1
  done
done
check "No password or token value appears in API answers" "$LEAK"

section "4. Integrations"
for name in "Zabbix" "Prometheus Alertmanager" "Prometheus" "NetBox" "PostgreSQL logs (OpenSearch)" "Container logs (OpenSearch)"; do
  id=$(integration_id "$name")
  if [ -z "$id" ]; then bad "Integration $name exists" "run ./configure.sh"; continue; fi
  R=$(umb POST "/api/integrations/$id/check")
  check "Integration $name: connection check" "$(jqt "$R" '.ok == true')" "$(jq -r '.message // .error' <<<"$R")"
done
R=$(umb POST "/api/integrations/$(integration_id NetBox)/sync")
check "NetBox: devices and VMs loaded as CIs with owners" "$(jqt "$R" '.ok == true')" "$(jq -r .message <<<"$R")"

section "5. CMDB: one CI per host from Zabbix, Prometheus and NetBox"
CIS=$(umb GET /api/cis)
HOST_CI=$(jq -c --arg h "$LAB_HOST" '[.items[] | select(.name == $h)][0] // empty' <<<"$CIS")
check "CI $LAB_HOST carries identities from zabbix, prometheus and netbox" "$(jqt "$HOST_CI" '[.identities[] | select(.until == null) | .kind] as $k | ["zabbix","prometheus","netbox"] | all(. as $x | $k | index($x) != null)')" "$(jq -c '[.identities[].kind]' <<<"$HOST_CI" 2>/dev/null)"
check "CI $LAB_HOST: owner from NetBox (Олег Иванов), team infra" "$(jqt "$HOST_CI" 'any(.owners[]; .email == "oleg.ivanov@lab.local") and .team == "infra"')" "$(jq -c '{owners,team}' <<<"$HOST_CI" 2>/dev/null)"
DB_CI=$(jq -c '[.items[] | select(.name == "zabbix-db")][0] // empty' <<<"$CIS")
check "CI zabbix-db: database, owner Ирина Смирнова (DBA)" "$(jqt "$DB_CI" '.type == "database" and any(.owners[]; .email == "irina.smirnova@lab.local")')" "$(jq -c '{type,owners}' <<<"$DB_CI" 2>/dev/null)"
KAFKA_CI=$(jq -c '[.items[] | select(.name == "kafka")][0] // empty' <<<"$CIS")
check "CI kafka: owner inherited from tenant Мониторинг (Мария Козлова)" "$(jqt "$KAFKA_CI" 'any(.owners[]; .email == "maria.kozlova@lab.local")')" "$(jq -c .owners <<<"$KAFKA_CI" 2>/dev/null)"
T=$(umb POST /api/cis "$(jq -nc --arg n "smoke-ci-$RUN" '{name:$n,type:"host",team:"smoke",owners:[{name:"Smoke",email:"smoke@lab.local"}],identities:[{kind:"ip",value:"10.255.0.9"}]}')")
TID=$(jq -r '.id // empty' <<<"$T")
umb PUT "/api/cis/$TID" '{"description":"edited by smoke test"}' >/dev/null
check "CI create and edit (owners, identities)" "$(is "$(umb_status)" 200)"
umb POST /api/relations "$(jq -nc --arg f "$(jq -r .id <<<"$KAFKA_CI")" --arg t "$TID" '{from:$f,to:$t,type:"depends_on"}')" >/dev/null
check "Relation create" "$(is "$(umb_status)" 201)"
umb DELETE "/api/cis/$TID" >/dev/null
check "CI delete (relations removed with it)" "$(is "$(umb_status)" 204)"

TM=$(umb GET /api/teams)
check "Teams monitoring, infra, dba are registered, with members and CIs" "$(jqt "$TM" '[.items[] | select(.managed)] as $m | ["monitoring","infra","dba"] | all(. as $id | $m | any(.id == $id and (.members | length) > 0)) and any($m[]; .id == "dba" and .cis > 0)')" "$(jq -c '[.items[] | {id,managed,cis,members}]' <<<"$TM")"
umb POST /api/teams "$(jq -nc --arg id "smoke-$RUN" '{id:$id,name:"Smoke team",email:"smoke@lab.local"}')" >/dev/null
check "Team create" "$(is "$(umb_status)" 201)"
umb PUT "/api/teams/smoke-$RUN" '{"name":"Smoke team 2","description":"edited"}' >/dev/null
check "Team edit" "$(is "$(umb_status)" 200)"
umb DELETE "/api/teams/smoke-$RUN" >/dev/null
check "Team delete" "$(is "$(umb_status)" 204)"
umb DELETE /api/teams/dba >/dev/null
check "A team in use is not deleted without detaching (409)" "$(is "$(umb_status)" 409)"

section "6. Incidents through a webhook integration"
WH=$(ensure_integration "Smoke webhook" '{"type":"webhook","name":"Smoke webhook","slug":"smoke","team":"smoke","params":{"ci":"${host}","signal":"${signal}"}}')
WT=$(umb POST "/api/integrations/$WH/token" | jq -r .token)
send() { http POST "$UMB_URL/api/ingest/smoke" "$1" ${2:+-H "X-Umbrella-Token: $2"}; }
ev() { jq -nc --arg id "$1" --arg h "$2" --arg s "$3" --arg sev "$4" --arg st "$5" '{id:$id,host:$h,signal:$s,severity:$sev,status:$st,title:("Smoke " + $s + " on " + $h)}'; }
send "$(ev "a$RUN" kafka "smoke.$RUN" warning firing)" >/dev/null
check "Ingest without the token is refused (401)" "$(is "$(umb_status)" 401)"
send "$(ev "a$RUN" kafka "smoke.$RUN" warning firing)" "$WT" >/dev/null
check "Ingest with the token from OpenBao (202)" "$(is "$(umb_status)" 202)"
send "$(ev "a$RUN" kafka "smoke.$RUN" warning firing)" "$WT" >/dev/null
send "$(ev "b$RUN" kafka "smoke.$RUN" critical firing)" "$WT" >/dev/null
A=$(incident "$(jq -r .id <<<"$KAFKA_CI")" "smoke.$RUN")
check "Repeated external_id dropped, same CI + signal folded, severity raised" "$(jqt "$A" '.count == 2 and .severity == "critical" and .ci_name == "kafka"')" "$A"
INC=$(jq -r .id <<<"$A")
D=$(umb GET "/api/incidents/$INC")
check "Incident card has events and the CI owners are known" "$(jqt "$D" '(.events | length) == 2')"
umb POST "/api/incidents/$INC/ack" >/dev/null
check "Acknowledge (200), second acknowledge is refused (409)" $([ "$(umb_status)" = 200 ] && umb POST "/api/incidents/$INC/ack" >/dev/null; [ "$(umb_status)" = 409 ]; echo $?)
send "$(ev "c$RUN" kafka "smoke.$RUN" critical resolved)" "$WT" >/dev/null
A=$(incident "$(jq -r .id <<<"$KAFKA_CI")" "smoke.$RUN")
check "Source recovery resolves the incident" "$(jqt "$A" '.status == "resolved"')" "$A"
check "PagerDuty state is honest: delivered, or failed with the reason" "$(jqt "$A" '.pd_state == "accepted" or .pd_state == "acked" or (.pd_state == "failed" and (.pd_error | length) > 0)')" "$(jq -c '{pd_state,pd_error}' <<<"$A")"
MW=$(umb POST /api/maintenance "$(jq -nc --arg c "$(jq -r .id <<<"$DB_CI")" --arg s "$(date -u -d '-1 min' +%FT%TZ 2>/dev/null || date -u -v-1M +%FT%TZ)" --arg e "$(date -u -d '+10 min' +%FT%TZ 2>/dev/null || date -u -v+10M +%FT%TZ)" '{title:"smoke",ci_id:$c,start:$s,end:$e}')" | jq -r '.id // empty')
send "$(ev "m$RUN" zabbix-db "smoke.mw.$RUN" critical firing)" "$WT" >/dev/null
A=$(incident "$(jq -r .id <<<"$DB_CI")" "smoke.mw.$RUN")
check "Maintenance window suppresses the incident (no PagerDuty)" "$(jqt "$A" '.suppressed == true and .pd_state == "skipped"')" "$A"
[ -n "$MW" ] && umb DELETE "/api/maintenance/$MW" >/dev/null
umb POST "/api/incidents/$(jq -r .id <<<"$A")/resolve" >/dev/null
http POST "$UMB_URL/api/ingest/smoke" '{broken' -H "X-Umbrella-Token: $WT" >/dev/null
PE=$(umb GET "/api/parse-errors?connector=$(jq -r .connector_id <<<"$(umb GET "/api/integrations/$WH")")")
check "Broken body goes to parse errors" "$(jqt "$PE" '(.items | length) > 0')"

section "7. RED/USE rules"
RL=$(umb GET /api/rules)
check "Lab rules exist and evaluate without errors" "$(jqt "$RL" '(.items | length) >= 7 and all(.items[]; (.last_error // "") == "")')" "$(jq -c '[.items[] | {name,last_error}]' <<<"$RL")"
check "Lab rules fire at low thresholds (at least one firing series)" "$(jqt "$RL" 'any(.items[]; (.firing // 0) > 0)')" "$(jq -c '[.items[] | {name,series,pending,firing}]' <<<"$RL")"
PROM_INT=$(integration_id Prometheus)
RB=$(jq -nc --arg s "$PROM_INT" --arg n "smoke-rule-$RUN" '{name:$n,method:"red",signal:("red.smoke." + $n),source_id:$s,query:"up{job=\"prometheus\"}",ci_label:"host",op:">",threshold:0,for:"0s",interval:"30s",severity:"info",enabled:true}')
PV=$(umb POST /api/rules/preview "$RB")
check "Rule preview on live Prometheus data" "$(jqt "$PV" '.matched >= 1')" "$PV"
RID=$(umb POST /api/rules "$RB" | jq -r '.id // empty')
umb POST "/api/rules/$RID/evaluate" >/dev/null
A=$(wait_incident "" "red.smoke.smoke-rule-$RUN" '.status == "open"' 30)
check "Rule fires -> incident on CI prometheus (method red)" "$(jqt "$A" '.ci_name == "prometheus" and .method == "red"')" "$A"
umb DELETE "/api/rules/$RID" >/dev/null
A=$(wait_incident "" "red.smoke.smoke-rule-$RUN" '.status == "resolved"' 30)
check "Deleting the rule resolves its incident" $? "$A"

section "8. Service scope"
OWNER=$(umb_token lab-owner "${LAB_OWNER_PASSWORD:-}")
NB_CI=$(jq -r '[.items[] | select(.name == "netbox-db")][0].id // empty' <<<"$CIS")
send "$(ev "s1$RUN" kafka "smoke.scope.$RUN" error firing)" "$WT" >/dev/null
send "$(ev "s2$RUN" netbox-db "smoke.scope.$RUN" error firing)" "$WT" >/dev/null
MINE=$(umb_as "$OWNER" GET "/api/incidents?view=open&q=smoke.scope.$RUN")
check "lab-owner sees the kafka incident (Мониторинг) and not netbox-db (Каталог)" "$(jqt "$MINE" '[.items[].ci_name] == ["kafka"]')" "$(jq -c '[.items[].ci_name]' <<<"$MINE")"
for c in "$(jq -r .id <<<"$KAFKA_CI")" "$NB_CI"; do
  i=$(incident "$c" "smoke.scope.$RUN" | jq -r '.id // empty')
  [ -n "$i" ] && umb POST "/api/incidents/$i/resolve" >/dev/null
done

if [ "$ZABBIX_E2E" = 1 ]; then
  section "9. Zabbix -> Umbrella"
  if zbx_login Admin "$ZABBIX_ADMIN_PASSWORD"; then
    HOSTID=$(zbx host.get "$(jq -nc --arg h "$LAB_HOST" '{filter:{host:[$h]},output:["hostid"]}')" | jq -r '.[0].hostid // empty')
    ITEM=$(zbx item.get "$(jq -nc --arg h "$HOSTID" '{hostids:[$h],filter:{key_:"umbrella.test"},output:["itemid"]}')" | jq -r '.[0].itemid // empty')
    TRG=$(zbx trigger.get "$(jq -nc --arg h "$HOSTID" '{hostids:[$h],filter:{description:"Umbrella test problem"},output:["triggerid"]}')" | jq -r '.[0].triggerid // empty')
    MT=$(zbx mediatype.get '{"filter":{"name":"Umbrella"},"output":["mediatypeid"]}' | jq -r '.[0].mediatypeid // empty')
    check "Umbrella created the media type and action in Zabbix" $([ -n "$MT" ]; echo $?)
    AVAIL=""
    for i in $(seq 1 20); do
      AVAIL=$(zbx host.get "$(jq -nc --arg h "$HOSTID" '{hostids:[$h],selectInterfaces:["available"],output:["hostid"]}')" | jq -r '.[0].interfaces[0].available')
      [ "$AVAIL" = 1 ] && break
      sleep 6
    done
    check "Zabbix agent 2 on $LAB_HOST is available" "$(is "$AVAIL" 1)"
    ZMEM=""
    for i in $(seq 1 20); do
      ZMEM=$(zbx item.get "$(jq -nc --arg h "$HOSTID" '{hostids:[$h],filter:{key_:"vm.memory.size[total]"},output:["lastvalue"]}')" | jq -r '.[0].lastvalue // empty')
      [ -n "$ZMEM" ] && [ "$ZMEM" != 0 ] && break
      sleep 6
    done
    PMEM=$(http GET "$PROM_URL/api/v1/query?query=node_memory_MemTotal_bytes" | jq -r '.data.result[0].value[1] // empty')
    check "Zabbix and node-exporter see the same host memory (Zabbix $ZMEM, node-exporter $PMEM)" \
      "$(jq -n --arg z "${ZMEM:-0}" --arg p "${PMEM:-0}" '($z|tonumber) > 0 and (($z|tonumber) - ($p|tonumber) | fabs) / ($p|tonumber) < 0.05' | grep -q true; echo $?)"
    HCI=$(jq -r '.id // empty' <<<"$HOST_CI")
    zbx history.push "$(jq -nc --arg i "$ITEM" '[{itemid:$i,value:"0"}]')" >/dev/null
    for i in $(seq 1 30); do
      [ "$(zbx trigger.get "$(jq -nc --arg t "$TRG" '{triggerids:[$t],output:["value"]}')" | jq -r '.[0].value')" = 0 ] && break
      sleep 2
    done
    sleep 2
    zbx history.push "$(jq -nc --arg i "$ITEM" '[{itemid:$i,value:"1"}]')" >/dev/null
    A=$(wait_incident "$HCI" "zabbix:$TRG" '.status == "open" or .status == "acknowledged"' 120)
    check "Zabbix problem -> incident on CI $LAB_HOST (High -> error)" "$(jqt "$A" '.severity == "error"')" "$A"
    zbx history.push "$(jq -nc --arg i "$ITEM" '[{itemid:$i,value:"0"}]')" >/dev/null
    A=$(wait_incident "$HCI" "zabbix:$TRG" '.status == "resolved"' 120)
    check "Zabbix recovery -> incident resolved" $? "$A"
    ZI=$(umb GET "/api/incidents?view=all&ci=$HCI&limit=1000")
    check "Low Zabbix thresholds: Linux template problems reach Umbrella" "$(jqt "$ZI" 'any(.items[]; (.signal | startswith("zabbix:")) and .signal != ("zabbix:" + $t))' --arg t "$TRG")" "wait a few minutes after install: triggers need 5 minutes of data"
  else
    bad "Zabbix API login" "check ZABBIX_ADMIN_PASSWORD"
  fi
fi

if [ "$STACK_E2E" = 1 ]; then
  section "10. Prometheus -> Alertmanager -> Umbrella"
  TG=$(http GET "$PROM_URL/api/v1/targets?state=active")
  check "Prometheus targets up: node, cadvisor, umbrella, alertmanager, prometheus" "$(jqt "$TG" '[.data.activeTargets[] | select(.health == "up") | .labels.job] as $up | ["node","cadvisor","umbrella","alertmanager","prometheus"] | all(. as $j | $up | index($j) != null)')" "$(jq -c '[.data.activeTargets[] | {job:.labels.job,health,lastError}]' <<<"$TG" 2>/dev/null)"
  Q=$(http GET "$PROM_URL/api/v1/query?query=count(container_last_seen%7Bservice!%3D%22%22%7D)")
  check "cAdvisor: Docker containers are measured with their compose service" "$(jqt "$Q" '(.data.result[0].value[1] | tonumber) >= 10')" "$Q"
  HCI=$(jq -r '.id // empty' <<<"$HOST_CI")
  rm -f "$TEXTFILE"
  wait_incident "$HCI" prometheus:UmbrellaLabTestFailure '.status == "resolved"' 120 >/dev/null || true
  T0=$(date -u +%s)
  (umask 022; printf 'umbrella_lab_test_failure{check="smoke"} 1\n' > "$TEXTFILE.tmp" && mv "$TEXTFILE.tmp" "$TEXTFILE")
  A=$(wait_incident "$HCI" prometheus:UmbrellaLabTestFailure "(.status == \"open\" or .status == \"acknowledged\") and (.last_seen | $TS) >= $T0" 180)
  check "Textfile metric -> rule -> Alertmanager -> incident on CI $LAB_HOST" $? "$A"
  rm -f "$TEXTFILE"
  A=$(wait_incident "$HCI" prometheus:UmbrellaLabTestFailure '.status == "resolved"' 180)
  check "Metric removed -> incident resolved" $? "$A"
  AI=$(umb GET "/api/incidents?view=all&limit=1000")
  check "Low Prometheus thresholds: host or container alerts reach Umbrella" "$(jqt "$AI" 'any(.items[]; .signal | test("^prometheus:(HostCpuBusy|HostLoadHigh|HostMemoryUsed|ContainerCpuBusy|ContainerMemoryLarge)$"))')"

  section "11. PostgreSQL logs: Telegraf -> Kafka -> Fluentd -> OpenSearch -> Umbrella"
  psql_lab "select * from $TABLE" >/dev/null 2>&1
  DOC=""
  for i in $(seq 1 40); do
    DOC=$(os_api POST "/pg-logs-*/_search" "$(jq -nc --arg t "$TABLE" '{size:1,query:{bool:{must:[{match_phrase:{message:$t}}],filter:[{term:{pg_level:"ERROR"}}]}}}')")
    [ "$(jq -r '.hits.hits | length' <<<"$DOC" 2>/dev/null)" -gt 0 ] 2>/dev/null && break
    sleep 3
  done
  check "The log line reached OpenSearch (pg-logs-*), parsed by Fluentd" "$(jqt "$DOC" '.hits.hits[0]._source | .level == "error" and .error_code == "SQLSTATE_42P01" and .host == "zabbix-db" and .service == "postgresql"')" "$(jq -c '.hits.hits[0]._source' <<<"$DOC" 2>/dev/null)"
  DBID=$(jq -r '.id // empty' <<<"$DB_CI")
  A=$(wait_incident "$DBID" opensearch:postgresql:SQLSTATE_42P01 '.status == "open" or .status == "acknowledged"' 90)
  check "Umbrella incident on CI zabbix-db from the PostgreSQL log" $? "$A"
  SL=$(os_api POST "/pg-logs-*/_count" '{"query":{"bool":{"filter":[{"term":{"service":"postgresql"}},{"range":{"@timestamp":{"gte":"now-1h"}}}]}}}')
  check "PostgreSQL log stream is flowing (documents in the last hour)" "$(jqt "$SL" '.count > 0')" "$SL"
  [ -n "$(jq -r '.id // empty' <<<"$A")" ] && umb POST "/api/incidents/$(jq -r .id <<<"$A")/resolve" >/dev/null

  section "12. Container logs -> OpenSearch -> Umbrella"
  CL=$(os_api POST "/container-logs-*/_search" '{"size":0,"aggs":{"svc":{"terms":{"field":"service","size":50}}}}')
  check "Container logs of several services are in OpenSearch (container-logs-*)" "$(jqt "$CL" '(.aggregations.svc.buckets | length) >= 5')" "$(jq -c '[.aggregations.svc.buckets[].key]' <<<"$CL" 2>/dev/null)"
  CLE=$(os_api POST "/container-logs-*/_search" '{"size":0,"query":{"range":{"@timestamp":{"gte":"now-15m"}}},"aggs":{"lvl":{"terms":{"field":"level"}}}}')
  check "Container log lines are fresh and classified by level" "$(jqt "$CLE" '.hits.total.value > 0 and (.aggregations.lvl.buckets | length) > 0')" "$(jq -c '[.aggregations.lvl.buckets[] | {key,doc_count}]' <<<"$CLE" 2>/dev/null)"
  if [ -z "${IN_TOOLBOX:-}" ]; then
    TOPICS=$(docker compose exec -T kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list 2>/dev/null)
    check "Kafka topics logs.postgres_log and logs.docker_log exist" $(grep -q '^logs.postgres_log$' <<<"$TOPICS" && grep -q '^logs.docker_log$' <<<"$TOPICS"; echo $?) "$TOPICS"
  fi

  section "13. Grafana and NetBox"
  G=$(grafana GET /api/health)
  check "Grafana is healthy" "$(jqt "$G" '.database == "ok"')" "$G"
  DS=$(grafana GET /api/datasources)
  check "Grafana data sources: Prometheus, Umbrella API, PostgreSQL logs, Container logs" "$(jqt "$DS" '[.[].name] as $n | ["Prometheus","Umbrella API","PostgreSQL logs","Container logs"] | all(. as $x | $n | index($x) != null)')" "$(jq -c '[.[].name]' <<<"$DS" 2>/dev/null)"
  S=$(umb GET /api/settings)
  check "Umbrella opens incident context in Grafana" "$(jqt "$S" '.settings.grafana_url | contains("/d/umbrella-incidents")')"
  NBS=$(nb GET /api/status/)
  check "NetBox API answers" "$(jqt "$NBS" '."netbox-version" | length > 0')" "$NBS"
fi

if [ "$RESTART" = 1 ] && [ -z "${IN_TOOLBOX:-}" ]; then
  section "14. Update safety: restart of the umbrella container"
  BEFORE=$(umb GET "/api/incidents?view=all&limit=1000" | jq '.total')
  ICOUNT=$(umb GET /api/integrations | jq '.items | length')
  docker compose restart umbrella >/dev/null
  wait_http "$UMB_URL/healthz" 120
  AFTER=$(umb GET "/api/incidents?view=all&limit=1000" | jq '.total')
  check "Session token still valid after restart" "$(is "$(umb_status)" 200)"
  check "Incidents kept ($BEFORE -> $AFTER)" $([ "${AFTER:-0}" -ge "${BEFORE:-1}" ]; echo $?)
  check "Integrations kept" "$(is "$(umb GET /api/integrations | jq '.items | length')" "$ICOUNT")"
  check "State is read back from PostgreSQL" "$(jqt "$(umb GET /api/selfcheck)" '.persistence.kind == "postgres"')"
fi

UMB_TOKEN=$ADMIN_TOKEN

printf '\n\033[1mResult: %d passed, %d failed\033[0m\n' "$PASS" "$FAIL"
for f in "${FAILED[@]}"; do printf '  - %s\n' "$f"; done
[ "$FAIL" = 0 ]
