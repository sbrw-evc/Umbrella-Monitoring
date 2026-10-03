<#
.SYNOPSIS
  Tests the lab: runs smoke-test.sh in the toolbox container.
.EXAMPLE
  .\smoke-test.ps1 -All
#>
param([switch]$Zabbix, [switch]$Stack, [switch]$All)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'lib.ps1')
Set-Location $PSScriptRoot
$a = @()
if ($Zabbix) { $a += '--zabbix' }
if ($Stack) { $a += '--stack' }
if ($All) { $a += '--all' }
exit (Invoke-Toolbox 'smoke-test.sh' $a)
