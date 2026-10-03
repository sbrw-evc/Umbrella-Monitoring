<#
.SYNOPSIS
  One-command install of the Umbrella lab on Windows with Docker Desktop.
.DESCRIPTION
  Fetches the repository, writes .env with random secrets, initializes and
  unseals OpenBao, builds and starts the lab, connects it to Umbrella and runs
  the test. configure.sh and smoke-test.sh run in the toolbox container, so
  the logic is the same as on Linux. Works in ConstrainedLanguage mode.

    irm https://raw.githubusercontent.com/sbrw-evc/Umbrella-Monitoring/feature/mvp-app/deploy/lab/install.ps1 -OutFile $env:TEMP\umbrella-install.ps1; powershell -ExecutionPolicy Bypass -File $env:TEMP\umbrella-install.ps1
#>
param(
  [string]$Dir = $(if ($env:UMB_DIR) { $env:UMB_DIR } else { Join-Path $HOME 'umbrella-monitoring' }),
  [string]$Branch = $(if ($env:UMB_BRANCH) { $env:UMB_BRANCH } else { 'feature/mvp-app' }),
  [string]$Repo = 'sbrw-evc/Umbrella-Monitoring',
  [switch]$SkipTest
)
$ErrorActionPreference = 'Stop'

if ($PSScriptRoot -and (Test-Path (Join-Path $PSScriptRoot 'lib.ps1')) -and (Test-Path (Join-Path $PSScriptRoot '..\..\app'))) {
  $lab = $PSScriptRoot
  . (Join-Path $lab 'lib.ps1')
  Assert-Docker
} else {
  if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
    Write-Host 'git is required: winget install -e --id Git.Git' -ForegroundColor Red
    exit 1
  }
  if (Test-Path (Join-Path $Dir '.git')) {
    & git -C $Dir fetch -q origin $Branch
    & git -C $Dir checkout -q -B $Branch FETCH_HEAD
  } else {
    & git -c core.autocrlf=false clone -q --branch $Branch "https://github.com/$Repo.git" $Dir
  }
  if ($LASTEXITCODE -ne 0) { Write-Host "git failed with code $LASTEXITCODE" -ForegroundColor Red; exit 1 }
  $lab = Join-Path $Dir 'deploy\lab'
  . (Join-Path $lab 'lib.ps1')
  Assert-Docker
}
Set-Location $lab

Test-AppArmor $lab

Write-Step 'Settings and secrets (.env)'
Update-DotEnv (Join-Path $lab '.env')
$cfg = Read-DotEnv (Join-Path $lab '.env')

Write-Step 'OpenBao: start, initialize and unseal'
Start-OpenBao $lab
Write-Step "  unseal keys and root token: $lab\secrets\openbao\init.txt"

Write-Step 'Building and starting containers (the first build takes several minutes)'
$env:UMBRELLA_VERSION = Get-NativeOutput git @('-C', $lab, 'describe', '--tags', '--always', '--dirty')
if (-not $env:UMBRELLA_VERSION) { $env:UMBRELLA_VERSION = 'lab' }
Invoke-Native docker @('compose', 'build', '--pull', 'umbrella', 'fluentd')
Invoke-Native docker @('compose', 'up', '-d')

Write-Step 'Connecting the lab to Umbrella (toolbox container)'
if ((Invoke-Toolbox 'configure.sh') -ne 0) { Stop-Lab 'configure.sh failed' }

$test = 0
if (-not $SkipTest) {
  Write-Step 'Testing the lab'
  $test = Invoke-Toolbox 'smoke-test.sh' @('--all')
}

$h = $cfg['PUBLIC_HOST']
Write-Host ''
Write-Host "Umbrella:    http://${h}:$($cfg['UMBRELLA_PORT'])   admin / UMBRELLA_ADMIN_PASSWORD"
Write-Host "Grafana:     http://${h}:$($cfg['GRAFANA_PORT'])   admin / GRAFANA_ADMIN_PASSWORD"
Write-Host "Zabbix:      http://${h}:$($cfg['ZABBIX_WEB_PORT'])   Admin / ZABBIX_ADMIN_PASSWORD"
Write-Host "NetBox:      http://${h}:$($cfg['NETBOX_PORT'])   admin / NETBOX_ADMIN_PASSWORD"
Write-Host "Prometheus:  http://${h}:$($cfg['PROMETHEUS_PORT'])"
Write-Host "OpenSearch Dashboards:  http://${h}:$($cfg['OSD_PORT'])"
Write-Host "Passwords and tokens: $lab\.env. Update without losing data: .\update.ps1"
exit $test
