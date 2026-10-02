<#
.SYNOPSIS
  Connects Zabbix to Umbrella (PowerShell version of configure.sh).
.DESCRIPTION
  Safe to run again: finds what it created before and updates it. Umbrella MVP
  keeps its data in memory, so run it again after the umbrella container restarts.

  Umbrella: CMDB entries for the lab, the "Zabbix" webhook connector
  (parse.json -> map.severity -> enrich.labels -> map.event -> out.event ->
  ack.response), published and running.
  Zabbix: Admin password from .env, the "Umbrella" webhook media type, media for
  Admin, the "Send problems to Umbrella" action, the lab agent host with the
  Linux template and a trapper item with a trigger the test can fire.
.EXAMPLE
  .\configure.ps1
#>
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'lib.ps1')
Initialize-Lab $PSScriptRoot

$webhookToken = Get-Setting 'ZABBIX_WEBHOOK_TOKEN'
$adminPassword = Get-Setting 'ZABBIX_ADMIN_PASSWORD'
if (-not $webhookToken -or -not $adminPassword) { Stop-Lab 'set ZABBIX_WEBHOOK_TOKEN and ZABBIX_ADMIN_PASSWORD in .env' }
# Zabbix server reaches Umbrella over the compose network.
$umbInternal = Get-Setting 'UMB_INTERNAL_URL' 'http://umbrella:8080'
$labHost = 'umbrella-lab-agent'
$labTeam = Get-Setting 'LAB_TEAM' 'monitoring'

Write-Step "Waiting for Umbrella at $UmbUrl"
if (-not (Wait-Http "$UmbUrl/healthz" 180)) { Stop-Lab "Umbrella does not answer at $UmbUrl" }

Write-Step 'Umbrella: CMDB entries for the lab'
$svcId = Get-LabCi 'umbrella-lab' 'it_service' $labTeam
$hostCi = Get-LabCi $labHost 'host' $labTeam $svcId
Write-Step "  it_service umbrella-lab = $svcId, host $labHost = $hostCi"

Write-Step 'Umbrella: Zabbix connector'
$graph = New-WebhookGraph 'openbao://lab/zabbix' 'severity' "Disaster=critical`nHigh=error`nAverage=warning`nWarning=warning`n*=info" `
  "source=zabbix`nzabbix_trigger=`${trigger_id}`nzabbix_url=`${url}" @{
  title = '${trigger_name}'; ci = '${host}'; signal = 'zabbix:${trigger_id}'; method = 'use'
  status = '${status}'; external_id = '${event_id}'; value = '${value}'
}
$connId = Set-WebhookConnector 'Zabbix' $labTeam $graph
$sample = ConvertTo-Json -Compress -InputObject @{ event_id = '1001'; event_value = '1'; status = 'firing'; host = $labHost
  trigger_id = '20001'; trigger_name = 'High CPU utilization'; severity = 'High'; value = '97 %'; url = '' }
[void](Umb PUT "/api/connectors/$connId" @{ description = 'Zabbix 7.0 webhook media type "Umbrella" (deploy/zabbix-lab)'; sample_input = $sample })
Write-Step "  connector $connId, ingest $umbInternal/api/ingest/$connId"

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
  @{ name = 'url'; value = "$umbInternal/api/ingest/$connId" }, @{ name = 'token'; value = $webhookToken },
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

Write-Host ''
Write-Host 'Done.'
Write-Host "  Umbrella:  $UmbUrl   (connector $connId `"Zabbix`")"
Write-Host "  Zabbix:    $ZbxUrl   (Admin / ZABBIX_ADMIN_PASSWORD from .env)"
