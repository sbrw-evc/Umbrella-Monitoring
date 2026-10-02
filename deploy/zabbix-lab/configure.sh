#!/usr/bin/env bash
# Connects the lab sources to Umbrella. Safe to run again: it finds what it
# created before and updates it. Umbrella MVP keeps its data in memory, so run
# this again after the umbrella container restarts.
#
#   ./configure.sh            # reads .env next to this script
#
# Umbrella (signed in as UMBRELLA_ADMIN_USER):
#   CMDB: business services lab-shop and lab-billing, their IT services and hosts;
#   user lab-owner (role owner, bound to lab-shop) to show service scope;
#   connectors (published and running), ingest URL /api/ingest/<slug>:
#     zabbix      Zabbix webhook media type
#     prometheus  Alertmanager webhook (alerts[] with labels, annotations, status, fingerprint)
#     opensearch  pull: every 15 s searches lab-logs for recent error entries
#   notification channels "Lab Teams" and "Lab Zoom" (the notify-sink
#   container, or real webhooks from TEAMS_WEBHOOK_URL / ZOOM_WEBHOOK_URL).
# Zabbix: Admin password from .env, the "Umbrella" webhook media type, media
#   for Admin, the "Send problems to Umbrella" action, the lab agent host with
#   the Linux template and a trapper item with a trigger the test can fire.
# OpenSearch: index lab-logs with its mapping; OpenSearch Dashboards index
#   pattern lab-logs*.
set -euo pipefail
cd "$(dirname "$0")"
[ -f .env ] && set -a && . ./.env && set +a
. ./lib.sh

: "${UMBRELLA_ADMIN_PASSWORD:?set in .env}"
: "${ZABBIX_WEBHOOK_TOKEN:?set in .env}"
: "${ZABBIX_ADMIN_PASSWORD:?set in .env}"
: "${LAB_OWNER_PASSWORD:?set in .env (run install.sh to add the new settings)}"
UMB_ADMIN=${UMBRELLA_ADMIN_USER:-admin}
# Zabbix server and Alertmanager reach Umbrella over the compose network;
# Umbrella reaches OpenSearch and the notification sink the same way.
UMB_INTERNAL_URL=${UMB_INTERNAL_URL:-http://umbrella:8080}
OS_INTERNAL_URL=${OS_INTERNAL_URL:-http://opensearch:9200}
SINK_URL=${SINK_URL:-http://notify-sink:8080}
LAB_HOST=umbrella-lab-agent
LAB_TEAM=${LAB_TEAM:-monitoring}

log "Waiting for Umbrella at $UMB_URL"
wait_http "$UMB_URL/healthz" 180 || die "Umbrella does not answer at $UMB_URL"
umb_login "$UMB_ADMIN" "$UMBRELLA_ADMIN_PASSWORD" ||
  die "cannot sign in to Umbrella as $UMB_ADMIN: check UMBRELLA_ADMIN_PASSWORD in .env (status $(umb_status))"
log "Umbrella: signed in as $UMB_ADMIN"

log "Umbrella: CMDB entries for the lab"
SHOP_ID=$(ensure_ci lab-shop business_service shop)
SVC_ID=$(ensure_ci umbrella-lab it_service "$LAB_TEAM" "$SHOP_ID")
HOST_ID=$(ensure_ci "$LAB_HOST" host "$LAB_TEAM" "$SVC_ID")
NODE_ID=$(ensure_ci umbrella-lab-node host "$LAB_TEAM" "$SVC_ID")
API_ID=$(ensure_ci lab-shop-api it_service shop "$SHOP_ID")
SHOP_HOST_ID=$(ensure_ci shop-api-1 host shop "$API_ID")
BILL_ID=$(ensure_ci lab-billing business_service billing)
BILL_API_ID=$(ensure_ci lab-billing-api it_service billing "$BILL_ID")
BILL_HOST_ID=$(ensure_ci billing-api-1 host billing "$BILL_API_ID")
log "  business_service lab-shop = $SHOP_ID: umbrella-lab ($LAB_HOST, umbrella-lab-node), lab-shop-api (shop-api-1)"
log "  business_service lab-billing = $BILL_ID: lab-billing-api (billing-api-1)"

log "Umbrella: user lab-owner (role owner, service lab-shop)"
OWNER_ID=$(ensure_user lab-owner "$(jq -nc --arg p "$LAB_OWNER_PASSWORD" --arg s "$SHOP_ID" '{username:"lab-owner",
  name:"Владелец lab-shop",password:$p,roles:["owner"],business_services:[$s],must_change_password:false}')")
log "  user $OWNER_ID, password LAB_OWNER_PASSWORD in .env"

log "Umbrella: Zabbix connector (slug zabbix)"
GRAPH=$(webhook_graph openbao://lab/zabbix severity \
  $'Disaster=critical\nHigh=error\nAverage=warning\nWarning=warning\n*=info' \
  $'source=zabbix\nzabbix_trigger=${trigger_id}\nzabbix_url=${url}' \
  '${trigger_name}' '${host}' 'zabbix:${trigger_id}' use '${status}' '${event_id}' '${value}')
CONN_ID=$(ensure_connector Zabbix zabbix "$LAB_TEAM" webhook-json "$GRAPH")
umb PUT "/api/connectors/$CONN_ID" "$(jq -nc '{description:"Zabbix 7.0 webhook media type \"Umbrella\" (deploy/zabbix-lab)",
  sample_input:({event_id:"1001",event_value:"1",status:"firing",host:"umbrella-lab-agent",trigger_id:"20001",
  trigger_name:"High CPU utilization",severity:"High",value:"97 %",url:""}|tojson)}')" >/dev/null
log "  connector $CONN_ID, ingest $UMB_INTERNAL_URL/api/ingest/zabbix"

log "Umbrella: Prometheus connector (slug prometheus, Alertmanager webhook)"
GRAPH=$(webhook_graph openbao://lab/prometheus labels.severity \
  $'critical=critical\nerror=error\nwarning=warning\ninfo=info\npage=critical\n*=warning' \
  $'source=prometheus\nalertname=${labels.alertname}\nprometheus_job=${labels.job}' \
  '${annotations.summary|$labels.alertname}' '${labels.host|$labels.instance}' 'prometheus:${labels.alertname}' \
  '${labels.method|other}' '${status}' '${fingerprint}' '${annotations.value}' alerts)
PROM_CONN_ID=$(ensure_connector Prometheus prometheus "$LAB_TEAM" webhook-json "$GRAPH")
umb PUT "/api/connectors/$PROM_CONN_ID" "$(jq -nc '{description:"Alertmanager webhook_configs -> /api/ingest/prometheus, Bearer PROMETHEUS_WEBHOOK_TOKEN (deploy/zabbix-lab)",
  sample_input:({version:"4",status:"firing",receiver:"umbrella",groupKey:"{}:{alertname=\"UmbrellaLabTestFailure\"}",
  alerts:[{status:"firing",fingerprint:"5f2b4c1d9e0a7b36",startsAt:"2026-01-01T10:00:00Z",endsAt:"0001-01-01T00:00:00Z",
  labels:{alertname:"UmbrellaLabTestFailure",host:"umbrella-lab-node",instance:"node-exporter:9100",job:"node",severity:"error",method:"other"},
  annotations:{summary:"Lab test failure on umbrella-lab-node",value:"1"}}]}|tojson)}')" >/dev/null
log "  connector $PROM_CONN_ID, Alertmanager posts to $UMB_INTERNAL_URL/api/ingest/prometheus"

log "Umbrella: OpenSearch connector (slug opensearch, pull every 15 s)"
OS_QUERY='{"size":100,"sort":[{"@timestamp":"desc"}],"query":{"bool":{"filter":[{"terms":{"level":["error","critical","fatal"]}},{"range":{"@timestamp":{"gte":"now-15m"}}}]}}}'
GRAPH=$(pull_graph 15s "$OS_INTERNAL_URL/lab-logs/_search?ignore_unavailable=true" POST "$OS_QUERY" hits.hits _source.level \
  $'fatal=critical\ncritical=critical\nerror=error\n*=warning' \
  $'source=opensearch\nservice=${_source.service}\nopensearch_index=${_index}' \
  '${_source.message}' '${_source.host}' 'opensearch:${_source.service|app}:${_source.error_code|error}' red firing '${_id}' '${_source.error_code}')
OS_CONN_ID=$(ensure_connector OpenSearch opensearch shop pull-http "$GRAPH")
umb PUT "/api/connectors/$OS_CONN_ID" "$(jq -nc '{description:"Polls OpenSearch index lab-logs for error entries of the last 15 minutes (deploy/zabbix-lab)",
  sample_input:({hits:{total:{value:1},hits:[{_index:"lab-logs",_id:"Kq3x",_source:{"@timestamp":"2026-01-01T10:00:00Z",
  level:"error",host:"shop-api-1",service:"lab-shop-api",message:"Payment gateway timeout",error_code:"PAY-504"}}]}}|tojson)}')" >/dev/null
log "  connector $OS_CONN_ID, POST $OS_INTERNAL_URL/lab-logs/_search"

log "Umbrella: notification channels Lab Teams and Lab Zoom"
TEAMS_URL=${TEAMS_WEBHOOK_URL:-$SINK_URL/teams}
ZOOM_URL=${ZOOM_WEBHOOK_URL:-$SINK_URL/zoom}
TEAMS_CH=$(ensure_channel "Lab Teams" "$(jq -nc --arg u "$TEAMS_URL" '{name:"Lab Teams",type:"teams",url:$u,mode:"always",
  min_severity:"warning",events:["open","escalate","ack","resolve","fallback"],services:[],enabled:true}')")
ZOOM_CH=$(ensure_channel "Lab Zoom" "$(jq -nc --arg u "$ZOOM_URL" --arg t "${ZOOM_VERIFICATION_TOKEN:-${LAB_ZOOM_TOKEN:-}}" --arg s "$SHOP_ID" \
  '{name:"Lab Zoom",type:"zoom",url:$u,token:$t,mode:"always",min_severity:"error",
  events:["open","escalate","resolve","fallback"],services:[$s],enabled:true}')")
log "  Teams $TEAMS_CH -> ${TEAMS_WEBHOOK_URL:+real webhook}${TEAMS_WEBHOOK_URL:-$TEAMS_URL}"
log "  Zoom $ZOOM_CH (service lab-shop) -> ${ZOOM_WEBHOOK_URL:+real webhook}${ZOOM_WEBHOOK_URL:-$ZOOM_URL}"

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
MT_PARAMS=$(jq -nc --arg url "$UMB_INTERNAL_URL/api/ingest/zabbix" --arg token "$ZABBIX_WEBHOOK_TOKEN" \
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


log "Waiting for OpenSearch Dashboards at $OSD_URL"
for i in $(seq 1 100); do
  [ "$(osd GET /api/status | jq -r '.status.overall.state // empty' 2>/dev/null)" = green ] && break
  sleep 3
done
[ "$(osd GET /api/status | jq -r '.status.overall.state // empty' 2>/dev/null)" = green ] ||
  die "OpenSearch Dashboards is not green at $OSD_URL"

log "OpenSearch: index lab-logs"
os_api GET /lab-logs >/dev/null || true
if [ "$(umb_status)" = 404 ]; then
  os_api PUT /lab-logs '{"settings":{"number_of_shards":1,"number_of_replicas":0},
    "mappings":{"properties":{"@timestamp":{"type":"date"},"level":{"type":"keyword"},"host":{"type":"keyword"},
    "service":{"type":"keyword"},"error_code":{"type":"keyword"},"message":{"type":"text"}}}}' >/dev/null
  [ "$(umb_status)" = 200 ] || die "OpenSearch: create lab-logs answered $(umb_status)"
  log "  created"
else
  log "  exists"
fi

log "OpenSearch Dashboards: index pattern lab-logs*"
osd POST '/api/saved_objects/index-pattern/lab-logs?overwrite=true' \
  '{"attributes":{"title":"lab-logs*","timeFieldName":"@timestamp"}}' >/dev/null
[ "$(umb_status)" = 200 ] || die "OpenSearch Dashboards: index pattern answered $(umb_status)"
osd POST /api/opensearch-dashboards/settings '{"changes":{"defaultIndex":"lab-logs"}}' >/dev/null || true

cat <<MSG

Done.
  Umbrella:    $UMB_URL   ($UMB_ADMIN / UMBRELLA_ADMIN_PASSWORD, lab-owner / LAB_OWNER_PASSWORD from .env)
    connectors: zabbix $CONN_ID, prometheus $PROM_CONN_ID, opensearch $OS_CONN_ID; channels $TEAMS_CH, $ZOOM_CH
  Zabbix:      $ZBX_URL   (Admin / ZABBIX_ADMIN_PASSWORD from .env)
  Dashboards:  $OSD_URL   (index pattern lab-logs*)
  Grafana:     $GRAFANA_URL   (admin / GRAFANA_ADMIN_PASSWORD from .env)
  Prometheus:  $PROM_URL
MSG
