<#
.SYNOPSIS
  Checks the Umbrella MVP functions through its API (PowerShell version of smoke-test.sh).
.DESCRIPTION
  With -Zabbix also checks the real chain Zabbix trigger -> webhook -> Umbrella
  incident -> recovery (run configure.ps1 first). Every run uses its own CI
  names and signals, so it can be repeated.
.EXAMPLE
  .\smoke-test.ps1
  .\smoke-test.ps1 -Zabbix
#>
param([switch]$Zabbix)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'lib.ps1')
Initialize-Lab $PSScriptRoot 'smoke-test'
if ((Get-Setting 'ZABBIX_E2E') -eq '1') { $Zabbix = $true }

$Token = Get-Setting 'SMOKE_WEBHOOK_TOKEN'
if (-not $Token) { Stop-Lab 'set SMOKE_WEBHOOK_TOKEN in .env (Umbrella reads it as UMB_SECRET_LAB_SMOKE)' }
$Run = (Get-Date -Format 'HHmmss') + (Get-Random -Maximum 99999)
$script:Pass = 0
$script:Failed = New-Object System.Collections.Generic.List[string]

function Section([string]$Text) { Write-Host ''; Write-Host $Text -ForegroundColor White }
# Check NAME { condition } [detail]: the condition must return $true.
function Check([string]$Name, [scriptblock]$Condition, $Detail = $null) {
  $okay = $false
  try { $okay = [bool](& $Condition) } catch { $Detail = "$Detail $($_.Exception.Message)" }
  if ($okay) {
    $script:Pass++
    Write-Host '  PASS ' -ForegroundColor Green -NoNewline; Write-Host $Name
  } else {
    $script:Failed.Add($Name)
    Write-Host '  FAIL ' -ForegroundColor Red -NoNewline; Write-Host $Name
    if ($Detail) {
      if ($Detail -isnot [string]) { $Detail = ConvertTo-Json -InputObject $Detail -Depth 6 -Compress }
      Write-Host "       $Detail"
    }
  }
}

function Get-Incident([string]$Ci, [string]$Signal) {
  $q = '/api/incidents?view=all&limit=1000'
  if ($Ci) { $q += "&ci=$Ci" }
  @((Umb GET $q).items) | Where-Object { $_.signal -eq $Signal } | Sort-Object first_seen | Select-Object -Last 1
}
function Wait-Incident([string]$Ci, [string]$Signal, [scriptblock]$Until, [int]$Seconds) {
  $deadline = (Get-Date).AddSeconds($Seconds)
  do {
    $a = Get-Incident $Ci $Signal
    if ($a -and (& $Until $a)) { return $a }
    Start-Sleep -Seconds 2
  } while ((Get-Date) -lt $deadline)
  return $a
}
function New-Event([string]$Id, [string]$CiName, [string]$Signal, [string]$Severity, [string]$Status, [string]$Method = 'red') {
  @{ id = $Id; host = $CiName; signal = $Signal; severity = $Severity; status = $Status; title = "Smoke $Signal on $CiName"; method = $Method; value = '42' }
}
function Status { $script:LastStatus }

Section '1. Service is up'
$r = Send-Http GET "$UmbUrl/healthz"
Check 'GET /healthz answers 200' { $r.Status -eq 200 }
$meta = Umb GET '/api/meta'
Check 'GET /api/meta: version and PagerDuty mode' { $meta.version -and $meta.pd_mode } $meta
$self = Umb GET '/api/selfcheck'
Check 'GET /api/selfcheck: store, bus, PagerDuty status' { $self.store -and $self.bus -and $self.pagerduty.mode } $self
$kinds = @((Umb GET '/api/blocks').items | ForEach-Object { $_.kind })
Check 'GET /api/blocks: block palette for the builder' { ($kinds -contains 'map.event') -and ($kinds -contains 'trigger.webhook') }
$html = (Send-Http GET "$UmbUrl/heatmap").Body
Check 'Web UI is served (SPA route /heatmap)' { $html -match '<div id="root"' }

Section '2. CMDB'
$team = 'smoke'
$svc = Get-LabCi "smoke-service-$Run" 'it_service' $team
$h1n = "smoke-host-1-$Run"; $h2n = "smoke-host-2-$Run"
$h1 = Get-LabCi $h1n 'host' $team $svc
$h2 = Get-LabCi $h2n 'host' $team $svc
Check 'CIs created: IT service and two hosts below it' { $svc -and $h1 -and $h2 }
$graphCmdb = Umb GET '/api/cmdb/graph'
Check 'CMDB graph links the service to the hosts' {
  @($graphCmdb.edges | Where-Object { (($_.from, $_.source) -contains $svc) -and (($_.to, $_.target) -contains $h1) }).Count -gt 0 }

Section '3. Connector builder'
$g = New-WebhookGraph 'openbao://lab/smoke' 'severity' "critical=critical`nerror=error`nwarning=warning`n*=info" "source=smoke`nrun=$Run" @{
  title = '${title}'; ci = '${host}'; signal = '${signal}'; method = '${method|red}'
  status = '${status}'; external_id = '${id}'; value = '${value}'
}
$dry = Umb POST '/api/connectors/none/dry-run' @{ graph = $g; sample = (ConvertTo-Json -Compress -InputObject (New-Event 'd1' $h1n 'dry' 'error' 'firing')) }
Check 'Dry-run: 7 blocks traced, one event out' { @($dry.trace).Count -eq 7 -and @($dry.events).Count -eq 1 } $dry
Check 'Dry-run: severity mapped, CI and signal templated' {
  $e = @($dry.events)[0]; $e.severity -eq 'error' -and $e.ci -eq $h1n -and $e.signal -eq 'dry' -and $e.labels.source -eq 'smoke' } $dry
$dryBad = Umb POST '/api/connectors/none/dry-run' @{ graph = $g; sample = 'not json {' }
Check 'Dry-run: broken payload shows a block error' { @($dryBad.errors).Count -gt 0 } $dryBad

$badC = (Umb POST '/api/connectors' @{ name = 'smoke-invalid'; template = 'webhook-json' }).id
[void](Umb PUT "/api/connectors/$badC" @{ draft = @{ nodes = @(@{ id = 'n1'; kind = 'parse.json'; x = 0; y = 0; config = @{} }); edges = @() } })
[void](Umb POST "/api/connectors/$badC/publish")
$st = Status
Check 'Publish rejects a graph without a trigger (422)' { $st -eq 422 } "status $st"
[void](Umb DELETE "/api/connectors/$badC")

$c1 = Set-WebhookConnector 'smoke-webhook' $team $g
$c2 = Set-WebhookConnector 'smoke-second-source' $team $g
$conn = Umb GET "/api/connectors/$c1"
Check 'Connectors published and running' { $conn.connector.status -eq 'running' -and $conn.connector.version -ge 1 -and $conn.ingest_url } $conn

Section '4. Ingest and webhook auth'
[void](Send-Ingest $c1 '' (New-Event 'x' $h1n 'sig-a' 'warning' 'firing')); $st = Status
Check 'Ingest without token is refused (401)' { $st -eq 401 } "status $st"
[void](Send-Ingest $c1 'wrong-token' (New-Event 'x' $h1n 'sig-a' 'warning' 'firing')); $st = Status
Check 'Ingest with a wrong token is refused (401)' { $st -eq 401 } "status $st"
$r = Send-Ingest $c1 $Token (New-Event "a1-$Run" $h1n 'sig-a' 'warning' 'firing'); $st = Status
Check 'Ingest with token is accepted (202, 1 event)' { $st -eq 202 -and $r.accepted -eq 1 } "status $st"
$a = Wait-Incident $h1 'sig-a' { param($x) $x.status -eq 'open' } 10
Check 'Incident opened and bound to the CMDB host' { $a.ci_id -eq $h1 -and $a.severity -eq 'warning' -and $a.team -eq 'smoke' -and $a.service -like 'smoke-service*' } $a
$inc = $a.id
Check 'Method RED taken from the payload' { $a.method -eq 'red' } $a

Section '5. Dedup'
[void](Send-Ingest $c1 $Token (New-Event "a1-$Run" $h1n 'sig-a' 'warning' 'firing'))
$a = Get-Incident $h1 'sig-a'
Check 'Retry with the same external_id is dropped (inbox dedup)' { $a.count -eq 1 } $a
[void](Send-Ingest $c1 $Token (New-Event "a2-$Run" $h1n 'sig-a' 'error' 'firing'))
$a = Get-Incident $h1 'sig-a'
Check 'Repeat of the same signal folds into one incident' { $a.id -eq $inc -and $a.count -eq 2 } $a
Check 'Severity escalates to the worst seen (error)' { $a.severity -eq 'error' } $a
[void](Send-Ingest $c2 $Token (New-Event "b1-$Run" $h1n 'sig-a' 'warning' 'firing'))
$a = Get-Incident $h1 'sig-a'
Check 'Same CI and signal from a second source: same incident, 2 sources' { $a.id -eq $inc -and @($a.sources.PSObject.Properties).Count -eq 2 } $a

Section '6. Incident actions'
$r = Umb POST "/api/incidents/$inc/ack"
Check 'Ack: status acknowledged, who acked' { $r.status -eq 'acknowledged' -and $r.acked_by -eq 'smoke-test' } $r
[void](Umb POST "/api/incidents/$inc/ack"); $st = Status
Check 'Second ack is refused (409)' { $st -eq 409 } "status $st"
[void](Umb POST "/api/incidents/$inc/comment" @{ text = 'smoke comment' })
$d = Umb GET "/api/incidents/$inc"
Check 'Comment lands in the timeline' { @($d.incident.timeline | Where-Object { $_.kind -eq 'comment' -and $_.text -eq 'smoke comment' }).Count -eq 1 }
Check 'Incident card lists its events' { @($d.events).Count -ge 3 }

Section '7. Recovery from sources'
[void](Send-Ingest $c1 $Token (New-Event "a2-$Run" $h1n 'sig-a' 'error' 'resolved'))
$a = Get-Incident $h1 'sig-a'
Check 'One source recovered, the other still firing: stays active' { $a.status -eq 'acknowledged' } $a
[void](Send-Ingest $c2 $Token (New-Event "b1-$Run" $h1n 'sig-a' 'warning' 'ok'))
$a = Get-Incident $h1 'sig-a'
Check 'All sources recovered: incident resolved' { $a.status -eq 'resolved' -and $a.resolved_at } $a
[void](Send-Ingest $c1 $Token (New-Event "a3-$Run" $h1n 'sig-a' 'warning' 'firing'))
$a = Get-Incident $h1 'sig-a'
Check 'Repeat within the fold window reopens the same incident' { $a.id -eq $inc -and $a.status -eq 'open' } $a
$r = Umb POST "/api/incidents/$inc/resolve"
Check 'Manual resolve' { $r.status -eq 'resolved' } $r

Section '8. Maintenance'
$fmt = 'yyyy-MM-ddTHH:mm:ssZ'
$now = (Get-Date).ToUniversalTime()
$mw = Umb POST '/api/maintenance' @{ title = 'smoke maintenance'; ci_id = $h2; start = $now.AddMinutes(-1).ToString($fmt); end = $now.AddHours(1).ToString($fmt) }
$st = Status
Check 'Maintenance window created' { $st -eq 201 } $mw
[void](Send-Ingest $c1 $Token (New-Event "m1-$Run" $h2n 'sig-m' 'critical' 'firing'))
$a = Get-Incident $h2 'sig-m'
Check 'Event during maintenance: incident suppressed, not sent to PagerDuty' { $a.suppressed -eq $true -and $a.pd_state -eq 'skipped' } $a
[void](Umb DELETE "/api/maintenance/$($mw.id)"); $st = Status
Check 'Maintenance window deleted' { $st -eq 204 }

Section '9. Unknown CI and parse errors'
[void](Send-Ingest $c1 $Token (New-Event "u1-$Run" "smoke-unknown-$Run" 'sig-u' 'warning' 'firing'))
$a = Get-Incident '' 'sig-u'
Check 'Event for a CI missing in CMDB opens an unbound incident' { $a.ci_name -eq "smoke-unknown-$Run" -and -not $a.ci_id } $a
$before = @((Umb GET '/api/parse-errors').items | Where-Object { $_.connector_id -eq $c1 }).Count
[void](Send-Ingest $c1 $Token 'definitely not json {')
$after = @((Umb GET '/api/parse-errors').items | Where-Object { $_.connector_id -eq $c1 }).Count
Check 'Broken payload goes to the parse error list' { $after -gt $before } "before $before after $after"

Section '10. Bulk actions'
[void](Send-Ingest $c1 $Token (New-Event "k1-$Run" $h1n 'sig-k1' 'info' 'firing'))
[void](Send-Ingest $c1 $Token (New-Event "k2-$Run" $h1n 'sig-k2' 'info' 'firing'))
$k1 = (Get-Incident $h1 'sig-k1').id; $k2 = (Get-Incident $h1 'sig-k2').id
$r = Umb POST '/api/incidents/bulk' @{ ids = @($k1, $k2); action = 'ack' }
Check 'Bulk ack of two incidents' { $r.done -eq 2 } $r
$r = Umb POST '/api/incidents/bulk' @{ ids = @($k1, $k2); action = 'resolve' }
Check 'Bulk resolve of two incidents' { $r.done -eq 2 } $r

Section '11. Views'
$l = Umb GET '/api/incidents?view=closed&team=smoke&q=sig-k1'
Check 'Incident list: filters by view, team and text' { @($l.items | ForEach-Object { $_.id }) -contains $k1 }
$hm = Umb GET '/api/heatmap?hours=6&group=service&team=smoke'
Check 'Heatmap: 24 columns, host row with incidents' {
  @($hm.columns).Count -eq 24 -and @($hm.groups | ForEach-Object { $_.rows } | Where-Object { $_.ci_id -eq $h1 -and $_.total -gt 0 }).Count -eq 1 }
$hmt = Umb GET '/api/heatmap?hours=24&group=team'
Check 'Heatmap grouped by team' { @($hmt.groups | ForEach-Object { $_.key }) -contains 'smoke' }
$ev = Umb GET "/api/events?connector=$c1&limit=50"
Check 'Event log keeps raw payload and labels' { @($ev.items | Where-Object { $_.labels.run -eq $Run -and $_.raw }).Count -ge 5 }
[void](Umb GET "/api/cis/$h1"); $st = Status
Check 'CI card opens' { $st -eq 200 }
$wsCode = 0
try {
  $ws = New-Object System.Net.WebSockets.ClientWebSocket
  $ws.Options.SetRequestHeader('Origin', $UmbUrl)
  $ws.ConnectAsync([uri](($UmbUrl -replace '^http', 'ws') + '/api/ws'), [Threading.CancellationToken]::None).Wait(5000) | Out-Null
  if ($ws.State -eq 'Open') { $wsCode = 101 }
  $ws.Dispose()
} catch { }
Check 'Live updates: WebSocket handshake (101)' { $wsCode -eq 101 }

Section '12. PagerDuty gateway'
[void](Umb POST '/api/selfcheck/pd-outage' @{ on = $true })
[void](Send-Ingest $c1 $Token (New-Event "p1-$Run" $h1n 'sig-pd' 'critical' 'firing'))
Start-Sleep -Seconds 3
$a = Get-Incident $h1 'sig-pd'
$self = Umb GET '/api/selfcheck'
Check 'Simulated PagerDuty outage: delivery not accepted' { @('pending', 'failed') -contains $a.pd_state } $a
Check 'Self-check shows the outage' { $self.pagerduty.simulated_outage -eq $true } $self
[void](Umb POST '/api/selfcheck/pd-outage' @{ on = $false })
[void](Send-Ingest $c1 $Token (New-Event "p2-$Run" $h1n 'sig-pd2' 'critical' 'firing'))
$a = Wait-Incident $h1 'sig-pd2' { param($x) $x.pd_state -eq 'accepted' } 20
Check 'After the outage: new incident accepted by PagerDuty (or dry-run)' { $a.pd_state -eq 'accepted' } $a
foreach ($s in 'sig-pd', 'sig-pd2') { [void](Umb POST "/api/incidents/$((Get-Incident $h1 $s).id)/resolve") }

Section '13. Connector lifecycle and audit'
[void](Umb POST "/api/connectors/$c2/stop")
[void](Send-Ingest $c2 $Token (New-Event "s1-$Run" $h1n 'sig-s' 'info' 'firing')); $st = Status
Check 'Stopped connector refuses events (409)' { $st -eq 409 } "status $st"
[void](Umb POST "/api/connectors/$c2/start")
$acts = @((Umb GET '/api/audit').items | Where-Object { $_.actor -eq 'smoke-test' } | ForEach-Object { $_.action })
Check 'Audit log: publish, maintenance, acks by smoke-test' {
  @($acts | Where-Object { $_ -like 'connector.publish*' }).Count -gt 0 -and $acts -contains 'maintenance.create' -and $acts -contains 'alert.ack' }

if ($Zabbix) {
  Section '14. Zabbix end-to-end'
  $labHost = 'umbrella-lab-agent'
  if (Connect-Zabbix 'Admin' (Get-Setting 'ZABBIX_ADMIN_PASSWORD')) {
    Check 'Zabbix API login' { $true }
    $hostId = (First (Zbx 'host.get' @{ filter = @{ host = @($labHost) }; output = @('hostid') })).hostid
    $item = (First (Zbx 'item.get' @{ hostids = @($hostId); filter = @{ key_ = 'umbrella.test' }; output = @('itemid') })).itemid
    $trg = (First (Zbx 'trigger.get' @{ hostids = @($hostId); filter = @{ description = 'Umbrella test problem' }; output = @('triggerid') })).triggerid
    Check 'Lab host, test item and trigger exist in Zabbix' { $item -and $trg }
    $avail = ''
    for ($i = 0; $i -lt 20; $i++) {
      $avail = [string](First (First (Zbx 'host.get' @{ hostids = @($hostId); selectInterfaces = @('available'); output = @('hostid') })).interfaces).available
      if ($avail -eq '1') { break }
      Start-Sleep -Seconds 6
    }
    Check 'Zabbix agent 2 is reachable (interface available)' { $avail -eq '1' } "available=$avail"
    $labCi = Get-CiId $labHost
    # Start from OK: a problem left open by an earlier run would not fire again.
    [void](Zbx 'history.push' @(@{ itemid = $item; value = '0' }))
    for ($i = 0; $i -lt 30; $i++) {
      if ([string](First (Zbx 'trigger.get' @{ triggerids = @($trg); output = @('value') })).value -eq '0') { break }
      Start-Sleep -Seconds 2
    }
    Start-Sleep -Seconds 2
    [void](Zbx 'history.push' @(@{ itemid = $item; value = '1' }))
    $a = Wait-Incident $labCi "zabbix:$trg" { param($x) @('open', 'acknowledged') -contains $x.status } 120
    Check 'Zabbix problem -> Umbrella incident on the lab host (High -> error)' { $a.severity -eq 'error' -and $a.method -eq 'use' -and $a.status -ne 'resolved' } $a
    $zev = (First (Zbx 'problem.get' @{ objectids = @($trg); output = @('eventid') })).eventid
    if ($zev) {
      $sent = @(Zbx 'alert.get' @{ eventids = @($zev); output = @('status', 'error') })
      Check 'Zabbix marks the webhook as sent (Umbrella acked with 2xx)' { @($sent | Where-Object { $_.status -eq '1' }).Count -gt 0 } $sent
    }
    [void](Zbx 'history.push' @(@{ itemid = $item; value = '0' }))
    $a = Wait-Incident $labCi "zabbix:$trg" { param($x) $x.status -eq 'resolved' } 120
    Check 'Zabbix recovery -> Umbrella incident resolved' { $a.status -eq 'resolved' } $a
  } else {
    Check 'Zabbix API login' { $false } 'check ZABBIX_ADMIN_PASSWORD and run .\configure.ps1'
  }
}

Write-Host ''
Write-Host ("Result: {0} passed, {1} failed" -f $script:Pass, $script:Failed.Count) -ForegroundColor White
foreach ($f in $script:Failed) { Write-Host "  - $f" }
if ($script:Failed.Count -gt 0) { exit 1 }
exit 0
