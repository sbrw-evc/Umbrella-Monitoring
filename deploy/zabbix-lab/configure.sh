#!/usr/bin/env bash
# Connects Zabbix to Umbrella. Safe to run again: it finds what it created
# before and updates it. Umbrella MVP keeps its data in memory, so run this
# again after the umbrella container restarts.
#
#   ./configure.sh            # reads .env next to this script
#
# Umbrella: CMDB entries for the lab, the "Zabbix" webhook connector
# (parse.json -> map.severity -> enrich.labels -> map.event -> out.event ->
# ack.response), published and running.
# Zabbix: Admin password from .env, the "Umbrella" webhook media type, media
# for Admin, the "Send problems to Umbrella" action, the lab agent host with
# the Linux template and a trapper item with a trigger the test can fire.
set -euo pipefail
cd "$(dirname "$0")"
[ -f .env ] && set -a && . ./.env && set +a
. ./lib.sh

: "${ZABBIX_WEBHOOK_TOKEN:?set in .env}"
: "${ZABBIX_ADMIN_PASSWORD:?set in .env}"
# Zabbix server reaches Umbrella over the compose network.
UMB_INTERNAL_URL=${UMB_INTERNAL_URL:-http://umbrella:8080}
LAB_HOST=umbrella-lab-agent
LAB_TEAM=${LAB_TEAM:-monitoring}

log "Waiting for Umbrella at $UMB_URL"
wait_http "$UMB_URL/healthz" 180 || die "Umbrella does not answer at $UMB_URL"

log "Umbrella: CMDB entries for the lab"
SVC_ID=$(ensure_ci umbrella-lab it_service "$LAB_TEAM")
HOST_ID=$(ensure_ci "$LAB_HOST" host "$LAB_TEAM" "$SVC_ID")
log "  it_service umbrella-lab = $SVC_ID, host $LAB_HOST = $HOST_ID"

log "Umbrella: Zabbix connector"
GRAPH=$(webhook_graph openbao://lab/zabbix severity \
  $'Disaster=critical\nHigh=error\nAverage=warning\nWarning=warning\n*=info' \
  $'source=zabbix\nzabbix_trigger=${trigger_id}\nzabbix_url=${url}' \
  '${trigger_name}' '${host}' 'zabbix:${trigger_id}' use '${status}' '${event_id}' '${value}')
CONN_ID=$(ensure_webhook_connector Zabbix "$LAB_TEAM" "$GRAPH")
umb PUT "/api/connectors/$CONN_ID" "$(jq -nc '{description:"Zabbix 7.0 webhook media type \"Umbrella\" (deploy/zabbix-lab)",
  sample_input:({event_id:"1001",event_value:"1",status:"firing",host:"umbrella-lab-agent",trigger_id:"20001",
  trigger_name:"High CPU utilization",severity:"High",value:"97 %",url:""}|tojson)}')" >/dev/null
log "  connector $CONN_ID, ingest $UMB_INTERNAL_URL/api/ingest/$CONN_ID"

log "Waiting for Zabbix web at $ZBX_URL"
wait_http "$ZBX_URL/" 300 || die "Zabbix web does not answer at $ZBX_URL"
for i in $(seq 1 60); do
  zbx apiinfo.version '{}' >/dev/null 2>&1 && break
  sleep 5
done

log "Zabbix: login"
if ! zbx_login Admin "$ZABBIX_ADMIN_PASSWORD"; then
  zbx_login Admin zabbix || die "cannot log in to Zabbix as Admin"
  ADMIN_ID=$(zbx user.get '{"filter":{"username":"Admin"},"output":["userid"]}' | jq -r '.[0].userid')
  zbx user.update "$(jq -nc --arg id "$ADMIN_ID" --arg p "$ZABBIX_ADMIN_PASSWORD" '{userid:$id,current_passwd:"zabbix",passwd:$p}')" >/dev/null
  log "  default Admin password replaced with ZABBIX_ADMIN_PASSWORD"
  zbx_login Admin "$ZABBIX_ADMIN_PASSWORD" || die "cannot log in with the new password"
fi
ADMIN_ID=$(zbx user.get '{"filter":{"username":"Admin"},"output":["userid"]}' | jq -r '.[0].userid')

log "Zabbix: webhook media type Umbrella"
read -r -d '' SCRIPT <<'JS' || true
var p = JSON.parse(value);
var req = new HttpRequest();
req.addHeader('Content-Type: application/json');
req.addHeader('X-Umbrella-Token: ' + p.token);
var body = {
  event_id: p.event_id,
  event_value: p.event_value,
  status: p.event_value === '0' ? 'resolved' : 'firing',
  host: p.host,
  host_name: p.host_name,
  trigger_id: p.trigger_id,
  trigger_name: p.trigger_name,
  severity: p.severity,
  value: p.value,
  tags: p.tags,
  url: p.zabbix_url
};
var resp = req.post(p.url, JSON.stringify(body));
var code = req.getStatus();
Zabbix.log(4, '[Umbrella] ' + code + ' ' + resp);
if (code < 200 || code >= 300) {
  throw 'Umbrella answered ' + code + ': ' + resp;
}
return 'OK';
JS
ZBX_PUBLIC_URL="http://${PUBLIC_HOST:-localhost}:${ZABBIX_WEB_PORT:-8081}"
MT_PARAMS=$(jq -nc --arg url "$UMB_INTERNAL_URL/api/ingest/$CONN_ID" --arg token "$ZABBIX_WEBHOOK_TOKEN" \
  --arg zurl "$ZBX_PUBLIC_URL/zabbix.php?action=problem.view&triggerids%5B%5D={TRIGGER.ID}" '[
  {name:"url",value:$url},{name:"token",value:$token},
  {name:"event_id",value:"{EVENT.ID}"},{name:"event_value",value:"{EVENT.VALUE}"},
  {name:"host",value:"{HOST.HOST}"},{name:"host_name",value:"{HOST.NAME}"},
  {name:"trigger_id",value:"{TRIGGER.ID}"},{name:"trigger_name",value:"{EVENT.NAME}"},
  {name:"severity",value:"{EVENT.SEVERITY}"},{name:"value",value:"{EVENT.OPDATA}"},
  {name:"tags",value:"{EVENT.TAGS}"},{name:"zabbix_url",value:$zurl}]')
MT_BODY=$(jq -nc --arg script "$SCRIPT" --argjson params "$MT_PARAMS" '{
  name:"Umbrella",type:4,status:0,script:$script,parameters:$params,timeout:"10s",
  maxattempts:3,attempt_interval:"10s",process_tags:0,
  description:"Sends Zabbix problems and recoveries to Umbrella. A non-2xx answer makes Zabbix retry.",
  message_templates:[
    {eventsource:0,recovery:0,subject:"Problem: {EVENT.NAME}",message:"{EVENT.NAME} on {HOST.NAME}"},
    {eventsource:0,recovery:1,subject:"Resolved: {EVENT.NAME}",message:"{EVENT.NAME} on {HOST.NAME} resolved"},
    {eventsource:0,recovery:2,subject:"Updated: {EVENT.NAME}",message:"{EVENT.UPDATE.MESSAGE}"}]}')
MT_ID=$(zbx mediatype.get '{"filter":{"name":"Umbrella"},"output":["mediatypeid"]}' | jq -r '.[0].mediatypeid // empty')
if [ -n "$MT_ID" ]; then
  zbx mediatype.update "$(jq -c --arg id "$MT_ID" '. + {mediatypeid:$id}' <<<"$MT_BODY")" >/dev/null
else
  MT_ID=$(zbx mediatype.create "$MT_BODY" | jq -r '.mediatypeids[0]')
fi
log "  media type $MT_ID"

log "Zabbix: media for Admin"
zbx user.update "$(jq -nc --arg id "$ADMIN_ID" --arg mt "$MT_ID" \
  '{userid:$id,medias:[{mediatypeid:$mt,sendto:"umbrella",active:0,severity:63,period:"1-7,00:00-24:00"}]}')" >/dev/null

log "Zabbix: action Send problems to Umbrella"
ACT_BODY=$(jq -nc --arg uid "$ADMIN_ID" --arg mt "$MT_ID" '{
  name:"Send problems to Umbrella",eventsource:0,status:0,esc_period:"1h",
  operations:[{operationtype:0,opmessage:{default_msg:1,mediatypeid:$mt},opmessage_usr:[{userid:$uid}]}],
  recovery_operations:[{operationtype:11,opmessage:{default_msg:1}}]}')
ACT_ID=$(zbx action.get '{"filter":{"name":"Send problems to Umbrella"},"output":["actionid"]}' | jq -r '.[0].actionid // empty')
if [ -z "$ACT_ID" ]; then
  ACT_ID=$(zbx action.create "$ACT_BODY" | jq -r '.actionids[0]')
fi
log "  action $ACT_ID"

log "Zabbix: host $LAB_HOST (agent 2 container)"
GROUP_ID=$(zbx hostgroup.get '{"filter":{"name":["Linux servers"]},"output":["groupid"]}' | jq -r '.[0].groupid // empty')
[ -n "$GROUP_ID" ] || GROUP_ID=$(zbx hostgroup.create '{"name":"Linux servers"}' | jq -r '.groupids[0]')
TPL_ID=$(zbx template.get '{"filter":{"host":["Linux by Zabbix agent"]},"output":["templateid"]}' | jq -r '.[0].templateid // empty')
HOSTID=$(zbx host.get "$(jq -nc --arg h "$LAB_HOST" '{filter:{host:[$h]},output:["hostid"]}')" | jq -r '.[0].hostid // empty')
if [ -z "$HOSTID" ]; then
  HOSTID=$(zbx host.create "$(jq -nc --arg h "$LAB_HOST" --arg g "$GROUP_ID" --arg t "$TPL_ID" '{
    host:$h,name:$h,groups:[{groupid:$g}],
    templates:(if $t == "" then [] else [{templateid:$t}] end),
    interfaces:[{type:1,main:1,useip:0,ip:"",dns:"zabbix-agent",port:"10050"}],
    tags:[{tag:"umbrella",value:"lab"}]}')" | jq -r '.hostids[0]')
fi
log "  host $HOSTID (template: ${TPL_ID:-none})"

log "Zabbix: test item and trigger"
ITEM_ID=$(zbx item.get "$(jq -nc --arg h "$HOSTID" '{hostids:[$h],filter:{key_:"umbrella.test"},output:["itemid"]}')" | jq -r '.[0].itemid // empty')
if [ -z "$ITEM_ID" ]; then
  ITEM_ID=$(zbx item.create "$(jq -nc --arg h "$HOSTID" '{hostid:$h,name:"Umbrella test value",key_:"umbrella.test",type:2,value_type:3,history:"7d",trends:"0"}')" | jq -r '.itemids[0]')
fi
TRG_ID=$(zbx trigger.get "$(jq -nc --arg h "$HOSTID" '{hostids:[$h],filter:{description:"Umbrella test problem"},output:["triggerid"]}')" | jq -r '.[0].triggerid // empty')
if [ -z "$TRG_ID" ]; then
  TRG_ID=$(zbx trigger.create "$(jq -nc --arg h "$LAB_HOST" '{description:"Umbrella test problem",
    expression:("last(/" + $h + "/umbrella.test)>0"),priority:4,opdata:"value {ITEM.LASTVALUE}",
    manual_close:1,tags:[{tag:"umbrella",value:"test"}]}')" | jq -r '.triggerids[0]')
fi
log "  item $ITEM_ID, trigger $TRG_ID (fire it: ./smoke-test.sh --zabbix)"

cat <<MSG

Done.
  Umbrella:  $UMB_URL   (connector $CONN_ID "Zabbix")
  Zabbix:    $ZBX_URL   (Admin / ZABBIX_ADMIN_PASSWORD from .env)
MSG
