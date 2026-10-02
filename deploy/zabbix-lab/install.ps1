<#
.SYNOPSIS
  One-command Umbrella + Zabbix lab install for Windows (PowerShell version of install.sh).
.DESCRIPTION
  Needs Docker Desktop (or Docker Engine with the compose plugin). Runs in
  ConstrainedLanguage mode (AppLocker / WDAC): only cmdlets, no .NET calls. Fetches the
  repository, writes .env with random secrets, starts Umbrella, Zabbix 7.0
  (server, web, agent 2), Prometheus + Alertmanager + node-exporter, OpenSearch
  + Dashboards, Grafana and the notification sink, connects the sources to
  Umbrella (configure.ps1) and runs the MVP test (smoke-test.ps1 -All).
  Running it again updates the code and restarts the containers; the secrets
  in .env are kept, missing ones are added.

  One line, in PowerShell:
    irm https://raw.githubusercontent.com/sbrw-evc/Umbrella-Monitoring/feature/mvp-app/deploy/zabbix-lab/install.ps1 -OutFile $env:TEMP\umbrella-install.ps1; powershell -ExecutionPolicy Bypass -File $env:TEMP\umbrella-install.ps1

  From a checkout:
    powershell -ExecutionPolicy Bypass -File deploy\zabbix-lab\install.ps1

  Settings come from parameters or environment variables with the same names:
  UMB_DIR, UMB_BRANCH, PUBLIC_HOST, UMBRELLA_PORT, ZABBIX_WEB_PORT,
  GRAFANA_PORT, PROMETHEUS_PORT, OSD_PORT, UMBRELLA_DEMO, UMBRELLA_PD_ROUTING_KEY.
#>
param(
  [string]$Dir = $(if ($env:UMB_DIR) { $env:UMB_DIR } else { Join-Path $HOME 'umbrella-monitoring' }),
  [string]$Branch = $(if ($env:UMB_BRANCH) { $env:UMB_BRANCH } else { 'feature/mvp-app' }),
  [string]$Repo = 'sbrw-evc/Umbrella-Monitoring',
  [switch]$NoBuild,   # use an existing umbrella-mvp:lab image
  [switch]$SkipTest
)
$ErrorActionPreference = 'Stop'

function Write-Step([string]$Text) { Write-Host "==> $Text" -ForegroundColor Cyan }
function Stop-Lab([string]$Text) { Write-Host "ERROR: $Text" -ForegroundColor Red; exit 1 }
# Test-Native runs a command quietly and reports success. Windows PowerShell 5.1
# turns redirected stderr into errors, so the preference is relaxed here.
function Test-Native([string]$Exe, [string[]]$ArgList) {
  $old = $ErrorActionPreference
  $ErrorActionPreference = 'Continue'
  try { & $Exe @ArgList *> $null; return ($LASTEXITCODE -eq 0) } catch { return $false } finally { $ErrorActionPreference = $old }
}
function Invoke-Native([string]$Exe, [string[]]$ArgList) {
  & $Exe @ArgList
  if ($LASTEXITCODE -ne 0) { Stop-Lab "$Exe $($ArgList -join ' ') failed with code $LASTEXITCODE" }
}

Write-Step 'Docker'
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
  Write-Host 'Docker is not installed. Install Docker Desktop and run this script again:'
  Write-Host '  winget install -e --id Docker.DockerDesktop'
  Write-Host '  (or https://docs.docker.com/desktop/setup/install/windows-install/)'
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

# Use the checkout this script lives in, otherwise fetch the repository to $Dir.
$lab = $null
if ($PSScriptRoot -and (Test-Path (Join-Path $PSScriptRoot 'docker-compose.yml')) -and (Test-Path (Join-Path $PSScriptRoot '..\..\app'))) {
  $lab = $PSScriptRoot
} elseif (Get-Command git -ErrorAction SilentlyContinue) {
  if (Test-Path (Join-Path $Dir '.git')) {
    Write-Step "Updating $Dir ($Branch)"
    Invoke-Native git @('-C', $Dir, 'fetch', '-q', 'origin', $Branch)
    Invoke-Native git @('-C', $Dir, 'checkout', '-q', '-B', $Branch, 'FETCH_HEAD')
  } else {
    Write-Step "Cloning $Repo ($Branch) to $Dir"
    Invoke-Native git @('-c', 'core.autocrlf=false', 'clone', '-q', '--branch', $Branch, "https://github.com/$Repo.git", $Dir)
  }
  $lab = Join-Path $Dir 'deploy\zabbix-lab'
} else {
  Write-Step "Downloading $Repo ($Branch) to $Dir (git not found)"
  $tempDir = $env:TEMP
  if (-not $tempDir) { $tempDir = '/tmp' }
  $zip = Join-Path $tempDir 'umbrella-monitoring.zip'
  $tmp = Join-Path $tempDir ('umbrella-' + (New-Guid).Guid)
  Invoke-WebRequest -UseBasicParsing "https://codeload.github.com/$Repo/zip/refs/heads/$Branch" -OutFile $zip
  # tar (built into Windows 10 and later) unpacks zip without .NET calls;
  # Expand-Archive is the fallback.
  New-Item -ItemType Directory -Path $tmp | Out-Null
  $tar = ''
  if ($env:SystemRoot) { $tar = Join-Path $env:SystemRoot 'System32\tar.exe' }
  if ($tar -and (Test-Path $tar)) {
    Invoke-Native $tar @('-xf', $zip, '-C', $tmp)
  } else {
    Expand-Archive $zip $tmp
  }
  $src = Get-ChildItem $tmp | Select-Object -First 1
  $keep = $null
  $envFile = Join-Path $Dir 'deploy\zabbix-lab\.env'
  if (Test-Path $envFile) { $keep = Get-Content $envFile -Raw }
  if (Test-Path $Dir) { Remove-Item $Dir -Recurse -Force }
  Move-Item $src.FullName $Dir
  if ($keep) { Set-Content -Path $envFile -Value $keep -NoNewline -Encoding ascii }
  Remove-Item $zip, $tmp -Recurse -Force -ErrorAction SilentlyContinue
  $lab = Join-Path $Dir 'deploy\zabbix-lab'
}
Set-Location $lab

$envPath = Join-Path $lab '.env'
# New-Guid draws from the system crypto RNG; two GUIDs give 64 hex chars.
function New-Secret { ((New-Guid).Guid + (New-Guid).Guid) -replace '-', '' }
# Umbrella password policy: 10+ characters, letters and digits.
function New-Password { 'Lab' + ((New-Guid).Guid -replace '-', '').Substring(0, 24) + '7' }
function Pick([string]$Name, [string]$Default) {
  $item = Get-Item -Path "Env:$Name" -ErrorAction SilentlyContinue
  if ($item -and $item.Value) { return $item.Value }
  return $Default
}
$newEnv = $false
if (-not (Test-Path $envPath)) {
  Write-Step 'Writing .env with random secrets'
  $newEnv = $true
  $ip = Pick 'PUBLIC_HOST' ''
  if (-not $ip -and (Get-Command Get-NetIPAddress -ErrorAction SilentlyContinue)) {
    $ip = Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue |
      Where-Object { $_.IPAddress -notlike '127.*' -and $_.IPAddress -notlike '169.254.*' -and $_.InterfaceAlias -notlike 'vEthernet*' } |
      Select-Object -First 1 -ExpandProperty IPAddress
  }
  if (-not $ip -and (Get-Command hostname -CommandType Application -ErrorAction SilentlyContinue) -and -not ($env:OS -eq 'Windows_NT')) {
    $ip = [string](@(& hostname -I 2>$null) | Select-Object -First 1)
    $ip = ($ip.Trim() -split '\s+')[0]
  }
  if (-not $ip) { $ip = 'localhost' }
  $lines = @(
    "PUBLIC_HOST=$ip",
    "UMBRELLA_PORT=$(Pick 'UMBRELLA_PORT' '8080')",
    "ZABBIX_WEB_PORT=$(Pick 'ZABBIX_WEB_PORT' '8081')",
    "UMBRELLA_DEMO=$(Pick 'UMBRELLA_DEMO' 'false')",
    "UMBRELLA_PD_ROUTING_KEY=$(Pick 'UMBRELLA_PD_ROUTING_KEY' '')",
    "ZABBIX_DB_PASSWORD=$(New-Secret)",
    "ZABBIX_ADMIN_PASSWORD=$(New-Secret)",
    "ZABBIX_WEBHOOK_TOKEN=$(New-Secret)",
    "SMOKE_WEBHOOK_TOKEN=$(New-Secret)",
    "TZ=$(Pick 'TZ' 'Europe/Moscow')"
  )
  # LF line endings and no BOM, as docker compose expects.
  Set-Content -Path $envPath -Value (($lines -join "`n") + "`n") -NoNewline -Encoding ascii
}
# Settings added after the first version of the lab: appended when missing,
# so an existing .env keeps its passwords.
$text = Get-Content $envPath -Raw
if (-not $text) { $text = '' }
if ($text.Length -gt 0 -and -not $text.EndsWith("`n")) { $text += "`n" }
$added = @()
$want = @(
  @('GRAFANA_PORT', (Pick 'GRAFANA_PORT' '3000')),
  @('PROMETHEUS_PORT', (Pick 'PROMETHEUS_PORT' '9090')),
  @('OSD_PORT', (Pick 'OSD_PORT' '5601')),
  @('UMBRELLA_ADMIN_USER', (Pick 'UMBRELLA_ADMIN_USER' 'admin')),
  @('UMBRELLA_ADMIN_PASSWORD', (New-Password)),
  @('LAB_OWNER_PASSWORD', (New-Password)),
  @('GRAFANA_ADMIN_PASSWORD', (New-Secret).Substring(0, 32)),
  @('UMBRELLA_GRAFANA_TOKEN', ('umb_' + (New-Secret).Substring(0, 48))),
  @('UMBRELLA_METRICS_TOKEN', (New-Secret).Substring(0, 32)),
  @('PROMETHEUS_WEBHOOK_TOKEN', (New-Secret).Substring(0, 32)),
  @('LAB_ZOOM_TOKEN', (New-Secret).Substring(0, 32))
)
foreach ($kv in $want) {
  if ($text -notmatch ('(?m)^' + $kv[0] + '=')) {
    $text += $kv[0] + '=' + $kv[1] + "`n"
    $added += $kv[0]
  }
}
if ($added.Count -gt 0) {
  Set-Content -Path $envPath -Value $text -NoNewline -Encoding ascii
  Write-Step ('.env: added ' + ($added -join ', '))
}
if ($newEnv) {
  if ($IsWindows -or $env:OS -eq 'Windows_NT') {
    [void](Test-Native icacls @($envPath, '/inheritance:r', '/grant:r', "$($env:USERNAME):(R,W)"))
  } else {
    & chmod 600 $envPath
  }
}

# OpenSearch wants vm.max_map_count >= 262144. On Linux (pwsh) it is set and
# kept across reboots; Docker Desktop runs containers in a WSL2 VM that
# Windows cannot configure from here, so only a hint is printed.
if ($IsLinux) {
  $cur = 0
  $mmc = '/proc/sys/vm/max_map_count'
  if (Test-Path $mmc) { $cur = [int](Get-Content $mmc -Raw).Trim() }
  if ($cur -lt 262144) {
    Write-Step 'sysctl vm.max_map_count=262144 (OpenSearch)'
    if (-not (Test-Native sysctl @('-q', '-w', 'vm.max_map_count=262144'))) { Write-Host '  could not set it (run as root); OpenSearch may refuse to start' }
  }
  if ((Test-Path '/etc/sysctl.d') -and -not (Test-Path '/etc/sysctl.d/99-umbrella-opensearch.conf')) {
    try { Set-Content -Path '/etc/sysctl.d/99-umbrella-opensearch.conf' -Value "vm.max_map_count = 262144`n" -NoNewline -Encoding ascii } catch { }
  }
} else {
  Write-Host 'OpenSearch needs vm.max_map_count=262144 in the Docker VM. If the opensearch container stops, run:'
  Write-Host '  wsl -d docker-desktop sysctl -w vm.max_map_count=262144'
  Write-Host '  (to keep it after a restart, add "kernelCommandLine = sysctl.vm.max_map_count=262144" under [wsl2] in %USERPROFILE%\.wslconfig)'
}

if ($NoBuild) {
  Write-Step 'Starting containers (existing image)'
  Invoke-Native docker @('compose', 'up', '-d', '--no-build')
} else {
  Write-Step 'Building and starting containers (first build takes a few minutes)'
  Invoke-Native docker @('compose', 'up', '-d', '--build')
}
& docker compose ps

Write-Step 'Connecting the lab sources to Umbrella'
$global:LASTEXITCODE = 0
& (Join-Path $lab 'configure.ps1')
if ($LASTEXITCODE -ne 0) { Stop-Lab 'configure.ps1 failed' }

$test = 0
if (-not $SkipTest) {
  Write-Step 'Testing MVP functions'
  & (Join-Path $lab 'smoke-test.ps1') -All
  $test = $LASTEXITCODE
}

. (Join-Path $lab 'lib.ps1')
Read-DotEnv $envPath
$h = Get-Setting 'PUBLIC_HOST' 'localhost'
Write-Host ''
Write-Host "Umbrella:    http://${h}:$(Get-Setting 'UMBRELLA_PORT' '8080')   login $(Get-Setting 'UMBRELLA_ADMIN_USER' 'admin'), password UMBRELLA_ADMIN_PASSWORD"
Write-Host '             owner of lab-shop: lab-owner, password LAB_OWNER_PASSWORD'
Write-Host "Grafana:     http://${h}:$(Get-Setting 'GRAFANA_PORT' '3000')   login admin, password GRAFANA_ADMIN_PASSWORD"
Write-Host "Zabbix:      http://${h}:$(Get-Setting 'ZABBIX_WEB_PORT' '8081')   login Admin, password ZABBIX_ADMIN_PASSWORD"
Write-Host "Prometheus:  http://${h}:$(Get-Setting 'PROMETHEUS_PORT' '9090')   (no login)"
Write-Host "OpenSearch Dashboards:  http://${h}:$(Get-Setting 'OSD_PORT' '5601')   (no login)"
Write-Host "All passwords and tokens are in $envPath"
Write-Host "Re-run the test:  $(Join-Path $lab 'smoke-test.ps1') -All"
Write-Host "After a restart of the umbrella container (data is in memory):  $(Join-Path $lab 'configure.ps1')"
Write-Host 'Prometheus and OpenSearch Dashboards have no login: open the ports only to your own addresses.'
exit $test
