$ErrorActionPreference = 'Stop'

function Write-Step([string]$Text) { Write-Host "==> $Text" -ForegroundColor Cyan }
function Stop-Lab([string]$Text) { Write-Host "ERROR: $Text" -ForegroundColor Red; exit 1 }

function Test-Native([string]$Exe, [string[]]$ArgList) {
  $old = $ErrorActionPreference
  $ErrorActionPreference = 'Continue'
  try { & $Exe @ArgList *> $null; return ($LASTEXITCODE -eq 0) } catch { return $false } finally { $ErrorActionPreference = $old }
}

function Invoke-Native([string]$Exe, [string[]]$ArgList) {
  $old = $ErrorActionPreference
  $ErrorActionPreference = 'Continue'
  try { & $Exe @ArgList } finally { $ErrorActionPreference = $old }
  if ($LASTEXITCODE -ne 0) { Stop-Lab "$Exe $($ArgList -join ' ') failed with code $LASTEXITCODE" }
}

function Get-NativeOutput([string]$Exe, [string[]]$ArgList) {
  $old = $ErrorActionPreference
  $ErrorActionPreference = 'Continue'
  try { return ((& $Exe @ArgList 2>$null) -join "`n").Trim() } catch { return '' } finally { $ErrorActionPreference = $old }
}

function New-Secret([int]$Length = 32) { (((New-Guid).Guid + (New-Guid).Guid) -replace '-', '').Substring(0, $Length) }
function New-Password { 'Lab' + ((New-Guid).Guid -replace '-', '').Substring(0, 24) + '7' }

function Pick([string]$Name, [string]$Default) {
  $item = Get-Item -Path "Env:$Name" -ErrorAction SilentlyContinue
  if ($item -and $item.Value) { return $item.Value }
  return $Default
}

function Read-DotEnv([string]$Path) {
  $map = @{}
  if (-not (Test-Path $Path)) { return $map }
  foreach ($line in Get-Content $Path) {
    if ($line -match '^\s*([A-Za-z_][A-Za-z0-9_]*)=(.*)$') { $map[$Matches[1]] = $Matches[2] }
  }
  return $map
}

function Get-PublicHost {
  $ip = Pick 'PUBLIC_HOST' ''
  if (-not $ip -and (Get-Command Get-NetIPAddress -ErrorAction SilentlyContinue)) {
    $ip = Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue |
      Where-Object { $_.IPAddress -notlike '127.*' -and $_.IPAddress -notlike '169.254.*' -and $_.InterfaceAlias -notlike 'vEthernet*' } |
      Select-Object -First 1 -ExpandProperty IPAddress
  }
  if (-not $ip) { $ip = 'localhost' }
  return $ip
}

function Update-DotEnv([string]$Path) {
  $have = Read-DotEnv $Path
  $wanted = [ordered]@{
    PUBLIC_HOST              = { Get-PublicHost }
    LAB_HOST                 = { Pick 'LAB_HOST' 'lab-host-1' }
    UMBRELLA_PORT            = { Pick 'UMBRELLA_PORT' '8080' }
    ZABBIX_WEB_PORT          = { Pick 'ZABBIX_WEB_PORT' '8081' }
    GRAFANA_PORT             = { Pick 'GRAFANA_PORT' '3000' }
    PROMETHEUS_PORT          = { Pick 'PROMETHEUS_PORT' '9090' }
    OSD_PORT                 = { Pick 'OSD_PORT' '5601' }
    NETBOX_PORT              = { Pick 'NETBOX_PORT' '8000' }
    OPENBAO_PORT             = { Pick 'OPENBAO_PORT' '8200' }
    TZ                       = { Pick 'TZ' 'Europe/Moscow' }
    PG_SLOW_MS               = { Pick 'PG_SLOW_MS' '20' }
    UMBRELLA_ADMIN_USER      = { Pick 'UMBRELLA_ADMIN_USER' 'admin' }
    UMBRELLA_ADMIN_PASSWORD  = { New-Password }
    LAB_OWNER_PASSWORD       = { New-Password }
    UMBRELLA_GRAFANA_TOKEN   = { 'umb_' + (New-Secret 48) }
    UMBRELLA_METRICS_TOKEN   = { New-Secret }
    PROMETHEUS_WEBHOOK_TOKEN = { New-Secret }
    UMBRELLA_DB_PASSWORD     = { New-Secret }
    ZABBIX_DB_PASSWORD       = { New-Secret }
    ZABBIX_ADMIN_PASSWORD    = { New-Secret }
    GRAFANA_ADMIN_PASSWORD   = { New-Secret }
    NETBOX_DB_PASSWORD       = { New-Secret }
    NETBOX_REDIS_PASSWORD    = { New-Secret }
    NETBOX_SECRET_KEY        = { New-Secret 64 }
    NETBOX_ADMIN_PASSWORD    = { New-Password }
    NETBOX_API_TOKEN         = { New-Secret 40 }
    UMBRELLA_PD_ROUTING_KEY  = { Pick 'UMBRELLA_PD_ROUTING_KEY' '' }
    UMBRELLA_PD_API_TOKEN    = { Pick 'UMBRELLA_PD_API_TOKEN' '' }
    UMBRELLA_PD_REGION       = { Pick 'UMBRELLA_PD_REGION' 'us' }
    TEAMS_WEBHOOK_URL        = { Pick 'TEAMS_WEBHOOK_URL' '' }
    ZOOM_WEBHOOK_URL         = { Pick 'ZOOM_WEBHOOK_URL' '' }
    ZOOM_VERIFICATION_TOKEN  = { Pick 'ZOOM_VERIFICATION_TOKEN' '' }
  }
  $text = ''
  if (Test-Path $Path) { $text = (Get-Content $Path -Raw) -replace "`r", '' }
  if ($text -and -not $text.EndsWith("`n")) { $text += "`n" }
  foreach ($k in $wanted.Keys) {
    if ($have.ContainsKey($k)) { continue }
    $text += "$k=$(& $wanted[$k])`n"
    Write-Step "  .env: $k"
  }
  Set-Content -Path $Path -Value $text -NoNewline -Encoding ascii
}

function Initialize-Secrets([string]$Lab) {
  foreach ($d in @('secrets', 'secrets\openbao', 'secrets\umbrella')) {
    $p = Join-Path $Lab $d
    if (-not (Test-Path $p)) { New-Item -ItemType Directory -Path $p | Out-Null }
  }
}

function Get-ContainerHealth([string]$Service) {
  $id = Get-NativeOutput docker @('compose', 'ps', '-q', $Service)
  if (-not $id) { return '' }
  return Get-NativeOutput docker @('inspect', '-f', '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}', $id)
}

function Wait-Healthy([string]$Service, [int]$Tries = 60) {
  for ($i = 0; $i -lt $Tries; $i++) {
    if ((Get-ContainerHealth $Service) -eq 'healthy') { return $true }
    Start-Sleep -Seconds 3
  }
  return $false
}

function Start-OpenBao([string]$Lab) {
  Initialize-Secrets $Lab
  Invoke-Native docker @('compose', 'up', '-d', 'openbao', 'openbao-init')
  if (-not (Wait-Healthy 'openbao-init' 100)) {
    Invoke-Native docker @('compose', 'logs', '--tail', '20', 'openbao-init')
    Stop-Lab 'OpenBao is not ready'
  }
}

function Invoke-Toolbox([string]$Script, [string[]]$ArgList = @()) {
  $a = @('compose', '--profile', 'tools', 'run', '--rm', '--build', 'toolbox', "./$Script") + $ArgList
  $old = $ErrorActionPreference
  $ErrorActionPreference = 'Continue'
  try { & docker @a } finally { $ErrorActionPreference = $old }
  return $LASTEXITCODE
}

function Get-UmbrellaVersion([string]$Port) {
  try {
    $r = Invoke-WebRequest -UseBasicParsing -Method Head -Uri "http://localhost:$Port/healthz" -TimeoutSec 5
    return [string]$r.Headers['X-Umbrella-Version']
  } catch { return '' }
}

function Assert-Docker {
  if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    Write-Host 'Docker is not installed. Install Docker Desktop and run this script again:'
    Write-Host '  winget install -e --id Docker.DockerDesktop'
    exit 1
  }
  if (-not (Test-Native docker @('compose', 'version'))) { Stop-Lab 'docker compose is missing: update Docker Desktop' }
  if (-not (Test-Native docker @('info'))) {
    $desktop = Join-Path $env:ProgramFiles 'Docker\Docker\Docker Desktop.exe'
    if ($env:ProgramFiles -and (Test-Path $desktop)) {
      Write-Step 'Starting Docker Desktop'
      Start-Process $desktop
      for ($i = 0; $i -lt 60; $i++) {
        Start-Sleep -Seconds 5
        if (Test-Native docker @('info')) { break }
      }
    }
    if (-not (Test-Native docker @('info'))) { Stop-Lab 'Docker engine is not running: start Docker Desktop (Linux containers mode)' }
  }
}

function Test-Probe([string[]]$DockerArgs, [string[]]$Command) {
  $image = Pick 'PROBE_IMAGE' 'alpine:3.22'
  $a = @('run', '--rm', '--pull=missing') + $DockerArgs + @($image) + $Command
  return (Test-Native docker $a)
}

function Test-AppArmor([string]$Lab) {
  Write-Step 'AppArmor'
  $kernel = Get-NativeOutput docker @('run', '--rm', '--pull=missing', (Pick 'PROBE_IMAGE' 'alpine:3.22'), 'cat', '/sys/module/apparmor/parameters/enabled')
  if ($kernel -eq 'Y') { Write-Step '  AppArmor is enabled in the Docker VM kernel' } else { Write-Step '  AppArmor is not enabled in the Docker VM kernel (Docker Desktop / WSL2)' }
  $so = Get-NativeOutput docker @('info', '--format', '{{json .SecurityOptions}}')
  if ($so -notmatch 'apparmor') {
    Write-Step '  Docker does not use AppArmor'
    return
  }
  Write-Step '  Docker uses AppArmor: probing the lab container profiles'
  if (-not (Test-Probe @('--security-opt', 'apparmor=docker-default') @('true'))) { Stop-Lab 'a container under AppArmor profile docker-default does not start' }
  if (-not (Test-Probe @('--privileged') @('true'))) { Stop-Lab 'a privileged container (cAdvisor) does not start under AppArmor' }
  if (-not (Test-Probe @('-v', '/var/run/docker.sock:/var/run/docker.sock:ro') @('test', '-S', '/var/run/docker.sock'))) {
    Stop-Lab 'AppArmor blocks /var/run/docker.sock in containers (Telegraf reads container logs through it)'
  }
  if (-not (Test-Probe @('-v', "${Lab}:/lab:ro") @('test', '-r', '/lab/openbao/init.sh'))) { Stop-Lab "AppArmor or file sharing blocks the bind mount of $Lab" }
  Write-Step '  docker-default, privileged, docker.sock and bind mounts work'
}
