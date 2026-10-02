# Shared helpers for configure.ps1 and smoke-test.ps1.
# Works in Windows PowerShell 5.1 and PowerShell 7 (Windows, Linux, macOS).

Add-Type -AssemblyName System.Net.Http
$ErrorActionPreference = 'Stop'

function Read-DotEnv([string]$Path) {
  if (-not (Test-Path $Path)) { return }
  foreach ($line in Get-Content $Path) {
    if ($line -match '^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*?)\s*$') {
      $v = $Matches[2] -replace '\s+#.*$', ''
      [Environment]::SetEnvironmentVariable($Matches[1], $v.Trim('"'), 'Process')
    }
  }
}

function Get-Setting([string]$Name, [string]$Default = '') {
  $v = [Environment]::GetEnvironmentVariable($Name, 'Process')
  if ([string]::IsNullOrEmpty($v)) { return $Default }
  return $v
}

function Write-Step([string]$Text) { Write-Host "==> $Text" -ForegroundColor Cyan }
function Stop-Lab([string]$Text) { Write-Host "ERROR: $Text" -ForegroundColor Red; exit 1 }

$script:Http = New-Object System.Net.Http.HttpClient
$script:Http.Timeout = [TimeSpan]::FromSeconds(30)
$script:LastStatus = 0
$script:ZbxToken = ''

# Initialize-Lab reads .env from the lab folder and sets the API addresses.
function Initialize-Lab([string]$Dir, [string]$User = 'lab-setup') {
  Read-DotEnv (Join-Path $Dir '.env')
  $script:UmbUrl = Get-Setting 'UMB_URL' ("http://localhost:" + (Get-Setting 'UMBRELLA_PORT' '8080'))
  $script:ZbxUrl = Get-Setting 'ZBX_URL' ("http://localhost:" + (Get-Setting 'ZABBIX_WEB_PORT' '8081'))
  $script:UmbUser = Get-Setting 'UMB_USER' $User
}

function ConvertTo-JsonBody($Value) {
  if ($Value -is [string]) { return $Value }
  return (ConvertTo-Json -InputObject $Value -Depth 20 -Compress)
}

# Send-Http returns @{ Status; Body; Json }. Non-2xx answers are returned, not thrown.
function Send-Http([string]$Method, [string]$Url, $Body = $null, [hashtable]$Headers = @{}) {
  $req = [System.Net.Http.HttpRequestMessage]::new([System.Net.Http.HttpMethod]::new($Method), $Url)
  foreach ($k in $Headers.Keys) { [void]$req.Headers.TryAddWithoutValidation($k, [string]$Headers[$k]) }
  if ($null -ne $Body) {
    $req.Content = [System.Net.Http.StringContent]::new((ConvertTo-JsonBody $Body), [Text.Encoding]::UTF8, 'application/json')
  }
  try {
    $resp = $script:Http.SendAsync($req).GetAwaiter().GetResult()
  } catch {
    $script:LastStatus = 0
    return @{ Status = 0; Body = $_.Exception.Message; Json = $null }
  }
  $text = $resp.Content.ReadAsStringAsync().GetAwaiter().GetResult()
  $script:LastStatus = [int]$resp.StatusCode
  $json = $null
  if ($text -and $text.TrimStart().Length -gt 0 -and '{['.Contains($text.TrimStart()[0])) {
    try { $json = $text | ConvertFrom-Json } catch { }
  }
  return @{ Status = [int]$resp.StatusCode; Body = $text; Json = $json }
}

# Umb calls the Umbrella API and returns the parsed JSON; $script:LastStatus holds the code.
function Umb([string]$Method, [string]$Path, $Body = $null) {
  (Send-Http $Method ($script:UmbUrl + $Path) $Body @{ 'X-Umbrella-User' = $script:UmbUser }).Json
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

function Get-ConnectorId([string]$Name) {
  $c = First (@((Umb GET '/api/connectors').items) | Where-Object { $_.name -eq $Name })
  if ($c) { return $c.id }
  return ''
}

function Get-CiId([string]$Name) {
  $c = First (@((Umb GET ('/api/cis?q=' + [uri]::EscapeDataString($Name))).items) | Where-Object { $_.name -eq $Name })
  if ($c) { return $c.id }
  return ''
}

# Get-LabCi returns the CI id, creating the CI when missing.
function Get-LabCi([string]$Name, [string]$Type, [string]$Team, [string]$Parent = '') {
  $id = Get-CiId $Name
  if (-not $id) {
    $id = (Umb POST '/api/cis' @{ name = $Name; type = $Type; team = $Team; parent = $Parent; description = 'Umbrella lab'
        identities = @(@{ kind = 'hostname'; value = $Name }) }).id
  }
  return $id
}

# Set-WebhookConnector returns the id of a published, running connector with that graph.
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
function New-WebhookGraph([string]$SecretRef, [string]$SeverityField, [string]$Mapping, [string]$Labels, [hashtable]$Event) {
  $ev = @{ severity = '${_severity}' }
  foreach ($k in $Event.Keys) { $ev[$k] = $Event[$k] }
  @{
    nodes = @(
      @{ id = 'n1'; kind = 'trigger.webhook'; x = 40; y = 140; config = @{ auth = 'token'; secret_ref = $SecretRef } },
      @{ id = 'n2'; kind = 'parse.json'; x = 280; y = 140; config = @{} },
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
