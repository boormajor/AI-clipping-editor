<#
.SYNOPSIS
  Builds the Windows executable and stages the distributable ZIP in dist/.

.PARAMETER Version
  Version string stamped into the binary (shown in the app and /api/status).

.EXAMPLE
  ./scripts/Build-Bundle.ps1 -Version v1.0.0
#>
[CmdletBinding()]
param(
  [string]$Version = 'dev',
  [string]$OutDir = 'dist'
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$commit = (& git rev-parse --short HEAD 2>$null)
if (-not $commit) { $commit = 'unknown' }
$date = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
$clean = $Version.TrimStart('v')

Write-Host "Building Podcast AI Studio $clean ($commit)" -ForegroundColor Cyan
$env:CGO_ENABLED = '0'
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
& go build -trimpath -ldflags "-s -w -X main.version=$clean -X main.commit=$commit -X main.date=$date" -o PodcastAIStudio.exe ./cmd/studio
if ($LASTEXITCODE -ne 0) { throw 'go build failed' }

$stage = Join-Path $OutDir 'PodcastAIStudio'
if (Test-Path $stage) { Remove-Item -Recurse -Force $stage }
New-Item -ItemType Directory -Force -Path $stage | Out-Null

Copy-Item PodcastAIStudio.exe, Install-and-Run.bat, README.md, LICENSE, NOTICE.md, CHANGELOG.md -Destination $stage
New-Item -ItemType Directory -Force -Path (Join-Path $stage 'scripts') | Out-Null
Copy-Item scripts/Install-All-Tools.ps1, scripts/face_audit.py, scripts/scene_audit.py, scripts/requirements.txt -Destination (Join-Path $stage 'scripts')
Copy-Item -Recurse examples -Destination (Join-Path $stage 'examples')
Copy-Item -Recurse docs -Destination (Join-Path $stage 'docs')

# Remotion project without node_modules or staged jobs; the installer runs npm ci.
$rem = Join-Path $stage 'remotion'
New-Item -ItemType Directory -Force -Path $rem | Out-Null
foreach ($item in @('package.json', 'package-lock.json', 'tsconfig.json', 'remotion.config.ts', 'src', 'scripts')) {
  Copy-Item -Recurse (Join-Path 'remotion' $item) -Destination (Join-Path $rem $item)
}
New-Item -ItemType Directory -Force -Path (Join-Path $rem 'public/jobs') | Out-Null
New-Item -ItemType File -Force -Path (Join-Path $rem 'public/jobs/.gitkeep') | Out-Null

$zip = Join-Path $OutDir "PodcastAIStudio-$clean-windows-x64.zip"
if (Test-Path $zip) { Remove-Item -Force $zip }
Compress-Archive -Path (Join-Path $stage '*') -DestinationPath $zip -CompressionLevel Optimal

$hash = (Get-FileHash $zip -Algorithm SHA256).Hash.ToLower()
"$hash  $(Split-Path -Leaf $zip)" | Set-Content -Path (Join-Path $OutDir 'SHA256SUMS.txt') -Encoding ascii
Write-Host "Bundle: $zip" -ForegroundColor Green
Write-Host "SHA256: $hash"
