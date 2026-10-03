<#
.SYNOPSIS
  Connects the lab to Umbrella: runs configure.sh in the toolbox container.
#>
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'lib.ps1')
Set-Location $PSScriptRoot
exit (Invoke-Toolbox 'configure.sh')
