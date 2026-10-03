<#
.SYNOPSIS
  Updates Umbrella in the lab in place (PowerShell version of update.sh).
.DESCRIPTION
  Data (volume umbrella-data), secrets (OpenBao) and the other services are
  kept; only the umbrella container is replaced. A failed health check rolls
  back to the previous image.
#>
param(
  [switch]$NoPull,
  [switch]$All,
  [switch]$Configure,
  [switch]$Test,
  [switch]$Rollback
)
$ErrorActionPreference = 'Stop'
$lab = $PSScriptRoot
. (Join-Path $lab 'lib.ps1')
Set-Location $lab
Assert-Docker
if (-not (Test-Path (Join-Path $lab '.env'))) { Stop-Lab 'no .env: install the lab first (install.ps1)' }
Test-AppArmor $lab
$image = 'umbrella-mvp:lab'
$prev = 'umbrella-mvp:lab-prev'
$cfg = Read-DotEnv (Join-Path $lab '.env')
$port = $cfg['UMBRELLA_PORT']
if (-not $port) { $port = '8080' }
$before = Get-UmbrellaVersion $port

if ($Rollback) {
  if (-not (Test-Native docker @('image', 'inspect', $prev))) { Stop-Lab "no previous image $prev" }
  Write-Step 'Rolling back to the previous Umbrella image'
  Invoke-Native docker @('image', 'tag', $prev, $image)
  Invoke-Native docker @('compose', 'up', '-d', '--no-deps', '--no-build', 'umbrella')
  if (-not (Wait-Healthy 'umbrella')) { Stop-Lab 'Umbrella is not healthy after rollback: docker compose logs umbrella' }
  Write-Step "Umbrella $before -> $(Get-UmbrellaVersion $port)"
  exit 0
}

if (-not $NoPull -and (Get-Command git -ErrorAction SilentlyContinue)) {
  $old = Get-NativeOutput git @('-C', $lab, 'rev-parse', '--short', 'HEAD')
  Write-Step 'Fetching code'
  Invoke-Native git @('-C', $lab, 'pull', '--ff-only', '-q')
  $new = Get-NativeOutput git @('-C', $lab, 'rev-parse', '--short', 'HEAD')
  Write-Step "  $old -> $new"
}

Update-DotEnv (Join-Path $lab '.env')
Write-Step 'OpenBao'
Start-OpenBao $lab

$env:UMBRELLA_VERSION = Get-NativeOutput git @('-C', $lab, 'describe', '--tags', '--always', '--dirty')
if (-not $env:UMBRELLA_VERSION) { $env:UMBRELLA_VERSION = Get-Date -Format 'yyyyMMddHHmm' }
if (Test-Native docker @('image', 'inspect', $image)) { Invoke-Native docker @('image', 'tag', $image, $prev) }
Write-Step "Building Umbrella $($env:UMBRELLA_VERSION) (the running container keeps serving)"
Invoke-Native docker @('compose', 'build', '--pull', 'umbrella')

if ($All) {
  Write-Step 'Pulling images and applying configs of every service'
  Invoke-Native docker @('compose', 'pull', '--ignore-buildable', '--quiet')
  Invoke-Native docker @('compose', 'build', 'fluentd')
  Invoke-Native docker @('compose', 'up', '-d', '--remove-orphans')
  if ($old -and $new -and $old -ne $new) {
    $changed = @(Get-NativeOutput git @('-C', $lab, 'diff', '--name-only', $old, $new)) -split "`n"
    $map = [ordered]@{ 'deploy/lab/telegraf/' = 'telegraf'; 'deploy/lab/prometheus/' = 'prometheus';
      'deploy/lab/alertmanager/' = 'alertmanager'; 'deploy/lab/node-textfile/' = 'node-exporter'; 'deploy/grafana/' = 'grafana' }
    $restart = @($map.Keys | Where-Object { $p = $_; $changed | Where-Object { $_.StartsWith($p) } } | ForEach-Object { $map[$_] })
    if ($restart.Count -gt 0) {
      Write-Step "Restarting services with changed mounted configs: $($restart -join ' ')"
      Invoke-Native docker (@('compose', 'restart') + $restart)
    }
  }
} else {
  Write-Step 'Replacing the umbrella container'
  Invoke-Native docker @('compose', 'up', '-d', '--no-deps', '--no-build', 'umbrella')
}

if (-not (Wait-Healthy 'umbrella')) {
  Invoke-Native docker @('compose', 'logs', '--tail', '30', 'umbrella')
  if (Test-Native docker @('image', 'inspect', $prev)) {
    Write-Step 'Umbrella is not healthy: rolling back'
    Invoke-Native docker @('image', 'tag', $prev, $image)
    Invoke-Native docker @('compose', 'up', '-d', '--no-deps', '--no-build', 'umbrella')
    Wait-Healthy 'umbrella' | Out-Null
  }
  Stop-Lab 'update failed; the previous version is running again'
}
Write-Step "Umbrella $before -> $(Get-UmbrellaVersion $port)"

if ($Configure -and (Invoke-Toolbox 'configure.sh') -ne 0) { Stop-Lab 'configure.sh failed' }
if ($Test) { exit (Invoke-Toolbox 'smoke-test.sh' @('--all')) }
