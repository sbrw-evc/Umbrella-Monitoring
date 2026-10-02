<#
.SYNOPSIS
  One-command Umbrella + Zabbix lab install for Windows (PowerShell version of install.sh).
.DESCRIPTION
  Needs Docker Desktop (or Docker Engine with the compose plugin). Fetches the
  repository, writes .env with random secrets, starts Umbrella + Zabbix 7.0
  (server, web, agent 2), connects Zabbix to Umbrella (configure.ps1) and runs
  the MVP test (smoke-test.ps1 -Zabbix). Running it again updates the code and
  restarts the containers; the passwords in .env are kept.

  One line, in PowerShell:
    irm https://raw.githubusercontent.com/sbrw-evc/Umbrella-Monitoring/feature/mvp-app/deploy/zabbix-lab/install.ps1 -OutFile $env:TEMP\umbrella-install.ps1; powershell -ExecutionPolicy Bypass -File $env:TEMP\umbrella-install.ps1

  From a checkout:
    powershell -ExecutionPolicy Bypass -File deploy\zabbix-lab\install.ps1

  Settings come from parameters or environment variables with the same names:
  UMB_DIR, UMB_BRANCH, PUBLIC_HOST, UMBRELLA_PORT, ZABBIX_WEB_PORT,
  UMBRELLA_DEMO, UMBRELLA_PD_ROUTING_KEY.
#>
param(
  [string]$Dir = $(if ($env:UMB_DIR) { $env:UMB_DIR } else { Join-Path $HOME 'umbrella-monitoring' }),
  [string]$Branch = $(if ($env:UMB_BRANCH) { $env:UMB_BRANCH } else { 'feature/mvp-app' }),
  [string]$Repo = 'sbrw-evc/Umbrella-Monitoring',
  [switch]$NoBuild,   # use an existing umbrella-mvp:lab image
  [switch]$SkipTest
)
$ErrorActionPreference = 'Stop'
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

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
  $zip = Join-Path ([IO.Path]::GetTempPath()) 'umbrella-monitoring.zip'
  $tmp = Join-Path ([IO.Path]::GetTempPath()) ('umbrella-' + [guid]::NewGuid())
  Invoke-WebRequest -UseBasicParsing "https://codeload.github.com/$Repo/zip/refs/heads/$Branch" -OutFile $zip
  Expand-Archive $zip $tmp
  $src = Get-ChildItem $tmp | Select-Object -First 1
  $keep = $null
  $envFile = Join-Path $Dir 'deploy\zabbix-lab\.env'
  if (Test-Path $envFile) { $keep = Get-Content $envFile -Raw }
  if (Test-Path $Dir) { Remove-Item $Dir -Recurse -Force }
  Move-Item $src.FullName $Dir
  if ($keep) { [IO.File]::WriteAllText($envFile, $keep) }
  Remove-Item $zip, $tmp -Recurse -Force -ErrorAction SilentlyContinue
  $lab = Join-Path $Dir 'deploy\zabbix-lab'
}
Set-Location $lab

$envPath = Join-Path $lab '.env'
if (-not (Test-Path $envPath)) {
  Write-Step 'Writing .env with random secrets'
  function New-Secret {
    $b = New-Object byte[] 16
    [Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($b)
    -join ($b | ForEach-Object { $_.ToString('x2') })
  }
  function Pick([string]$Name, [string]$Default) {
    $v = [Environment]::GetEnvironmentVariable($Name)
    if ($v) { return $v }
    return $Default
  }
  $ip = Pick 'PUBLIC_HOST' ''
  if (-not $ip) {
    try {
      $ip = [Net.Dns]::GetHostAddresses([Net.Dns]::GetHostName()) |
        Where-Object { $_.AddressFamily -eq 'InterNetwork' -and -not [Net.IPAddress]::IsLoopback($_) } |
        Select-Object -First 1 | ForEach-Object { $_.ToString() }
    } catch { }
    if (-not $ip) { $ip = 'localhost' }
  }
  $lines = @(
    "PUBLIC_HOST=$ip",
    "UMBRELLA_PORT=$(Pick 'UMBRELLA_PORT' '8080')",
    "ZABBIX_WEB_PORT=$(Pick 'ZABBIX_WEB_PORT' '8081')",
    "UMBRELLA_DEMO=$(Pick 'UMBRELLA_DEMO' 'true')",
    "UMBRELLA_PD_ROUTING_KEY=$(Pick 'UMBRELLA_PD_ROUTING_KEY' '')",
    "ZABBIX_DB_PASSWORD=$(New-Secret)",
    "ZABBIX_ADMIN_PASSWORD=$(New-Secret)",
    "ZABBIX_WEBHOOK_TOKEN=$(New-Secret)",
    "SMOKE_WEBHOOK_TOKEN=$(New-Secret)",
    "TZ=$(Pick 'TZ' 'Europe/Moscow')"
  )
  # LF line endings and no BOM, as docker compose expects.
  [IO.File]::WriteAllText($envPath, (($lines -join "`n") + "`n"), (New-Object Text.UTF8Encoding $false))
  if ($IsWindows -or $env:OS -eq 'Windows_NT') {
    [void](Test-Native icacls @($envPath, '/inheritance:r', '/grant:r', "$($env:USERNAME):(R,W)"))
  } else {
    & chmod 600 $envPath
  }
}

if ($NoBuild) {
  Write-Step 'Starting containers (existing image)'
  Invoke-Native docker @('compose', 'up', '-d', '--no-build')
} else {
  Write-Step 'Building and starting containers (first build takes a few minutes)'
  Invoke-Native docker @('compose', 'up', '-d', '--build')
}
& docker compose ps

Write-Step 'Connecting Zabbix to Umbrella'
$global:LASTEXITCODE = 0
& (Join-Path $lab 'configure.ps1')
if ($LASTEXITCODE -ne 0) { Stop-Lab 'configure.ps1 failed' }

$test = 0
if (-not $SkipTest) {
  Write-Step 'Testing MVP functions'
  & (Join-Path $lab 'smoke-test.ps1') -Zabbix
  $test = $LASTEXITCODE
}

. (Join-Path $lab 'lib.ps1')
Read-DotEnv $envPath
$h = Get-Setting 'PUBLIC_HOST' 'localhost'
Write-Host ''
Write-Host "Umbrella:  http://${h}:$(Get-Setting 'UMBRELLA_PORT' '8080')"
Write-Host "Zabbix:    http://${h}:$(Get-Setting 'ZABBIX_WEB_PORT' '8081')   login Admin, password: ZABBIX_ADMIN_PASSWORD in $envPath"
Write-Host "Re-run the test:  $(Join-Path $lab 'smoke-test.ps1') -Zabbix"
Write-Host "After a restart of the umbrella container (data is in memory):  $(Join-Path $lab 'configure.ps1')"
Write-Host 'The lab API has no login: open the ports only to your own addresses.'
exit $test
