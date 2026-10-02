# Shared helpers for configure.ps1 and smoke-test.ps1.
# Written for ConstrainedLanguage mode (AppLocker / WDAC): only cmdlets and
# core types, no Add-Type and no .NET method calls. Works in Windows
# PowerShell 5.1 and PowerShell 7 (Windows, Linux, macOS).

$ErrorActionPreference = 'Stop'

function Get-Setting([string]$Name, [string]$Default = '') {
  $item = Get-Item -Path "Env:$Name" -ErrorAction SilentlyContinue
  if ($null -eq $item -or $item.Value -eq '') { return $Default }
  return $item.Value
}

function Read-DotEnv([string]$Path) {
  if (-not (Test-Path $Path)) { return }
  foreach ($line in Get-Content $Path) {
    if ($line -match '^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*?)\s*$') {
      $name = $Matches[1]
      $v = ($Matches[2] -replace '\s+#.*$', '').Trim('"')
      # Variables already set in the session win over .env, as in docker compose.
      if ((Get-Setting $name) -eq '') { Set-Item -Path "Env:$name" -Value $v }
    }
  }
}

function Write-Step([string]$Text) { Write-Host "==> $Text" -ForegroundColor Cyan }
function Stop-Lab([string]$Text) { Write-Host "ERROR: $Text" -ForegroundColor Red; exit 1 }

$script:LastStatus = 0
$script:ZbxToken = ''
# Bearer token of the signed-in Umbrella user (Connect-Umbrella) or an API token.
$script:UmbToken = ''

# Initialize-Lab reads .env from the lab folder and sets the API addresses.
function Initialize-Lab([string]$Dir) {
  Read-DotEnv (Join-Path $Dir '.env')
  $script:UmbUrl = Get-Setting 'UMB_URL' ("http://localhost:" + (Get-Setting 'UMBRELLA_PORT' '8080'))
  $script:ZbxUrl = Get-Setting 'ZBX_URL' ("http://localhost:" + (Get-Setting 'ZABBIX_WEB_PORT' '8081'))
  $script:PromUrl = Get-Setting 'PROM_URL' ("http://localhost:" + (Get-Setting 'PROMETHEUS_PORT' '9090'))
  $script:GrafanaUrl = Get-Setting 'GRAFANA_URL' ("http://localhost:" + (Get-Setting 'GRAFANA_PORT' '3000'))
  $script:OsdUrl = Get-Setting 'OSD_URL' ("http://localhost:" + (Get-Setting 'OSD_PORT' '5601'))
}

# ConvertTo-Base64Ascii encodes an ASCII string without .NET calls (for basic auth).
function ConvertTo-Base64Ascii([string]$Text) {
  $abc = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/'
  $out = ''
  for ($i = 0; $i -lt $Text.Length; $i += 3) {
    $n = [int][char]$Text[$i] * 65536
    if ($i + 1 -lt $Text.Length) { $n += [int][char]$Text[$i + 1] * 256 }
    if ($i + 2 -lt $Text.Length) { $n += [int][char]$Text[$i + 2] }
    $out += $abc[($n -shr 18) -band 63]
    $out += $abc[($n -shr 12) -band 63]
    if ($i + 1 -lt $Text.Length) { $out += $abc[($n -shr 6) -band 63] } else { $out += '=' }
    if ($i + 2 -lt $Text.Length) { $out += $abc[$n -band 63] } else { $out += '=' }
  }
  return $out
}

# ConvertTo-UrlPart percent-encodes the characters that matter in a query value.
function ConvertTo-UrlPart([string]$Text) {
  return (($Text -replace '%', '%25') -replace '/', '%2F' -replace '\?', '%3F' -replace '=', '%3D' -replace '&', '%26' -replace ' ', '%20')
}

function ConvertTo-JsonBody($Value) {
  if ($Value -is [string]) { return $Value }
  return (ConvertTo-Json -InputObject $Value -Depth 20 -Compress)
}

function ConvertFrom-JsonText([string]$Text) {
  if (-not $Text) { return $null }
  $t = $Text.TrimStart()
  if ($t.Length -eq 0 -or -not ($t.StartsWith('{') -or $t.StartsWith('['))) { return $null }
  try { return ($Text | ConvertFrom-Json) } catch { return $null }
}

# Send-Http returns @{ Status; Body; Json }. Non-2xx answers are returned, not
# thrown; a connection failure gives Status 0.
function Send-Http([string]$Method, [string]$Url, $Body = $null, [hashtable]$Headers = @{}) {
  $req = @{ Method = $Method; Uri = $Url; Headers = $Headers; UseBasicParsing = $true; TimeoutSec = 30 }
  if ($null -ne $Body) {
    $req['Body'] = ConvertTo-JsonBody $Body
    $req['ContentType'] = 'application/json; charset=utf-8'
  }
  $status = 0
  $text = ''
  if ($PSVersionTable.PSVersion.Major -ge 7) {
    $req['SkipHttpErrorCheck'] = $true
    try {
      $resp = Invoke-WebRequest @req
      $status = [int]$resp.StatusCode
      $text = [string]$resp.Content
    } catch { $text = $_.Exception.Message }
  } else {
    # Windows PowerShell 5.1 throws on non-2xx; the code and body are on the error.
    try {
      $resp = Invoke-WebRequest @req
      $status = [int]$resp.StatusCode
      $text = [string]$resp.Content
    } catch {
      if ($_.Exception.Response) { $status = [int]$_.Exception.Response.StatusCode }
      if ($_.ErrorDetails) { $text = [string]$_.ErrorDetails.Message } else { $text = $_.Exception.Message }
    }
  }
  $script:LastStatus = $status
  return @{ Status = $status; Body = $text; Json = (ConvertFrom-JsonText $text) }
}

# Umb calls the Umbrella API as $script:UmbToken and returns the parsed JSON;
# $script:LastStatus holds the code.
function Umb([string]$Method, [string]$Path, $Body = $null) {
  Umb-As $script:UmbToken $Method $Path $Body
}

# Umb-As calls the Umbrella API with another bearer token ('' = no sign-in).
function Umb-As([string]$Token, [string]$Method, [string]$Path, $Body = $null) {
  $h = @{}
  if ($Token) { $h['Authorization'] = "Bearer $Token" }
  (Send-Http $Method ($script:UmbUrl + $Path) $Body $h).Json
}

# Get-UmbToken returns a bearer session token (POST /api/auth/token) or ''.
function Get-UmbToken([string]$User, [string]$Password) {
  $r = Send-Http 'POST' ($script:UmbUrl + '/api/auth/token') @{ username = $User; password = $Password }
  if ($r.Status -eq 200 -and $r.Json.token) { return [string]$r.Json.token }
  return ''
}

# Connect-Umbrella signs in and keeps the token for Umb.
function Connect-Umbrella([string]$User, [string]$Password) {
  $r = Send-Http 'POST' ($script:UmbUrl + '/api/auth/token') @{ username = $User; password = $Password }
  if ($r.Status -ne 200 -or -not $r.Json.token) { return $false }
  if ($r.Json.must_change_password) {
    Stop-Lab "Umbrella asks $User to change the password: sign in to the web UI, change it and put it into UMBRELLA_ADMIN_PASSWORD in .env"
  }
  $script:UmbToken = [string]$r.Json.token
  return $true
}

# Invoke-OpenSearch calls OpenSearch through the OpenSearch Dashboards console
# proxy (OpenSearch itself is reachable only inside the lab network).
function Invoke-OpenSearch([string]$Method, [string]$Path, $Body = $null) {
  (Send-Http 'POST' ($script:OsdUrl + '/api/console/proxy?path=' + (ConvertTo-UrlPart $Path) + "&method=$Method") $Body @{ 'osd-xsrf' = 'true' }).Json
}

# Osd calls the OpenSearch Dashboards API.
function Osd([string]$Method, [string]$Path, $Body = $null) {
  (Send-Http $Method ($script:OsdUrl + $Path) $Body @{ 'osd-xsrf' = 'true' }).Json
}

# Grafana calls the Grafana API as admin (basic auth, GRAFANA_ADMIN_PASSWORD).
function Grafana([string]$Method, [string]$Path, $Body = $null) {
  $auth = 'Basic ' + (ConvertTo-Base64Ascii ('admin:' + (Get-Setting 'GRAFANA_ADMIN_PASSWORD')))
  (Send-Http $Method ($script:GrafanaUrl + $Path) $Body @{ Authorization = $auth }).Json
}

function Send-Ingest([string]$ConnectorId, [string]$Token, $Body) {
  $h = @{}
  if ($Token) { $h['X-Umbrella-Token'] = $Token }
  (Send-Http 'POST' ($script:UmbUrl + "/api/ingest/$ConnectorId") $Body $h).Json
}

function Wait-Http([string]$Url, [int]$Seconds) {
  $deadline = (Get-Date).AddSeconds($Seconds)
  while ((Get-Date) -lt $deadline) {
    $r = Send-Http 'GET' $Url
    if ($r.Status -ge 200 -and $r.Status -lt 400) { return $true }
    Start-Sleep -Seconds 3
  }
  return $false
}

# Zbx calls Zabbix JSON-RPC and returns .result; throws on .error.
function Zbx([string]$Method, $Params) {
  $h = @{}
  if ($script:ZbxToken) { $h['Authorization'] = "Bearer $($script:ZbxToken)" }
  $r = Send-Http 'POST' ($script:ZbxUrl + '/api_jsonrpc.php') @{ jsonrpc = '2.0'; method = $Method; params = $Params; id = 1 } $h
  if (-not $r.Json) { throw "Zabbix ${Method}: HTTP $($r.Status) $($r.Body)" }
  if ($r.Json.PSObject.Properties['error']) { throw "Zabbix ${Method}: $(ConvertTo-Json -InputObject $r.Json.error -Compress)" }
  return $r.Json.result
}

function Connect-Zabbix([string]$User, [string]$Password) {
  $script:ZbxToken = ''
  try { $t = Zbx 'user.login' @{ username = $User; password = $Password } } catch { return $false }
  if (-not $t) { return $false }
  $script:ZbxToken = [string]$t
  return $true
}

function First($List) { if ($null -eq $List) { return $null }; @($List)[0] }

# Get-ConnectorId returns the id of the connector with that slug or name.
function Get-ConnectorId([string]$Name) {
  $items = @((Umb GET '/api/connectors').items)
  $c = First ($items | Where-Object { $_.slug -eq $Name })
  if (-not $c) { $c = First ($items | Where-Object { $_.name -eq $Name }) }
  if ($c) { return $c.id }
  return ''
}

function Get-UserId([string]$Name) {
  $u = First (@((Umb GET '/api/users').items) | Where-Object { $_.username -eq $Name })
  if ($u) { return $u.id }
  return ''
}

# Set-LabUser creates the user or updates name, roles and services. The
# password is used only on create, so a user who changed it keeps the new one.
function Set-LabUser([hashtable]$User) {
  $id = Get-UserId $User.username
  if (-not $id) {
    $id = (Umb POST '/api/users' $User).id
    if ($script:LastStatus -ne 201) { Stop-Lab "user $($User.username): create answered $($script:LastStatus)" }
  } else {
    $upd = @{}
    foreach ($k in $User.Keys) { if (@('password', 'username', 'must_change_password', 'service') -notcontains $k) { $upd[$k] = $User[$k] } }
    [void](Umb PUT "/api/users/$id" $upd)
    if ($script:LastStatus -ne 200) { Stop-Lab "user $($User.username): update answered $($script:LastStatus)" }
  }
  return $id
}

# Set-LabChannel creates or updates a notification channel by name.
function Set-LabChannel([hashtable]$Channel) {
  $c = First (@((Umb GET '/api/channels').items) | Where-Object { $_.name -eq $Channel.name })
  if (-not $c) {
    $id = (Umb POST '/api/channels' $Channel).id
    if ($script:LastStatus -ne 201) { Stop-Lab "channel $($Channel.name): create answered $($script:LastStatus)" }
    return $id
  }
  [void](Umb PUT "/api/channels/$($c.id)" $Channel)
  if ($script:LastStatus -ne 200) { Stop-Lab "channel $($Channel.name): update answered $($script:LastStatus)" }
  return $c.id
}

function Get-CiId([string]$Name) {
  $c = First (@((Umb GET ('/api/cis?q=' + ($Name -replace '[^A-Za-z0-9._-]', ''))).items) | Where-Object { $_.name -eq $Name })
  if ($c) { return $c.id }
  return ''
}

# Get-LabCi returns the CI id, creating the CI when missing. Parent depends on
# the new CI (business service -> IT service -> host).
function Get-LabCi([string]$Name, [string]$Type, [string]$Team, [string]$Parent = '') {
  $id = Get-CiId $Name
  if (-not $id) {
    $id = (Umb POST '/api/cis' @{ name = $Name; type = $Type; team = $Team; parent = $Parent; description = 'Umbrella lab'
        identities = @(@{ kind = 'hostname'; value = $Name }) }).id
  }
  return $id
}

# Set-Connector returns the id of a published, running connector with that
# slug and graph. It is found by slug, then by name (connectors made by older
# versions of the lab scripts had no slug).
function Set-Connector([string]$Name, [string]$Slug, [string]$Team, [string]$Template, $Graph) {
  $id = Get-ConnectorId $Slug
  if (-not $id) { $id = Get-ConnectorId $Name }
  if (-not $id) {
    $id = (Umb POST '/api/connectors' @{ name = $Name; slug = $Slug; team = $Team; template = $Template }).id
    if ($script:LastStatus -ne 201) { Stop-Lab "connector ${Name}: create answered $($script:LastStatus)" }
  }
  [void](Umb PUT "/api/connectors/$id" @{ name = $Name; slug = $Slug; draft = $Graph })
  if ($script:LastStatus -ne 200) { Stop-Lab "connector ${Name}: update answered $($script:LastStatus)" }
  [void](Umb POST "/api/connectors/$id/publish")
  if ($script:LastStatus -ne 200) { Stop-Lab "connector ${Name}: publish answered $($script:LastStatus)" }
  [void](Umb POST "/api/connectors/$id/start")
  return $id
}

# Set-WebhookConnector: a connector without a slug (the smoke test makes these).
function Set-WebhookConnector([string]$Name, [string]$Team, $Graph) {
  $id = Get-ConnectorId $Name
  if (-not $id) { $id = (Umb POST '/api/connectors' @{ name = $Name; team = $Team; template = 'webhook-json' }).id }
  [void](Umb PUT "/api/connectors/$id" @{ draft = $Graph })
  [void](Umb POST "/api/connectors/$id/publish")
  if ($script:LastStatus -ne 200) { Stop-Lab "connector ${Name}: publish answered $($script:LastStatus)" }
  [void](Umb POST "/api/connectors/$id/start")
  return $id
}

# New-WebhookGraph builds trigger.webhook -> parse.json -> map.severity ->
# enrich.labels -> map.event -> out.event -> ack.response.
function New-WebhookGraph([string]$SecretRef, [string]$SeverityField, [string]$Mapping, [string]$Labels, [hashtable]$Event, [string]$Items = '') {
  $ev = @{ severity = '${_severity}' }
  foreach ($k in $Event.Keys) { $ev[$k] = $Event[$k] }
  $parse = @{}
  if ($Items) { $parse['items'] = $Items }
  @{
    nodes = @(
      @{ id = 'n1'; kind = 'trigger.webhook'; x = 40; y = 140; config = @{ auth = 'token'; secret_ref = $SecretRef } },
      @{ id = 'n2'; kind = 'parse.json'; x = 280; y = 140; config = $parse },
      @{ id = 'n3'; kind = 'map.severity'; x = 520; y = 140; config = @{ field = $SeverityField; mapping = $Mapping } },
      @{ id = 'n4'; kind = 'enrich.labels'; x = 760; y = 140; config = @{ labels = $Labels } },
      @{ id = 'n5'; kind = 'map.event'; x = 40; y = 320; config = $ev },
      @{ id = 'n6'; kind = 'out.event'; x = 300; y = 320; config = @{} },
      @{ id = 'n7'; kind = 'ack.response'; x = 560; y = 320; config = @{ mode = 'http_2xx' } }
    )
    edges = @(
      @{ id = 'e1'; source = 'n1'; target = 'n2' }, @{ id = 'e2'; source = 'n2'; target = 'n3' },
      @{ id = 'e3'; source = 'n3'; target = 'n4' }, @{ id = 'e4'; source = 'n4'; target = 'n5' },
      @{ id = 'e5'; source = 'n5'; target = 'n6' }, @{ id = 'e6'; source = 'n6'; target = 'n7' }
    )
  }
}

# New-PullGraph builds trigger.schedule -> fetch.http -> parse.json ->
# map.severity -> enrich.labels -> map.event -> out.event -> ack.response.
function New-PullGraph([string]$Interval, [hashtable]$Fetch, [string]$Items, [string]$SeverityField, [string]$Mapping, [string]$Labels, [hashtable]$Event) {
  $ev = @{ severity = '${_severity}' }
  foreach ($k in $Event.Keys) { $ev[$k] = $Event[$k] }
  @{
    nodes = @(
      @{ id = 'n1'; kind = 'trigger.schedule'; x = 40; y = 140; config = @{ interval = $Interval } },
      @{ id = 'n2'; kind = 'fetch.http'; x = 280; y = 140; config = $Fetch },
      @{ id = 'n3'; kind = 'parse.json'; x = 520; y = 140; config = @{ items = $Items } },
      @{ id = 'n4'; kind = 'map.severity'; x = 760; y = 140; config = @{ field = $SeverityField; mapping = $Mapping } },
      @{ id = 'n5'; kind = 'enrich.labels'; x = 40; y = 320; config = @{ labels = $Labels } },
      @{ id = 'n6'; kind = 'map.event'; x = 280; y = 320; config = $ev },
      @{ id = 'n7'; kind = 'out.event'; x = 520; y = 320; config = @{} },
      @{ id = 'n8'; kind = 'ack.response'; x = 760; y = 320; config = @{ mode = 'cursor' } }
    )
    edges = @(
      @{ id = 'e1'; source = 'n1'; target = 'n2' }, @{ id = 'e2'; source = 'n2'; target = 'n3' },
      @{ id = 'e3'; source = 'n3'; target = 'n4' }, @{ id = 'e4'; source = 'n4'; target = 'n5' },
      @{ id = 'e5'; source = 'n5'; target = 'n6' }, @{ id = 'e6'; source = 'n6'; target = 'n7' },
      @{ id = 'e7'; source = 'n7'; target = 'n8' }
    )
  }
}
