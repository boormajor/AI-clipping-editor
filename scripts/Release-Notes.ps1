<#
.SYNOPSIS
  Extracts the CHANGELOG section for a version into a release-notes file.
#>
[CmdletBinding()]
param(
  [Parameter(Mandatory)][string]$Version,
  [string]$Out = 'dist/RELEASE_NOTES.md'
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$clean = $Version.TrimStart('v')
$lines = Get-Content (Join-Path $root 'CHANGELOG.md')
$section = New-Object System.Collections.Generic.List[string]
$inside = $false
foreach ($line in $lines) {
  if ($line -match '^## \[?([^\]\s]+)') {
    if ($inside) { break }
    if ($Matches[1] -eq $clean) { $inside = $true; continue }
  }
  if ($inside) { $section.Add($line) }
}
if ($section.Count -eq 0) {
  $section.Add("Release $Version. See CHANGELOG.md for details.")
}
$section.Add('')
$section.Add('Verify the download with the SHA256SUMS.txt file attached to this release.')
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $Out) | Out-Null
$section -join "`n" | Set-Content -Path $Out -Encoding utf8
Write-Host "Release notes written to $Out"
