<#
.SYNOPSIS
  Checks the Umbrella MVP functions through its API (PowerShell version of smoke-test.sh).
.DESCRIPTION
  -Zabbix adds the real chain Zabbix trigger -> webhook -> Umbrella incident ->
  recovery; -Stack adds Prometheus -> Alertmanager -> Umbrella, OpenSearch ->
  Umbrella, Teams/Zoom notifications (the lab sink), Grafana and OpenSearch
  Dashboards; -All runs everything (run configure.ps1 first).

  Signs in as UMBRELLA_ADMIN_USER from .env, creates the service account
  smoke-test (roles monitoring + auditor) and works with its API token. Every
  run uses its own CI names, signals and users, so it can be repeated.
  Runs in ConstrainedLanguage mode: only cmdlets and core types.
.EXAMPLE
  .\smoke-test.ps1
  .\smoke-test.ps1 -Zabbix
  .\smoke-test.ps1 -Stack
  .\smoke-test.ps1 -All
#>
param([switch]$Zabbix, [switch]$Stack, [switch]$All)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'lib.ps1')
Initialize-Lab $PSScriptRoot
if ((Get-Setting 'ZABBIX_E2E') -eq '1') { $Zabbix = $true }
if ((Get-Setting 'STACK_E2E') -eq '1') { $Stack = $true }
if ($All) { $Zabbix = $true; $Stack = $true }

$Token = Get-Setting 'SMOKE_WEBHOOK_TOKEN'
if (-not $Token) { Stop-Lab 'set SMOKE_WEBHOOK_TOKEN in .env (Umbrella reads it as UMB_SECRET_LAB_SMOKE)' }
$umbAdmin = Get-Setting 'UMBRELLA_ADMIN_USER' 'admin'
$Run = (Get-Date -Format 'HHmmss') + (Get-Random -Maximum 99999)
# Password of the users this run creates (deleted at the end).
$RunPw = 'Smk' + ((New-Guid).Guid -replace '-', '').Substring(0, 16) + '9'
$script:Pass = 0
$script:Failed = @()

function Section([string]$Text) { Write-Host ''; Write-Host $Text -ForegroundColor White }
# Check NAME { condition } [detail]: the condition must return $true.
function Check([string]$Name, [scriptblock]$Condition, $Detail = $null) {
  $okay = $false
  try { $okay = [bool](& $Condition) } catch { $Detail = "$Detail $($_.Exception.Message)" }
  if ($okay) {
    $script:Pass++
    Write-Host '  PASS ' -ForegroundColor Green -NoNewline; Write-Host $Name
  } else {
    $script:Failed += $Name
    Write-Host '  FAIL ' -ForegroundColor Red -NoNewline; Write-Host $Name
    if ($Detail) {
      if ($Detail -isnot [string]) { $Detail = ConvertTo-Json -InputObject $Detail -Depth 6 -Compress }
      if ($Detail.Length -gt 600) { $Detail = $Detail.Substring(0, 600) }
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
# Get-UtcStamp turns an API timestamp (string, or DateTime in PowerShell 7)
# into 'yyyy-MM-ddTHH:mm:ss' UTC, which compares as text.
function Get-UtcStamp($Value) {
  if ($Value -is [datetime]) { return $Value.ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ss') }
  $s = [string]$Value
  if ($s.Length -ge 19) { return $s.Substring(0, 19) }
  return $s
}
function Get-Delivery([string]$Alert) { @((Umb GET '/api/deliveries').items | Where-Object { $_.alert_id -eq $Alert }) }

# Admin session and the smoke-test service account with a fresh API token.
$adminResp = Send-Http 'POST' "$UmbUrl/api/auth/token" @{ username = $umbAdmin; password = (Get-Setting 'UMBRELLA_ADMIN_PASSWORD') }
$adminToken = ''
if ($adminResp.Status -eq 200) { $adminToken = [string]$adminResp.Json.token }
if (-not $adminToken) { Stop-Lab "cannot sign in to Umbrella at $UmbUrl as $umbAdmin (status $($adminResp.Status)): check UMBRELLA_ADMIN_PASSWORD in .env" }
$script:UmbToken = $adminToken
$smokeUid = Set-LabUser @{ username = 'smoke-test'; name = 'Smoke test'; service = $true; roles = @('monitoring', 'auditor'); business_services = @() }
$tok = Umb POST "/api/users/$smokeUid/tokens" @{ name = "smoke-$Run"; days = 1 }
if (-not $tok.token) { Stop-Lab 'cannot create an API token for smoke-test' }
$smokeToken = [string]$tok.token
$script:UmbToken = $smokeToken

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

Section '2. Sign-in, roles and API tokens'
$r = Umb-As '' GET '/api/incidents'; $st = Status
Check 'API without sign-in is refused (401 unauthorized)' { $st -eq 401 -and $r.code -eq 'unauthorized' } "status $st"
Check "Sign-in POST /api/auth/token as $umbAdmin (200, bearer token)" {
  $adminResp.Json.token -and $adminResp.Json.must_change_password -eq $false -and $adminResp.Json.expires_in -gt 0 } "status $($adminResp.Status)"
$me = Umb-As $adminToken GET '/api/auth/me'
Check 'GET /api/auth/me: administrator with users.admin' { @($me.user.roles) -contains 'admin' -and @($me.permissions) -contains 'users.admin' -and $me.all_services -eq $true } $me
$roleIds = @((Umb-As $adminToken GET '/api/roles').items | ForEach-Object { $_.id })
Check 'Built-in roles: admin, monitoring, oncall, owner, viewer, auditor, reader' {
  @('admin', 'monitoring', 'oncall', 'owner', 'viewer', 'auditor', 'reader' | Where-Object { $roleIds -notcontains $_ }).Count -eq 0 } ($roleIds -join ',')
$me = Umb GET '/api/auth/me'
Check 'API token of the service account smoke-test works (monitoring + auditor)' {
  $me.user.username -eq 'smoke-test' -and $me.user.service -eq $true -and @($me.permissions) -contains 'connectors.edit' -and @($me.permissions) -notcontains 'users.admin' } $me
[void](Umb GET '/api/users'); $st = Status
Check 'Service account without users.admin: GET /api/users is refused (403)' { $st -eq 403 } "status $st"
[void](Send-Http 'POST' "$UmbUrl/api/auth/token" @{ username = 'smoke-test'; password = 'anything-1' }); $st = Status
Check 'Service account cannot sign in with a password (401)' { $st -eq 401 } "status $st"
$grafanaToken = Get-Setting 'UMBRELLA_GRAFANA_TOKEN'
if ($grafanaToken) {
  $users = @((Umb-As $adminToken GET '/api/users').items)
  Check 'Service account grafana (role reader) from UMBRELLA_GRAFANA_TOKEN' {
    @($users | Where-Object { $_.username -eq 'grafana' -and $_.service -eq $true -and (@($_.roles) -join ',') -eq 'reader' -and $_.tokens -ge 1 }).Count -eq 1 }
  $r = Umb-As $grafanaToken GET '/api/incidents?view=open'; $st = Status
  Check 'Grafana token reads incidents (200)' { $st -eq 200 -and $null -ne $r.items } "status $st"
  $r = Umb-As $grafanaToken POST '/api/incidents/bulk' @{ ids = @(); action = 'ack' }; $st = Status
  Check 'Grafana token cannot act on incidents (403 forbidden)' { $st -eq 403 -and $r.code -eq 'forbidden' } "status $st"
}
$metricsToken = Get-Setting 'UMBRELLA_METRICS_TOKEN'
$m = Send-Http GET "$UmbUrl/metrics"
if ($metricsToken) {
  $st = $m.Status
  Check 'GET /metrics without the metrics token is refused (401)' { $st -eq 401 } "status $st"
  $m = Send-Http GET "$UmbUrl/metrics" $null @{ Authorization = "Bearer $metricsToken" }
}
Check 'GET /metrics: Prometheus text with umbrella_up and incident gauges' {
  $m.Status -eq 200 -and $m.Body -match '(?m)^umbrella_up 1' -and $m.Body -match '(?m)^umbrella_incidents_active\{' } "status $($m.Status)"

Section '3. CMDB'
$team = 'smoke'
$bs = Get-LabCi "smoke-bs-$Run" 'business_service' $team
$svc = Get-LabCi "smoke-service-$Run" 'it_service' $team $bs
$h1n = "smoke-host-1-$Run"; $h2n = "smoke-host-2-$Run"; $h3n = "smoke-host-3-$Run"
$h1 = Get-LabCi $h1n 'host' $team $svc
$h2 = Get-LabCi $h2n 'host' $team $svc
$bs2 = Get-LabCi "smoke-bs2-$Run" 'business_service' 'smoke2'
$svc2 = Get-LabCi "smoke-service2-$Run" 'it_service' 'smoke2' $bs2
$h3 = Get-LabCi $h3n 'host' 'smoke2' $svc2
Check 'CIs created: business service, IT service and two hosts below it' { $bs -and $svc -and $h1 -and $h2 -and $h3 }
$graphCmdb = Umb GET '/api/cmdb/graph'
Check 'CMDB graph links the service to the hosts' {
  @($graphCmdb.edges | Where-Object { (($_.from, $_.source) -contains $svc) -and (($_.to, $_.target) -contains $h1) }).Count -gt 0 }
Check 'CMDB graph: business service depends on the IT service' {
  @($graphCmdb.edges | Where-Object { (($_.from, $_.source) -contains $bs) -and (($_.to, $_.target) -contains $svc) }).Count -gt 0 }

Section '4. Connector builder'
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

Section '5. Ingest and webhook auth'
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

Section '6. Dedup'
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

Section '7. Incident actions'
$r = Umb POST "/api/incidents/$inc/ack"
Check 'Ack: status acknowledged, who acked' { $r.status -eq 'acknowledged' -and $r.acked_by -eq 'smoke-test' } $r
[void](Umb POST "/api/incidents/$inc/ack"); $st = Status
Check 'Second ack is refused (409)' { $st -eq 409 } "status $st"
[void](Umb POST "/api/incidents/$inc/comment" @{ text = 'smoke comment' })
$d = Umb GET "/api/incidents/$inc"
Check 'Comment lands in the timeline' { @($d.incident.timeline | Where-Object { $_.kind -eq 'comment' -and $_.text -eq 'smoke comment' }).Count -eq 1 }
Check 'Incident card lists its events' { @($d.events).Count -ge 3 }

Section '8. Recovery from sources'
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

Section '9. Maintenance'
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

Section '10. Unknown CI and parse errors'
[void](Send-Ingest $c1 $Token (New-Event "u1-$Run" "smoke-unknown-$Run" 'sig-u' 'warning' 'firing'))
$a = Get-Incident '' 'sig-u'
$auto = First (@((Umb GET "/api/cis?q=smoke-unknown-$Run").items) | Where-Object { $_.name -eq "smoke-unknown-$Run" })
if ($auto) {
  Check 'Event for a CI missing in CMDB: CI added automatically (origin auto, UMBRELLA_CMDB_AUTO)' { $auto.origin -eq 'auto' -and $auto.type -eq 'host' } $auto
  Check '  ...and the incident is bound to it' { $a.ci_id -eq $auto.id } $a
} else {
  Check 'Event for a CI missing in CMDB opens an unbound incident (UMBRELLA_CMDB_AUTO=false)' { $a.ci_name -eq "smoke-unknown-$Run" -and -not $a.ci_id } $a
}
$before = @((Umb GET '/api/parse-errors').items | Where-Object { $_.connector_id -eq $c1 }).Count
[void](Send-Ingest $c1 $Token 'definitely not json {')
$after = @((Umb GET '/api/parse-errors').items | Where-Object { $_.connector_id -eq $c1 }).Count
Check 'Broken payload goes to the parse error list' { $after -gt $before } "before $before after $after"

Section '11. Bulk actions'
[void](Send-Ingest $c1 $Token (New-Event "k1-$Run" $h1n 'sig-k1' 'info' 'firing'))
[void](Send-Ingest $c1 $Token (New-Event "k2-$Run" $h1n 'sig-k2' 'info' 'firing'))
$k1 = (Get-Incident $h1 'sig-k1').id; $k2 = (Get-Incident $h1 'sig-k2').id
$r = Umb POST '/api/incidents/bulk' @{ ids = @($k1, $k2); action = 'ack' }
Check 'Bulk ack of two incidents' { $r.done -eq 2 } $r
$r = Umb POST '/api/incidents/bulk' @{ ids = @($k1, $k2); action = 'resolve' }
Check 'Bulk resolve of two incidents' { $r.done -eq 2 } $r

Section '12. Views'
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
# ConstrainedLanguage has no WebSocket client, so the handshake goes through
# curl (built into Windows 10 and later). Without curl the check is skipped.
$curl = Get-Command 'curl.exe' -CommandType Application -ErrorAction SilentlyContinue
if (-not $curl) { $curl = Get-Command 'curl' -CommandType Application -ErrorAction SilentlyContinue }
$wsLine = ''
if ($curl) {
  $old = $ErrorActionPreference
  $ErrorActionPreference = 'Continue'
  $wsLine = [string](@(& (First $curl).Source -sS -i -N --max-time 3 -H 'Connection: Upgrade' -H 'Upgrade: websocket' `
        -H 'Sec-WebSocket-Version: 13' -H 'Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==' -H "Origin: $UmbUrl" -H "Authorization: Bearer $($script:UmbToken)" "$UmbUrl/api/ws" 2>$null) | Select-Object -First 1)
  $ErrorActionPreference = $old
}
if ($curl) {
  Check 'Live updates: WebSocket handshake (101)' { $wsLine -match ' 101' } $wsLine
} else {
  Write-Host '  SKIP Live updates: WebSocket handshake (curl not found)' -ForegroundColor Yellow
}

Section '13. PagerDuty gateway'
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

Section '14. Connector lifecycle and audit'
[void](Umb POST "/api/connectors/$c2/stop")
[void](Send-Ingest $c2 $Token (New-Event "s1-$Run" $h1n 'sig-s' 'info' 'firing')); $st = Status
Check 'Stopped connector refuses events (409)' { $st -eq 409 } "status $st"
[void](Umb POST "/api/connectors/$c2/start")
$acts = @((Umb GET '/api/audit').items | Where-Object { $_.actor -eq 'smoke-test' } | ForEach-Object { $_.action })
Check 'Audit log: publish, maintenance, acks by smoke-test' {
  @($acts | Where-Object { $_ -like 'connector.publish*' }).Count -gt 0 -and $acts -contains 'maintenance.create' -and $acts -contains 'alert.ack' }

Section '15. Roles and service scope'
$script:UmbToken = $adminToken
$ownerName = "smoke-owner-$Run"; $viewerName = "smoke-viewer-$Run"
$ownerId = Set-LabUser @{ username = $ownerName; password = $RunPw; roles = @('owner'); business_services = @($bs); must_change_password = $false }
$viewerId = Set-LabUser @{ username = $viewerName; password = $RunPw; roles = @('viewer'); business_services = @($bs); must_change_password = $false }
$script:UmbToken = $smokeToken
Check "Users created: owner and viewer of smoke-bs-$Run" { $ownerId -and $viewerId }
[void](Umb-As $adminToken POST '/api/users' @{ username = 'smoke-weak'; password = 'short1'; roles = @('viewer') }); $st = Status
Check 'Password policy: a weak password is refused (400)' { $st -eq 400 } "status $st"
[void](Send-Ingest $c1 $Token (New-Event "o1-$Run" $h1n 'sig-own' 'warning' 'firing'))
[void](Send-Ingest $c1 $Token (New-Event "o2-$Run" $h3n 'sig-other' 'warning' 'firing'))
$mine = (Get-Incident $h1 'sig-own').id; $other = (Get-Incident $h3 'sig-other').id
$ownerToken = Get-UmbToken $ownerName $RunPw
$viewerToken = Get-UmbToken $viewerName $RunPw
Check 'Owner and viewer sign in with their passwords' { $ownerToken -and $viewerToken }
[void](Send-Http 'POST' "$UmbUrl/api/auth/token" @{ username = $viewerName; password = 'wrong-password-1' }); $st = Status
Check 'Wrong password is refused (401)' { $st -eq 401 } "status $st"
$ids = @((Umb-As $ownerToken GET '/api/incidents?view=open&limit=1000').items | ForEach-Object { $_.id })
Check 'Owner sees the incident of its business service' { $ids -contains $mine } $mine
Check 'Owner does not see the incident of another business service' { $ids -notcontains $other } $other
[void](Umb-As $ownerToken GET "/api/incidents/$other"); $st = Status
Check 'Owner opening a foreign incident gets 404' { $st -eq 404 } "status $st"
$ciIds = @((Umb-As $ownerToken GET '/api/cis').items | ForEach-Object { $_.id })
Check 'Owner CMDB view: own hosts only' { $ciIds -contains $h1 -and $ciIds -notcontains $h3 }
[void](Umb-As $ownerToken GET '/api/connectors'); $st = Status
Check 'Owner has no access to connectors (403)' { $st -eq 403 } "status $st"
$ids = @((Umb-As $viewerToken GET '/api/incidents?view=open&limit=1000').items | ForEach-Object { $_.id })
Check 'Viewer sees the incident of its business service' { $ids -contains $mine }
$r = Umb-As $viewerToken POST "/api/incidents/$mine/ack"; $st = Status
Check 'Viewer cannot acknowledge (403 forbidden)' { $st -eq 403 -and $r.code -eq 'forbidden' } "status $st"
$r = Umb-As $ownerToken POST "/api/incidents/$mine/ack"
Check 'Owner acknowledges the incident of its service' { $r.status -eq 'acknowledged' -and $r.acked_by -eq $ownerName } $r
foreach ($i in $mine, $other) { [void](Umb POST "/api/incidents/$i/resolve") }

if ($Stack) {
  if (-not (Get-Setting 'GRAFANA_ADMIN_PASSWORD')) { Stop-Lab 'set GRAFANA_ADMIN_PASSWORD in .env' }
  $labNodeCi = Get-CiId 'umbrella-lab-node'
  $textfile = Join-Path (Join-Path $PSScriptRoot 'node-textfile') 'umbrella_lab.prom'
  $promSignal = 'prometheus:UmbrellaLabTestFailure'

  Section '16. Prometheus -> Alertmanager -> Umbrella'
  $st = (Send-Http GET "$PromUrl/-/ready").Status
  Check "Prometheus is ready at $PromUrl" { $st -eq 200 } "status $st"
  $targets = @((Send-Http GET "$PromUrl/api/v1/targets?state=active").Json.data.activeTargets)
  $up = @($targets | Where-Object { $_.health -eq 'up' } | ForEach-Object { $_.labels.job })
  Check 'Prometheus targets up: prometheus, node, umbrella, alertmanager' {
    @('prometheus', 'node', 'umbrella', 'alertmanager' | Where-Object { $up -notcontains $_ }).Count -eq 0 } ($up -join ',')
  $ruleNames = @((Send-Http GET "$PromUrl/api/v1/rules").Json.data.groups | ForEach-Object { $_.rules } | ForEach-Object { $_.name })
  Check 'Lab alert rules loaded (UmbrellaLabTestFailure, HostHighLoad)' { $ruleNames -contains 'UmbrellaLabTestFailure' -and $ruleNames -contains 'HostHighLoad' }
  $pc = First (@((Umb GET '/api/connectors').items) | Where-Object { $_.slug -eq 'prometheus' })
  Check 'Umbrella connector prometheus is running (/api/ingest/prometheus)' { $pc.status -eq 'running' } $pc
  # Start from a resolved alert: an open one left by an aborted run would hide the new firing.
  Remove-Item $textfile -ErrorAction SilentlyContinue
  [void](Wait-Incident $labNodeCi $promSignal { param($x) $x.status -eq 'resolved' } 150)
  $t0 = (Get-Date).ToUniversalTime().AddSeconds(-1).ToString('yyyy-MM-ddTHH:mm:ss')
  Set-Content -Path "$textfile.tmp" -Value "# smoke-test $Run`numbrella_lab_test_failure{check=`"smoke`"} 1`n" -NoNewline -Encoding ascii
  Move-Item -Path "$textfile.tmp" -Destination $textfile -Force
  $a = Wait-Incident $labNodeCi $promSignal { param($x) (@('open', 'acknowledged') -contains $x.status) -and (Get-UtcStamp $x.last_seen) -ge $t0 } 180
  Check 'Textfile metric -> rule fires -> Alertmanager -> Umbrella incident (error)' {
    (@('open', 'acknowledged') -contains $a.status) -and (Get-UtcStamp $a.last_seen) -ge $t0 } $a
  Check 'Incident from Alertmanager: CI umbrella-lab-node, title from annotations.summary' {
    $a.ci_name -eq 'umbrella-lab-node' -and $a.severity -eq 'error' -and $a.title -like 'Lab test failure*' } $a
  $ev = @((Umb GET "/api/events?connector=$($pc.id)&limit=20").items)
  Check 'Event keeps the Alertmanager labels (source, alertname)' {
    @($ev | Where-Object { $_.labels.source -eq 'prometheus' -and $_.labels.alertname -eq 'UmbrellaLabTestFailure' }).Count -gt 0 }
  Remove-Item $textfile -ErrorAction SilentlyContinue
  $a = Wait-Incident $labNodeCi $promSignal { param($x) $x.status -eq 'resolved' } 240
  Check 'Metric removed -> alert resolved -> Umbrella incident resolved' { $a.status -eq 'resolved' } $a

  Section '17. OpenSearch -> Umbrella'
  $osdStatus = Osd GET '/api/status'
  Check "OpenSearch Dashboards status green at $OsdUrl" { $osdStatus.status.overall.state -eq 'green' }
  [void](Osd GET '/api/saved_objects/index-pattern/lab-logs'); $st = Status
  Check 'Dashboards index pattern lab-logs* exists' { $st -eq 200 } "status $st"
  [void](Invoke-OpenSearch GET '/lab-logs/_mapping'); $st = Status
  Check 'OpenSearch index lab-logs exists' { $st -eq 200 } "status $st"
  $oc = First (@((Umb GET '/api/connectors').items) | Where-Object { $_.slug -eq 'opensearch' })
  Check 'Umbrella connector opensearch (pull) is running' { $oc.status -eq 'running' } $oc
  $now = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
  [void](Invoke-OpenSearch POST '/lab-logs/_doc?refresh=true' @{ '@timestamp' = $now; level = 'error'; host = 'shop-api-1'
      service = 'lab-shop-api'; message = "Smoke ${Run}: payment gateway timeout"; error_code = "SMOKE-$Run" }); $st = Status
  Check 'Error entry indexed into lab-logs' { $st -eq 201 } "status $st"
  [void](Invoke-OpenSearch POST '/lab-logs/_doc?refresh=true' @{ '@timestamp' = $now; level = 'info'; host = 'shop-api-1'
      service = 'lab-shop-api'; message = "Smoke ${Run}: all good"; error_code = "INFO-$Run" })
  $osA = Wait-Incident '' "opensearch:lab-shop-api:SMOKE-$Run" { param($x) $x.status -eq 'open' } 90
  Check 'Error log -> Umbrella incident on shop-api-1 (service lab-shop-api, error)' {
    $osA.ci_name -eq 'shop-api-1' -and $osA.service -eq 'lab-shop-api' -and $osA.severity -eq 'error' } $osA
  Check 'Info log entry opens no incident' { -not (Get-Incident '' "opensearch:lab-shop-api:INFO-$Run") }
  Start-Sleep -Seconds 20
  $a = Get-Incident '' "opensearch:lab-shop-api:SMOKE-$Run"
  Check 'Repeated polls of the same entry do not add events (dedup by _id)' { $a.count -eq 1 } $a
  $osInc = $osA.id
  $labOwnerToken = Get-UmbToken 'lab-owner' (Get-Setting 'LAB_OWNER_PASSWORD')
  $me = Umb-As $labOwnerToken GET '/api/auth/me'
  Check 'User lab-owner (configure.sh) signs in, bound to lab-shop' {
    (@($me.user.roles) -join ',') -eq 'owner' -and @($me.business_services | Where-Object { $_.name -eq 'lab-shop' }).Count -eq 1 } $me
  $items = @((Umb-As $labOwnerToken GET '/api/incidents?view=open&limit=1000').items)
  Check 'lab-owner sees the OpenSearch incident of lab-shop, not the smoke ones' {
    @($items | ForEach-Object { $_.id }) -contains $osInc -and @($items | Where-Object { ([string]$_.service) -like 'smoke*' }).Count -eq 0 }

  Section '18. Notifications: Teams and Zoom'
  $chs = @((Umb GET '/api/channels').items)
  $teamsCh = (First ($chs | Where-Object { $_.name -eq 'Lab Teams' })).id
  $zoomCh = (First ($chs | Where-Object { $_.name -eq 'Lab Zoom' })).id
  $types = @($chs | Where-Object { $_.enabled } | ForEach-Object { $_.type })
  Check 'Channels Lab Teams (teams) and Lab Zoom (zoom) are enabled' { $types -contains 'teams' -and $types -contains 'zoom' }
  $r = Umb POST "/api/channels/$teamsCh/test"
  Check 'Teams test message delivered (200)' { $r.ok -eq $true -and $r.status -eq 200 } $r
  $r = Umb POST "/api/channels/$zoomCh/test"
  Check 'Zoom test message delivered (200)' { $r.ok -eq $true -and $r.status -eq 200 } $r
  $okOpen = @()
  for ($i = 0; $i -lt 15; $i++) {
    $okOpen = @(Get-Delivery $osInc | Where-Object { $_.event -eq 'open' -and $_.ok } | ForEach-Object { $_.channel_id })
    if ($okOpen -contains $teamsCh -and $okOpen -contains $zoomCh) { break }
    Start-Sleep -Seconds 2
  }
  Check 'OpenSearch incident -> open message delivered to Teams and Zoom (ok)' { $okOpen -contains $teamsCh -and $okOpen -contains $zoomCh } ($okOpen -join ',')
  $smokeDl = @(Get-Delivery $inc)
  Check 'Zoom channel is limited to lab-shop: no message for the smoke service incident' { @($smokeDl | Where-Object { $_.channel_id -eq $zoomCh }).Count -eq 0 }
  Check 'Teams channel (all services) got the smoke incident' { @($smokeDl | Where-Object { $_.channel_id -eq $teamsCh -and $_.ok }).Count -gt 0 }
  [void](Umb POST "/api/incidents/$osInc/resolve")
  $resolved = @()
  for ($i = 0; $i -lt 10; $i++) {
    $resolved = @(Get-Delivery $osInc | Where-Object { $_.event -eq 'resolve' -and $_.ok })
    if ($resolved.Count -gt 0) { break }
    Start-Sleep -Seconds 2
  }
  Check 'Resolve message delivered' { $resolved.Count -gt 0 }
  $mh = @{}
  if ($metricsToken) { $mh['Authorization'] = "Bearer $metricsToken" }
  $m = Send-Http GET "$UmbUrl/metrics" $null $mh
  Check 'Metrics count deliveries: umbrella_notifications_total{type="teams",result="ok"} > 0' {
    $m.Body -match '(?m)^umbrella_notifications_total\{.*result="ok".*type="teams"\} [1-9]' }

  Section '19. Grafana'
  $gh = (Send-Http GET "$GrafanaUrl/api/health").Json
  Check "Grafana is up at $GrafanaUrl (database ok)" { $gh.database -eq 'ok' } $gh
  $d = Grafana GET '/api/dashboards/uid/umbrella-incidents'; $st = Status
  Check 'Dashboard umbrella-incidents is provisioned' { $d.meta.provisioned -eq $true -and @($d.dashboard.panels).Count -ge 15 } "status $st"
  $wantUrl = 'http://' + (Get-Setting 'PUBLIC_HOST' 'localhost') + ':' + (Get-Setting 'UMBRELLA_PORT' '8080')
  $umbVar = First (@($d.dashboard.templating.list) | Where-Object { $_.name -eq 'umbrella' })
  Check 'Dashboard links to Umbrella at the public address' { $umbVar.query -eq $wantUrl } $umbVar
  $r = Grafana GET '/api/datasources/uid/prometheus/health'
  Check 'Data source Prometheus: health OK' { $r.status -eq 'OK' } $r
  $q = Grafana GET '/api/datasources/proxy/uid/prometheus/api/v1/query?query=umbrella_up'
  Check 'Grafana reads umbrella_up through Prometheus' { [string](@(@($q.data.result)[0].value)[1]) -eq '1' } $q
  $r = Grafana GET '/api/datasources/uid/umbrella-api/health'
  Check 'Data source Umbrella API (Infinity, token of grafana): health OK' { $r.status -eq 'OK' } "$(ConvertTo-Json -InputObject $r -Compress) (the Infinity plugin is installed from grafana.com at start)"
}

if ($Zabbix) {
  Section '20. Zabbix end-to-end'
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

# Clean up what this run created in the access model.
$script:UmbToken = $adminToken
foreach ($u in $ownerId, $viewerId) { if ($u) { [void](Umb DELETE "/api/users/$u") } }
if ($tok.item.id) { [void](Umb DELETE "/api/users/$smokeUid/tokens/$($tok.item.id)") }
Write-Host ''
Write-Host ("Result: {0} passed, {1} failed" -f $script:Pass, $script:Failed.Count) -ForegroundColor White
foreach ($f in $script:Failed) { Write-Host "  - $f" }
if ($script:Failed.Count -gt 0) { exit 1 }
exit 0
