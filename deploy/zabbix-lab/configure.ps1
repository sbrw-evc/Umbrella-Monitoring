<#
.SYNOPSIS
  Connects the lab sources to Umbrella (PowerShell version of configure.sh).
.DESCRIPTION
  Safe to run again: finds what it created before and updates it. Umbrella MVP
  keeps its data in memory, so run it again after the umbrella container restarts.

  Umbrella (signed in as UMBRELLA_ADMIN_USER):
    CMDB: business services lab-shop and lab-billing, their IT services and hosts;
    user lab-owner (role owner, bound to lab-shop) to show service scope;
    connectors (published and running), ingest URL /api/ingest/<slug>:
      zabbix      Zabbix webhook media type
      prometheus  Alertmanager webhook (alerts[] with labels, annotations, status, fingerprint)
      opensearch  pull: every 15 s searches lab-logs for recent error entries
    notification channels "Lab Teams" and "Lab Zoom" (the notify-sink container,
    or real webhooks from TEAMS_WEBHOOK_URL / ZOOM_WEBHOOK_URL).
  Zabbix: Admin password from .env, the "Umbrella" webhook media type, media for
  Admin, the "Send problems to Umbrella" action, the lab agent host with the
  Linux template and a trapper item with a trigger the test can fire.
  OpenSearch: index lab-logs with its mapping; OpenSearch Dashboards index
  pattern lab-logs*.
.EXAMPLE
  .\configure.ps1
#>
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'lib.ps1')
Initialize-Lab $PSScriptRoot

$umbAdmin = Get-Setting 'UMBRELLA_ADMIN_USER' 'admin'
$umbPassword = Get-Setting 'UMBRELLA_ADMIN_PASSWORD'
$ownerPassword = Get-Setting 'LAB_OWNER_PASSWORD'
$webhookToken = Get-Setting 'ZABBIX_WEBHOOK_TOKEN'
$adminPassword = Get-Setting 'ZABBIX_ADMIN_PASSWORD'
if (-not $umbPassword -or -not $webhookToken -or -not $adminPassword) { Stop-Lab 'set UMBRELLA_ADMIN_PASSWORD, ZABBIX_WEBHOOK_TOKEN and ZABBIX_ADMIN_PASSWORD in .env' }
if (-not $ownerPassword) { Stop-Lab 'set LAB_OWNER_PASSWORD in .env (run install.ps1 to add the new settings)' }
# Zabbix server and Alertmanager reach Umbrella over the compose network;
# Umbrella reaches OpenSearch and the notification sink the same way.
$umbInternal = Get-Setting 'UMB_INTERNAL_URL' 'http://umbrella:8080'
$osInternal = Get-Setting 'OS_INTERNAL_URL' 'http://opensearch:9200'
$sinkUrl = Get-Setting 'SINK_URL' 'http://notify-sink:8080'
$labHost = 'umbrella-lab-agent'
$labTeam = Get-Setting 'LAB_TEAM' 'monitoring'

Write-Step "Waiting for Umbrella at $UmbUrl"
if (-not (Wait-Http "$UmbUrl/healthz" 180)) { Stop-Lab "Umbrella does not answer at $UmbUrl" }
if (-not (Connect-Umbrella $umbAdmin $umbPassword)) {
  Stop-Lab "cannot sign in to Umbrella as ${umbAdmin}: check UMBRELLA_ADMIN_PASSWORD in .env (status $($script:LastStatus))"
}
Write-Step "Umbrella: signed in as $umbAdmin"

Write-Step 'Umbrella: CMDB entries for the lab'
$shopId = Get-LabCi 'lab-shop' 'business_service' 'shop'
$svcId = Get-LabCi 'umbrella-lab' 'it_service' $labTeam $shopId
$hostCi = Get-LabCi $labHost 'host' $labTeam $svcId
$nodeCi = Get-LabCi 'umbrella-lab-node' 'host' $labTeam $svcId
$apiId = Get-LabCi 'lab-shop-api' 'it_service' 'shop' $shopId
$shopHost = Get-LabCi 'shop-api-1' 'host' 'shop' $apiId
$billId = Get-LabCi 'lab-billing' 'business_service' 'billing'
$billApi = Get-LabCi 'lab-billing-api' 'it_service' 'billing' $billId
$billHost = Get-LabCi 'billing-api-1' 'host' 'billing' $billApi
Write-Step "  business_service lab-shop = ${shopId}: umbrella-lab ($labHost, umbrella-lab-node), lab-shop-api (shop-api-1)"
Write-Step "  business_service lab-billing = ${billId}: lab-billing-api (billing-api-1)"

Write-Step 'Umbrella: user lab-owner (role owner, service lab-shop)'
$ownerId = Set-LabUser @{ username = 'lab-owner'; name = 'lab-shop owner'; password = $ownerPassword; roles = @('owner')
  business_services = @($shopId); must_change_password = $false }
Write-Step "  user $ownerId, password LAB_OWNER_PASSWORD in .env"

Write-Step 'Umbrella: Zabbix connector (slug zabbix)'
$graph = New-WebhookGraph 'openbao://lab/zabbix' 'severity' "Disaster=critical`nHigh=error`nAverage=warning`nWarning=warning`n*=info" `
  "source=zabbix`nzabbix_trigger=`${trigger_id}`nzabbix_url=`${url}" @{
  title = '${trigger_name}'; ci = '${host}'; signal = 'zabbix:${trigger_id}'; method = 'use'
  status = '${status}'; external_id = '${event_id}'; value = '${value}'
}
$connId = Set-Connector 'Zabbix' 'zabbix' $labTeam 'webhook-json' $graph
$sample = ConvertTo-Json -Compress -InputObject @{ event_id = '1001'; event_value = '1'; status = 'firing'; host = $labHost
  trigger_id = '20001'; trigger_name = 'High CPU utilization'; severity = 'High'; value = '97 %'; url = '' }
[void](Umb PUT "/api/connectors/$connId" @{ description = 'Zabbix 7.0 webhook media type "Umbrella" (deploy/zabbix-lab)'; sample_input = $sample })
Write-Step "  connector $connId, ingest $umbInternal/api/ingest/zabbix"

Write-Step 'Umbrella: Prometheus connector (slug prometheus, Alertmanager webhook)'
$graph = New-WebhookGraph 'openbao://lab/prometheus' 'labels.severity' "critical=critical`nerror=error`nwarning=warning`ninfo=info`npage=critical`n*=warning" `
  "source=prometheus`nalertname=`${labels.alertname}`nprometheus_job=`${labels.job}" @{
  title = '${annotations.summary|$labels.alertname}'; ci = '${labels.host|$labels.instance}'; signal = 'prometheus:${labels.alertname}'
  method = '${labels.method|other}'; status = '${status}'; external_id = '${fingerprint}'; value = '${annotations.value}'
} 'alerts'
$promConnId = Set-Connector 'Prometheus' 'prometheus' $labTeam 'webhook-json' $graph
$sample = ConvertTo-Json -Compress -Depth 8 -InputObject @{ version = '4'; status = 'firing'; receiver = 'umbrella'
  groupKey = '{}:{alertname="UmbrellaLabTestFailure"}'
  alerts = @(@{ status = 'firing'; fingerprint = '5f2b4c1d9e0a7b36'; startsAt = '2026-01-01T10:00:00Z'; endsAt = '0001-01-01T00:00:00Z'
      labels = @{ alertname = 'UmbrellaLabTestFailure'; host = 'umbrella-lab-node'; instance = 'node-exporter:9100'; job = 'node'; severity = 'error'; method = 'other' }
      annotations = @{ summary = 'Lab test failure on umbrella-lab-node'; value = '1' } }) }
[void](Umb PUT "/api/connectors/$promConnId" @{ description = 'Alertmanager webhook_configs -> /api/ingest/prometheus, Bearer PROMETHEUS_WEBHOOK_TOKEN (deploy/zabbix-lab)'; sample_input = $sample })
Write-Step "  connector $promConnId, Alertmanager posts to $umbInternal/api/ingest/prometheus"

Write-Step 'Umbrella: OpenSearch connector (slug opensearch, pull every 15 s)'
$osQuery = '{"size":100,"sort":[{"@timestamp":"desc"}],"query":{"bool":{"filter":[{"terms":{"level":["error","critical","fatal"]}},{"range":{"@timestamp":{"gte":"now-15m"}}}]}}}'
$graph = New-PullGraph '15s' @{ url = "$osInternal/lab-logs/_search?ignore_unavailable=true"; method = 'POST'; body = $osQuery } 'hits.hits' '_source.level' `
  "fatal=critical`ncritical=critical`nerror=error`n*=warning" "source=opensearch`nservice=`${_source.service}`nopensearch_index=`${_index}" @{
  title = '${_source.message}'; ci = '${_source.host}'; signal = 'opensearch:${_source.service|app}:${_source.error_code|error}'
  method = 'red'; status = 'firing'; external_id = '${_id}'; value = '${_source.error_code}'
}
$osConnId = Set-Connector 'OpenSearch' 'opensearch' 'shop' 'pull-http' $graph
$sample = ConvertTo-Json -Compress -Depth 8 -InputObject @{ hits = @{ total = @{ value = 1 }; hits = @(@{ _index = 'lab-logs'; _id = 'Kq3x'
        _source = @{ '@timestamp' = '2026-01-01T10:00:00Z'; level = 'error'; host = 'shop-api-1'; service = 'lab-shop-api'; message = 'Payment gateway timeout'; error_code = 'PAY-504' } }) } }
[void](Umb PUT "/api/connectors/$osConnId" @{ description = 'Polls OpenSearch index lab-logs for error entries of the last 15 minutes (deploy/zabbix-lab)'; sample_input = $sample })
Write-Step "  connector $osConnId, POST $osInternal/lab-logs/_search"

Write-Step 'Umbrella: notification channels Lab Teams and Lab Zoom'
$teamsUrl = Get-Setting 'TEAMS_WEBHOOK_URL' "$sinkUrl/teams"
$zoomUrl = Get-Setting 'ZOOM_WEBHOOK_URL' "$sinkUrl/zoom"
$zoomToken = Get-Setting 'ZOOM_VERIFICATION_TOKEN' (Get-Setting 'LAB_ZOOM_TOKEN')
$teamsCh = Set-LabChannel @{ name = 'Lab Teams'; type = 'teams'; url = $teamsUrl; mode = 'always'; min_severity = 'warning'
  events = @('open', 'escalate', 'ack', 'resolve', 'fallback'); services = @(); enabled = $true }
$zoomCh = Set-LabChannel @{ name = 'Lab Zoom'; type = 'zoom'; url = $zoomUrl; token = $zoomToken; mode = 'always'; min_severity = 'error'
  events = @('open', 'escalate', 'resolve', 'fallback'); services = @($shopId); enabled = $true }
$teamsTarget = $teamsUrl; if (Get-Setting 'TEAMS_WEBHOOK_URL') { $teamsTarget = 'real webhook' }
$zoomTarget = $zoomUrl; if (Get-Setting 'ZOOM_WEBHOOK_URL') { $zoomTarget = 'real webhook' }
Write-Step "  Teams $teamsCh -> $teamsTarget"
Write-Step "  Zoom $zoomCh (service lab-shop) -> $zoomTarget"

Write-Step "Waiting for Zabbix web at $ZbxUrl"
if (-not (Wait-Http "$ZbxUrl/" 300)) { Stop-Lab "Zabbix web does not answer at $ZbxUrl" }
for ($i = 0; $i -lt 60; $i++) {
  try { [void](Zbx 'apiinfo.version' @{}); break } catch { Start-Sleep -Seconds 5 }
}

Write-Step 'Zabbix: login'
if (-not (Connect-Zabbix 'Admin' $adminPassword)) {
  if (-not (Connect-Zabbix 'Admin' 'zabbix')) { Stop-Lab 'cannot log in to Zabbix as Admin' }
  $adminId = (First (Zbx 'user.get' @{ filter = @{ username = 'Admin' }; output = @('userid') })).userid
  [void](Zbx 'user.update' @{ userid = $adminId; current_passwd = 'zabbix'; passwd = $adminPassword })
  Write-Step '  default Admin password replaced with ZABBIX_ADMIN_PASSWORD'
  if (-not (Connect-Zabbix 'Admin' $adminPassword)) { Stop-Lab 'cannot log in with the new password' }
}
$adminId = (First (Zbx 'user.get' @{ filter = @{ username = 'Admin' }; output = @('userid') })).userid

Write-Step 'Zabbix: webhook media type Umbrella'
$jsCode = @'
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
'@
$zbxPublic = 'http://' + (Get-Setting 'PUBLIC_HOST' 'localhost') + ':' + (Get-Setting 'ZABBIX_WEB_PORT' '8081')
$params = @(
  @{ name = 'url'; value = "$umbInternal/api/ingest/zabbix" }, @{ name = 'token'; value = $webhookToken },
  @{ name = 'event_id'; value = '{EVENT.ID}' }, @{ name = 'event_value'; value = '{EVENT.VALUE}' },
  @{ name = 'host'; value = '{HOST.HOST}' }, @{ name = 'host_name'; value = '{HOST.NAME}' },
  @{ name = 'trigger_id'; value = '{TRIGGER.ID}' }, @{ name = 'trigger_name'; value = '{EVENT.NAME}' },
  @{ name = 'severity'; value = '{EVENT.SEVERITY}' }, @{ name = 'value'; value = '{EVENT.OPDATA}' },
  @{ name = 'tags'; value = '{EVENT.TAGS}' },
  @{ name = 'zabbix_url'; value = "$zbxPublic/zabbix.php?action=problem.view&triggerids%5B%5D={TRIGGER.ID}" }
)
$mt = @{
  name = 'Umbrella'; type = 4; status = 0; script = $jsCode; parameters = $params; timeout = '10s'
  maxattempts = 3; attempt_interval = '10s'; process_tags = 0
  description = 'Sends Zabbix problems and recoveries to Umbrella. A non-2xx answer makes Zabbix retry.'
  message_templates = @(
    @{ eventsource = 0; recovery = 0; subject = 'Problem: {EVENT.NAME}'; message = '{EVENT.NAME} on {HOST.NAME}' },
    @{ eventsource = 0; recovery = 1; subject = 'Resolved: {EVENT.NAME}'; message = '{EVENT.NAME} on {HOST.NAME} resolved' },
    @{ eventsource = 0; recovery = 2; subject = 'Updated: {EVENT.NAME}'; message = '{EVENT.UPDATE.MESSAGE}' }
  )
}
$mtId = (First (Zbx 'mediatype.get' @{ filter = @{ name = 'Umbrella' }; output = @('mediatypeid') })).mediatypeid
if ($mtId) {
  $mt['mediatypeid'] = $mtId
  [void](Zbx 'mediatype.update' $mt)
} else {
  $mtId = (Zbx 'mediatype.create' $mt).mediatypeids[0]
}
Write-Step "  media type $mtId"

Write-Step 'Zabbix: media for Admin'
[void](Zbx 'user.update' @{ userid = $adminId; medias = @(@{ mediatypeid = $mtId; sendto = 'umbrella'; active = 0; severity = 63; period = '1-7,00:00-24:00' }) })

Write-Step 'Zabbix: action Send problems to Umbrella'
$actId = (First (Zbx 'action.get' @{ filter = @{ name = 'Send problems to Umbrella' }; output = @('actionid') })).actionid
if (-not $actId) {
  $actId = (Zbx 'action.create' @{
      name = 'Send problems to Umbrella'; eventsource = 0; status = 0; esc_period = '1h'
      operations = @(@{ operationtype = 0; opmessage = @{ default_msg = 1; mediatypeid = $mtId }; opmessage_usr = @(@{ userid = $adminId }) })
      recovery_operations = @(@{ operationtype = 11; opmessage = @{ default_msg = 1 } })
    }).actionids[0]
}
Write-Step "  action $actId"

Write-Step "Zabbix: host $labHost (agent 2 container)"
$groupId = (First (Zbx 'hostgroup.get' @{ filter = @{ name = @('Linux servers') }; output = @('groupid') })).groupid
if (-not $groupId) { $groupId = (Zbx 'hostgroup.create' @{ name = 'Linux servers' }).groupids[0] }
$tplId = (First (Zbx 'template.get' @{ filter = @{ host = @('Linux by Zabbix agent') }; output = @('templateid') })).templateid
$hostId = (First (Zbx 'host.get' @{ filter = @{ host = @($labHost) }; output = @('hostid') })).hostid
if (-not $hostId) {
  $templates = @()
  if ($tplId) { $templates = @(@{ templateid = $tplId }) }
  $hostId = (Zbx 'host.create' @{
      host = $labHost; name = $labHost; groups = @(@{ groupid = $groupId }); templates = $templates
      interfaces = @(@{ type = 1; main = 1; useip = 0; ip = ''; dns = 'zabbix-agent'; port = '10050' })
      tags = @(@{ tag = 'umbrella'; value = 'lab' })
    }).hostids[0]
}
Write-Step "  host $hostId (template: $tplId)"

Write-Step 'Zabbix: test item and trigger'
$itemId = (First (Zbx 'item.get' @{ hostids = @($hostId); filter = @{ key_ = 'umbrella.test' }; output = @('itemid') })).itemid
if (-not $itemId) {
  $itemId = (Zbx 'item.create' @{ hostid = $hostId; name = 'Umbrella test value'; key_ = 'umbrella.test'; type = 2; value_type = 3; history = '7d'; trends = '0' }).itemids[0]
}
$trgId = (First (Zbx 'trigger.get' @{ hostids = @($hostId); filter = @{ description = 'Umbrella test problem' }; output = @('triggerid') })).triggerid
if (-not $trgId) {
  $trgId = (Zbx 'trigger.create' @{ description = 'Umbrella test problem'; expression = "last(/$labHost/umbrella.test)>0"; priority = 4
      opdata = 'value {ITEM.LASTVALUE}'; manual_close = 1; tags = @(@{ tag = 'umbrella'; value = 'test' }) }).triggerids[0]
}
Write-Step "  item $itemId, trigger $trgId (fire it: .\smoke-test.ps1 -Zabbix)"


Write-Step "Waiting for OpenSearch Dashboards at $OsdUrl"
$green = $false
for ($i = 0; $i -lt 100; $i++) {
  $st = Osd GET '/api/status'
  if ($st -and $st.status.overall.state -eq 'green') { $green = $true; break }
  Start-Sleep -Seconds 3
}
if (-not $green) { Stop-Lab "OpenSearch Dashboards is not green at $OsdUrl" }

Write-Step 'OpenSearch: index lab-logs'
[void](Invoke-OpenSearch GET '/lab-logs')
if ($script:LastStatus -eq 404) {
  [void](Invoke-OpenSearch PUT '/lab-logs' @{ settings = @{ number_of_shards = 1; number_of_replicas = 0 }
      mappings = @{ properties = @{ '@timestamp' = @{ type = 'date' }; level = @{ type = 'keyword' }; host = @{ type = 'keyword' }
          service = @{ type = 'keyword' }; error_code = @{ type = 'keyword' }; message = @{ type = 'text' } } } })
  if ($script:LastStatus -ne 200) { Stop-Lab "OpenSearch: create lab-logs answered $($script:LastStatus)" }
  Write-Step '  created'
} else {
  Write-Step '  exists'
}

Write-Step 'OpenSearch Dashboards: index pattern lab-logs*'
[void](Osd POST '/api/saved_objects/index-pattern/lab-logs?overwrite=true' @{ attributes = @{ title = 'lab-logs*'; timeFieldName = '@timestamp' } })
if ($script:LastStatus -ne 200) { Stop-Lab "OpenSearch Dashboards: index pattern answered $($script:LastStatus)" }
[void](Osd POST '/api/opensearch-dashboards/settings' @{ changes = @{ defaultIndex = 'lab-logs' } })

Write-Host ''
Write-Host 'Done.'
Write-Host "  Umbrella:    $UmbUrl   ($umbAdmin / UMBRELLA_ADMIN_PASSWORD, lab-owner / LAB_OWNER_PASSWORD from .env)"
Write-Host "    connectors: zabbix $connId, prometheus $promConnId, opensearch $osConnId; channels $teamsCh, $zoomCh"
Write-Host "  Zabbix:      $ZbxUrl   (Admin / ZABBIX_ADMIN_PASSWORD from .env)"
Write-Host "  Dashboards:  $OsdUrl   (index pattern lab-logs*)"
Write-Host "  Grafana:     $GrafanaUrl   (admin / GRAFANA_ADMIN_PASSWORD from .env)"
Write-Host "  Prometheus:  $PromUrl"
