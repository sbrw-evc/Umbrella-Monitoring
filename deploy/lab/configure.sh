#!/usr/bin/env bash
set -euo pipefail
LAB_DIR=$(cd "$(dirname "$0")" && pwd)
cd "$LAB_DIR"
if [ -f .env ] && [ -z "${IN_TOOLBOX:-}" ]; then set -a; . ./.env; set +a; fi
. ./lib.sh

: "${UMBRELLA_ADMIN_PASSWORD:?set in .env}"
: "${ZABBIX_ADMIN_PASSWORD:?set in .env}"
: "${LAB_OWNER_PASSWORD:?set in .env}"
: "${NETBOX_API_TOKEN:?set in .env}"
: "${PROMETHEUS_WEBHOOK_TOKEN:?set in .env}"
: "${UMBRELLA_DB_PASSWORD:?set in .env (run env.sh)}"
UMB_ADMIN=${UMBRELLA_ADMIN_USER:-admin}
UMB_INTERNAL=http://umbrella:8080
SERVICES=(umbrella openbao zabbix-db zabbix-server zabbix-web zabbix-agent prometheus alertmanager node-exporter cadvisor
  kafka telegraf fluentd opensearch opensearch-dashboards netbox netbox-db netbox-redis grafana)
DB_SERVICES=" zabbix-db netbox-db "
CATALOG_SERVICES=" netbox netbox-db netbox-redis "

log "Umbrella at $UMB_URL"
wait_http "$UMB_URL/healthz" 240 || die "Umbrella does not answer at $UMB_URL"
umb_login "$UMB_ADMIN" "$UMBRELLA_ADMIN_PASSWORD" || die "cannot sign in to Umbrella as $UMB_ADMIN (status $(umb_status))"
[ "$(umb GET /api/openbao | jq -r '.token_ok and .mount_ok')" = true ] || die "Umbrella has no working OpenBao: $(umb GET /api/openbao | jq -r .error)"
log "  signed in as $UMB_ADMIN, OpenBao is connected"

if [ "$(http GET "$UMB_URL/api/setup/status" | jq -r .required)" = true ]; then
  log "Umbrella: setup wizard (light theme, Russian, PostgreSQL umbrella-db; the password goes to OpenBao)"
  R=$(umb POST /api/setup/complete "$(jq -nc --arg p "$UMBRELLA_DB_PASSWORD" '{theme:"light",locale:"ru",
    storage:{kind:"postgres",host:"umbrella-db",port:5432,database:"umbrella",user:"umbrella",sslmode:"disable",password:$p,adopt:true}}')")
  [ "$(umb_status)" = 200 ] || die "setup wizard answered $(umb_status): $(jq -r '.error // empty' <<<"$R")"
  log "  state storage: $(jq -r '.storage.where' <<<"$R")"
fi

log "NetBox at $NETBOX_URL"
for i in $(seq 1 120); do
  [ "$(nb GET /api/status/ | jq -r '."netbox-version" // empty' 2>/dev/null)" != "" ] && break
  sleep 5
done
NB_VERSION=$(nb GET /api/status/ | jq -r '."netbox-version" // empty')
[ -n "$NB_VERSION" ] || die "NetBox API does not answer at $NETBOX_URL (status $(umb_status))"
log "  NetBox $NB_VERSION: inventory and owners of the lab"
SITE=$(nb_ensure /api/dcim/sites/ slug=lab-dc '{"name":"Lab DC","slug":"lab-dc","status":"active"}')
T_INFRA=$(nb_ensure /api/tenancy/tenants/ slug=infra '{"name":"Инфраструктура","slug":"infra"}')
T_DBA=$(nb_ensure /api/tenancy/tenants/ slug=dba '{"name":"Базы данных","slug":"dba"}')
T_MON=$(nb_ensure /api/tenancy/tenants/ slug=monitoring '{"name":"Мониторинг","slug":"monitoring"}')
R_SERVER=$(nb_ensure /api/dcim/device-roles/ slug=server '{"name":"Server","slug":"server","color":"2196f3","vm_role":false}')
R_APP=$(nb_ensure /api/dcim/device-roles/ slug=application '{"name":"Application","slug":"application","color":"4caf50","vm_role":true}')
R_DB=$(nb_ensure /api/dcim/device-roles/ slug=database '{"name":"Database","slug":"database","color":"ff9800","vm_role":true}')
MFR=$(nb_ensure /api/dcim/manufacturers/ slug=generic '{"name":"Generic","slug":"generic"}')
DTYPE=$(nb_ensure /api/dcim/device-types/ slug=lab-server "$(jq -nc --argjson m "$MFR" '{manufacturer:$m,model:"Lab server",slug:"lab-server"}')")
DEVICE=$(nb_ensure /api/dcim/devices/ "name=$LAB_HOST" "$(jq -nc --arg n "$LAB_HOST" --argjson r "$R_SERVER" --argjson t "$DTYPE" --argjson s "$SITE" --argjson tn "$T_INFRA" \
  '{name:$n,role:$r,device_type:$t,site:$s,tenant:$tn,status:"active",description:"Docker host of the Umbrella lab"}')")
CTYPE=$(nb_ensure /api/virtualization/cluster-types/ slug=docker '{"name":"Docker","slug":"docker"}')
CLUSTER=$(nb_ensure /api/virtualization/clusters/ name=lab-docker "$(jq -nc --argjson t "$CTYPE" '{name:"lab-docker",type:$t,status:"active"}')")
declare -A VM
for svc in "${SERVICES[@]}"; do
  role=$R_APP tenant=$T_MON
  [[ "$DB_SERVICES" == *" $svc "* ]] && role=$R_DB && tenant=$T_DBA
  [[ "$CATALOG_SERVICES" == *" $svc "* && "$DB_SERVICES" != *" $svc "* ]] && tenant=$T_INFRA
  VM[$svc]=$(nb_ensure /api/virtualization/virtual-machines/ "name=$svc" "$(jq -nc --arg n "$svc" --argjson c "$CLUSTER" --argjson r "$role" --argjson t "$tenant" \
    '{name:$n,cluster:$c,role:$r,tenant:$t,status:"active",description:("compose service " + $n)}')")
done
CROLE=$(nb_ensure /api/tenancy/contact-roles/ slug=owner '{"name":"Owner","slug":"owner"}')
C_INFRA=$(nb_ensure /api/tenancy/contacts/ "email=oleg.ivanov@lab.local" '{"name":"Олег Иванов","email":"oleg.ivanov@lab.local","phone":"+7 900 000-00-01","title":"Инженер инфраструктуры"}')
C_DBA=$(nb_ensure /api/tenancy/contacts/ "email=irina.smirnova@lab.local" '{"name":"Ирина Смирнова","email":"irina.smirnova@lab.local","phone":"+7 900 000-00-02","title":"DBA"}')
C_MON=$(nb_ensure /api/tenancy/contacts/ "email=maria.kozlova@lab.local" '{"name":"Мария Козлова","email":"maria.kozlova@lab.local","phone":"+7 900 000-00-03","title":"Инженер мониторинга"}')
assign() {
  nb_ensure /api/tenancy/contact-assignments/ "object_type=$1&object_id=$2&contact_id=$3" \
    "$(jq -nc --arg t "$1" --argjson o "$2" --argjson c "$3" --argjson r "$CROLE" '{object_type:$t,object_id:$o,contact:$c,role:$r,priority:"primary"}')" >/dev/null
}
assign dcim.device "$DEVICE" "$C_INFRA"
assign virtualization.virtualmachine "${VM[zabbix-db]}" "$C_DBA"
assign virtualization.virtualmachine "${VM[netbox-db]}" "$C_DBA"
assign tenancy.tenant "$T_MON" "$C_MON"
assign tenancy.tenant "$T_INFRA" "$C_INFRA"
log "  device $LAB_HOST, ${#SERVICES[@]} virtual machines, 3 owners"

log "Zabbix at $ZBX_URL"
wait_http "$ZBX_URL/" 300 || die "Zabbix web does not answer at $ZBX_URL"
for i in $(seq 1 60); do zbx apiinfo.version '{}' >/dev/null 2>&1 && break; sleep 5; done
if ! zbx_login Admin "$ZABBIX_ADMIN_PASSWORD"; then
  zbx_login Admin zabbix || die "cannot log in to Zabbix as Admin"
  ADMIN_ID=$(zbx user.get '{"filter":{"username":"Admin"},"output":["userid"]}' | jq -r '.[0].userid')
  zbx user.update "$(jq -nc --arg id "$ADMIN_ID" --arg p "$ZABBIX_ADMIN_PASSWORD" '{userid:$id,current_passwd:"zabbix",passwd:$p}')" >/dev/null
  zbx_login Admin "$ZABBIX_ADMIN_PASSWORD" || die "cannot log in with the new Zabbix password"
  log "  default Admin password replaced"
fi
GROUP_ID=$(zbx hostgroup.get '{"filter":{"name":["Linux servers"]},"output":["groupid"]}' | jq -r '.[0].groupid // empty')
[ -n "$GROUP_ID" ] || GROUP_ID=$(zbx hostgroup.create '{"name":"Linux servers"}' | jq -r '.groupids[0]')
TPL_ID=$(zbx template.get '{"filter":{"host":["Linux by Zabbix agent"]},"output":["templateid"]}' | jq -r '.[0].templateid // empty')
MACROS='[{"macro":"{$CPU.UTIL.CRIT}","value":"3"},{"macro":"{$LOAD_AVG_PER_CPU.MAX.WARN}","value":"0.1"},
  {"macro":"{$MEMORY.UTIL.MAX}","value":"25"},{"macro":"{$VFS.FS.PUSED.MAX.WARN}","value":"5"},{"macro":"{$VFS.FS.PUSED.MAX.CRIT}","value":"10"},
  {"macro":"{$SWAP.PFREE.MIN.WARN}","value":"99"}]'
HOSTID=$(zbx host.get "$(jq -nc --arg h "$LAB_HOST" '{filter:{host:[$h]},output:["hostid"]}')" | jq -r '.[0].hostid // empty')
if [ -z "$HOSTID" ]; then
  HOSTID=$(zbx host.create "$(jq -nc --arg h "$LAB_HOST" --arg g "$GROUP_ID" --arg t "$TPL_ID" --argjson m "$MACROS" '{
    host:$h,name:$h,groups:[{groupid:$g}],templates:(if $t == "" then [] else [{templateid:$t}] end),
    interfaces:[{type:1,main:1,useip:0,ip:"",dns:"zabbix-agent",port:"10050"}],macros:$m,tags:[{tag:"umbrella",value:"lab"}]}')" | jq -r '.hostids[0]')
else
  zbx host.update "$(jq -nc --arg id "$HOSTID" --argjson m "$MACROS" '{hostid:$id,macros:$m}')" >/dev/null
fi
ITEM_ID=$(zbx item.get "$(jq -nc --arg h "$HOSTID" '{hostids:[$h],filter:{key_:"umbrella.test"},output:["itemid"]}')" | jq -r '.[0].itemid // empty')
[ -n "$ITEM_ID" ] || zbx item.create "$(jq -nc --arg h "$HOSTID" '{hostid:$h,name:"Umbrella test value",key_:"umbrella.test",type:2,value_type:3,history:"7d",trends:"0"}')" >/dev/null
TRG=$(zbx trigger.get "$(jq -nc --arg h "$HOSTID" '{hostids:[$h],filter:{description:"Umbrella test problem"},output:["triggerid"]}')" | jq -r '.[0].triggerid // empty')
[ -n "$TRG" ] || zbx trigger.create "$(jq -nc --arg h "$LAB_HOST" '{description:"Umbrella test problem",
  expression:("last(/" + $h + "/umbrella.test)>0"),priority:4,opdata:"value {ITEM.LASTVALUE}",manual_close:1,tags:[{tag:"umbrella",value:"test"}]}')" >/dev/null
log "  host $LAB_HOST ($HOSTID): Linux template with low thresholds (CPU > 3%, memory > 25%, disk > 5%)"

log "OpenSearch Dashboards at $OSD_URL"
for i in $(seq 1 100); do
  [ "$(osd GET /api/status | jq -r '.status.overall.state // empty' 2>/dev/null)" = green ] && break
  sleep 3
done
for idx in pg-logs container-logs; do
  os_api PUT "/_index_template/$idx" "$(jq -nc --arg p "$idx-*" '{index_patterns:[$p],template:{settings:{number_of_shards:1,number_of_replicas:0},
    mappings:{properties:{"@timestamp":{type:"date"},level:{type:"keyword"},host:{type:"keyword"},service:{type:"keyword"},
    error_code:{type:"keyword"},sqlstate:{type:"keyword"},pg_level:{type:"keyword"},duration_ms:{type:"float"},
    container:{type:"keyword"},image:{type:"keyword"},stream:{type:"keyword"},collector:{type:"keyword"},message:{type:"text"}}}}}')" >/dev/null
  [ "$(umb_status)" = 200 ] || warn "index template $idx answered $(umb_status)"
  osd POST "/api/saved_objects/index-pattern/$idx?overwrite=true" "$(jq -nc --arg t "$idx-*" '{attributes:{title:$t,timeFieldName:"@timestamp"}}')" >/dev/null
done
osd POST /api/opensearch-dashboards/settings '{"changes":{"defaultIndex":"container-logs"}}' >/dev/null || true
log "  index templates and patterns pg-logs-*, container-logs-*"

log "Umbrella: Grafana link"
umb PUT /api/settings "$(jq -nc --arg u "http://${PUBLIC_HOST:-localhost}:${GRAFANA_PORT:-3000}/d/umbrella-incidents" '{grafana_url:$u}')" >/dev/null

log "Umbrella: integrations"
ZBX_INT=$(ensure_integration "Zabbix" "$(jq -nc --arg p "$ZABBIX_ADMIN_PASSWORD" --arg team "$LAB_TEAM" --arg u "$UMB_INTERNAL" '{
  type:"zabbix",name:"Zabbix",slug:"zabbix",team:$team,url:"http://zabbix-web:8080",auth_type:"basic",username:"Admin",secret:$p,
  params:{zabbix_user:"Admin",umbrella_url:$u,discovery:"true",discovery_interval:"5m"}}')")
integration_action "$ZBX_INT" setup
integration_action "$ZBX_INT" sync
AM_INT=$(ensure_integration "Prometheus Alertmanager" "$(jq -nc --arg t "$PROMETHEUS_WEBHOOK_TOKEN" --arg team "$LAB_TEAM" --arg u "$UMB_INTERNAL" '{
  type:"alertmanager",name:"Prometheus Alertmanager",slug:"prometheus",team:$team,url:"http://alertmanager:9093",auth_type:"none",
  webhook_token:$t,params:{umbrella_url:$u}}')")
integration_action "$AM_INT" check
PROM_INT=$(ensure_integration "Prometheus" "$(jq -nc --arg team "$LAB_TEAM" '{type:"prometheus",name:"Prometheus",team:$team,
  url:"http://prometheus:9090",auth_type:"none",params:{discovery:"true",discovery_interval:"5m",host_label:"host"}}')")
integration_action "$PROM_INT" sync
NB_INT=$(ensure_integration "NetBox" "$(jq -nc --arg t "$NETBOX_API_TOKEN" --arg team "$LAB_TEAM" '{type:"netbox",name:"NetBox",team:$team,
  url:"http://netbox:8080",auth_type:"token",secret:$t,params:{interval:"10m",objects:"devices,vms",team_from:"tenant",missing:"mark"}}')")
integration_action "$NB_INT" sync
PG_INT=$(ensure_integration "PostgreSQL logs (OpenSearch)" "$(jq -nc '{type:"opensearch",name:"PostgreSQL logs (OpenSearch)",slug:"pg-logs",team:"dba",
  url:"http://opensearch:9200",auth_type:"none",params:{index:"pg-logs-*",interval:"20s",window:"5m",level_field:"level",
  levels:"fatal,error,warning",host_field:"host",service_field:"service",message_field:"message",code_field:"error_code"}}')")
integration_action "$PG_INT" check
CL_INT=$(ensure_integration "Container logs (OpenSearch)" "$(jq -nc --arg team "$LAB_TEAM" '{type:"opensearch",name:"Container logs (OpenSearch)",slug:"container-logs",team:$team,
  url:"http://opensearch:9200",auth_type:"none",params:{index:"container-logs-*",interval:"20s",window:"5m",level_field:"level",
  levels:"fatal,critical,error",host_field:"host",service_field:"service",message_field:"message",code_field:"error_code"}}')")
integration_action "$CL_INT" check

log "Umbrella: RED/USE rules on Prometheus (low thresholds)"
rule() {
  ensure_rule "$1" "$(jq -nc --arg n "$1" --arg m "$2" --arg s "$3" --arg q "$4" --arg l "$5" --arg op "$6" --argjson t "$7" --arg f "$8" \
    --arg sev "$9" --arg title "${10}" --arg src "$PROM_INT" --arg team "$LAB_TEAM" \
    '{name:$n,method:$m,signal:$s,source_id:$src,query:$q,ci_label:$l,op:$op,threshold:$t,for:$f,interval:"30s",severity:$sev,title:$title,team:$team,enabled:true}')" >/dev/null
}
rule "Загрузка CPU хоста" use use.cpu.utilization '100 * (1 - avg by (host) (rate(node_cpu_seconds_total{mode="idle"}[1m])))' host '>' 2 30s warning 'CPU ${ci}: ${value}% (порог ${threshold}%)'
rule "Память хоста" use use.mem.utilization '100 * (1 - avg by (host) (node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes))' host '>' 30 1m warning 'Память ${ci}: ${value}% занято'
rule "Насыщение CPU хоста" use use.saturation 'node_load1 / on (host) count by (host) (node_cpu_seconds_total{mode="idle"})' host '>' 0.1 1m error 'Load на ядро ${ci}: ${value}'
rule "CPU контейнера" use use.container.cpu '100 * sum by (service) (rate(container_cpu_usage_seconds_total{service!=""}[1m]))' service '>' 2 30s warning 'CPU контейнера ${ci}: ${value}%'
rule "Память контейнера" use use.container.memory 'sum by (service) (container_memory_working_set_bytes{service!=""}) / 1048576' service '>' 200 1m warning 'Память контейнера ${ci}: ${value} МБ'
rule "Поток запросов Prometheus" red red.rate 'sum by (host) (rate(prometheus_http_requests_total[5m]))' host '<' 5 1m warning 'Запросов к ${ci}: ${value}/с'
rule "Задержка Prometheus p99" red red.duration 'histogram_quantile(0.99, sum by (host, le) (rate(prometheus_http_request_duration_seconds_bucket[5m])))' host '>' 0.02 1m warning 'p99 ${ci}: ${value} с'
log "  7 rules"

log "Umbrella: CMDB business services"
BS_MON=$(ensure_ci "Мониторинг (стенд)" business_service "$LAB_TEAM" "Бизнес-услуга: мониторинг стенда")
BS_CAT=$(ensure_ci "Каталог (NetBox)" business_service infra "Бизнес-услуга: инвентарь и ответственные")
svc_ci() {
  local id
  id=$(ensure_ci "$1" it_service "$3" "$4")
  ensure_relation "$2" "$id" depends_on
  printf '%s' "$id"
}
IT_METRICS=$(svc_ci "Сбор метрик" "$BS_MON" "$LAB_TEAM" "Zabbix и Prometheus")
IT_LOGS=$(svc_ci "Сбор логов" "$BS_MON" "$LAB_TEAM" "Telegraf → Kafka → Fluentd → OpenSearch")
IT_UMB=$(svc_ci "Umbrella" "$BS_MON" "$LAB_TEAM" "Umbrella и OpenBao")
IT_NB=$(svc_ci "NetBox" "$BS_CAT" infra "NetBox, PostgreSQL, Valkey")
HOST_ID=$(ci_id "$LAB_HOST")
link() {
  local id
  id=$(ci_id "$2")
  [ -n "$id" ] || { warn "CI $2 not found yet (NetBox sync)"; return 0; }
  ensure_relation "$1" "$id" runs_on
  [ -z "$HOST_ID" ] || ensure_relation "$id" "$HOST_ID" runs_on
}
for s in zabbix-db zabbix-server zabbix-web zabbix-agent prometheus alertmanager node-exporter cadvisor; do link "$IT_METRICS" "$s"; done
for s in telegraf kafka fluentd opensearch opensearch-dashboards zabbix-db; do link "$IT_LOGS" "$s"; done
for s in umbrella openbao grafana; do link "$IT_UMB" "$s"; done
for s in netbox netbox-db netbox-redis; do link "$IT_NB" "$s"; done
log "  Мониторинг (стенд) = $BS_MON, Каталог (NetBox) = $BS_CAT"

log "Umbrella: user lab-owner (owner of Мониторинг (стенд))"
OWNER_ID=$(ensure_user lab-owner "$(jq -nc --arg p "$LAB_OWNER_PASSWORD" --arg s "$BS_MON" '{username:"lab-owner",
  name:"Владелец мониторинга стенда",password:$p,roles:["owner"],business_services:[$s],must_change_password:false}')")
ADMIN_ID=$(user_id "$UMB_ADMIN")

log "Umbrella: teams"
team() {
  local body
  body=$(jq -nc --arg id "$1" --arg n "$2" --arg d "$3" --arg e "$4" --argjson m "$5" --argjson l "$6" '{id:$id,name:$n,description:$d,email:$e,members:$m,leads:$l}')
  umb POST /api/teams "$body" >/dev/null
  [ "$(umb_status)" = 201 ] || umb PUT "/api/teams/$1" "$body" >/dev/null
}
team monitoring "Мониторинг" "Umbrella, Zabbix, Prometheus, логи" monitoring@lab.local "$(jq -nc --arg a "$ADMIN_ID" --arg o "$OWNER_ID" '[$a,$o]')" "$(jq -nc --arg a "$ADMIN_ID" '[$a]')"
team infra "Инфраструктура" "Хост стенда и NetBox" infra@lab.local "$(jq -nc --arg a "$ADMIN_ID" '[$a]')" '[]'
team dba "Базы данных" "PostgreSQL Zabbix и NetBox" dba@lab.local "$(jq -nc --arg a "$ADMIN_ID" '[$a]')" '[]'
log "  monitoring, infra, dba"

if [ -n "${UMBRELLA_PD_ROUTING_KEY:-}" ]; then
  log "Umbrella: PagerDuty (keys go to OpenBao)"
  umb PUT /api/pagerduty "$(jq -nc --arg k "$UMBRELLA_PD_ROUTING_KEY" --arg t "${UMBRELLA_PD_API_TOKEN:-}" --arg r "${UMBRELLA_PD_REGION:-us}" \
    '{enabled:true,region:$r,routing_key:$k} + (if $t == "" then {} else {api_token:$t} end)')" >/dev/null
  [ "$(umb_status)" = 200 ] || warn "PagerDuty settings answered $(umb_status)"
fi
if [ -n "${TEAMS_WEBHOOK_URL:-}" ]; then
  ensure_channel "Teams" "$(jq -nc --arg u "$TEAMS_WEBHOOK_URL" '{name:"Teams",type:"teams",url:$u,mode:"always",min_severity:"error",
    events:["open","escalate","ack","resolve","fallback"],services:[],enabled:true}')" >/dev/null
  log "Umbrella: channel Teams"
fi
if [ -n "${ZOOM_WEBHOOK_URL:-}" ]; then
  ensure_channel "Zoom" "$(jq -nc --arg u "$ZOOM_WEBHOOK_URL" --arg t "${ZOOM_VERIFICATION_TOKEN:-}" '{name:"Zoom",type:"zoom",url:$u,token:$t,
    mode:"fallback",min_severity:"error",events:["fallback","resolve"],services:[],enabled:true}')" >/dev/null
  log "Umbrella: channel Zoom"
fi

cat <<MSG

Done. The lab is connected through the Umbrella API; secrets are in OpenBao.
  Integrations: Zabbix $ZBX_INT, Alertmanager $AM_INT, Prometheus $PROM_INT, NetBox $NB_INT, logs $PG_INT and $CL_INT
  Hosts from Zabbix, Prometheus and NetBox are merged into CMDB entries (Umbrella → Конфигурационные единицы).
MSG
