<#
.SYNOPSIS
  Installs or verifies the tools Podcast AI Studio needs: FFmpeg, Node.js and
  the Remotion dependencies. Safe to run repeatedly; it skips work already done.

.NOTES
  No local AI models are downloaded. Transcription and editorial AI run through
  the hosted services you connect inside the app.

  A log is written to %LOCALAPPDATA%\PodcastAIStudio\setup.log.
#>
[CmdletBinding()]
param(
  # Re-run every step even when the stamp file says the install is current.
  [switch]$Force,
  # Only verify; never install anything. Exit code 2 means something is missing.
  [switch]$CheckOnly
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$remotion = Join-Path $root 'remotion'
$logDir = Join-Path $env:LOCALAPPDATA 'PodcastAIStudio'
New-Item -ItemType Directory -Force -Path $logDir | Out-Null
$log = Join-Path $logDir 'setup.log'
$minNode = [version]'18.0.0'

function Log([string]$Message, [string]$Color = 'Gray') {
  $line = "[{0}] {1}" -f (Get-Date -Format 'yyyy-MM-dd HH:mm:ss'), $Message
  Add-Content -Path $log -Value $line
  Write-Host $Message -ForegroundColor $Color
}

function Update-PathEnvironment {
  $machine = [Environment]::GetEnvironmentVariable('Path', 'Machine')
  $user = [Environment]::GetEnvironmentVariable('Path', 'User')
  $extra = @(
    (Join-Path $env:LOCALAPPDATA 'Microsoft\WinGet\Links'),
    (Join-Path $env:ProgramFiles 'nodejs'),
    (Join-Path ${env:ProgramFiles(x86)} 'nodejs')
  ) | Where-Object { $_ -and (Test-Path $_) }
  $env:Path = ($extra + @($machine, $user, $env:Path) | Where-Object { $_ }) -join ';'
}

function Has([string]$Command) {
  return [bool](Get-Command $Command -ErrorAction SilentlyContinue)
}

function Install-Tool([string]$Command, [string]$WingetId, [string]$DownloadUrl) {
  if (Has $Command) { return }
  if ($CheckOnly) { throw "$Command is not installed." }
  if (-not (Has 'winget')) {
    throw "$Command is missing and Windows Package Manager (winget) is not available. Install it manually from $DownloadUrl, then run this setup again."
  }
  Log "Installing $WingetId (you may see a Windows permission prompt)..." 'Yellow'
  & winget install --id $WingetId --exact --accept-package-agreements --accept-source-agreements --disable-interactivity
  if ($LASTEXITCODE -ne 0) { throw "winget could not install $WingetId (exit $LASTEXITCODE). Install it manually from $DownloadUrl." }
  Update-PathEnvironment
  if (-not (Has $Command)) {
    throw "$WingetId installed but '$Command' is still not on PATH. Close this window, open a new one and run setup again."
  }
}

try {
  Log "Podcast AI Studio setup starting in $root"
  Update-PathEnvironment

  Install-Tool 'ffmpeg' 'Gyan.FFmpeg' 'https://ffmpeg.org/download.html'
  Install-Tool 'ffprobe' 'Gyan.FFmpeg' 'https://ffmpeg.org/download.html'
  Install-Tool 'node' 'OpenJS.NodeJS.LTS' 'https://nodejs.org/en/download'
  Install-Tool 'npm.cmd' 'OpenJS.NodeJS.LTS' 'https://nodejs.org/en/download'

  $nodeVersion = [version]((& node -v).TrimStart('v'))
  if ($nodeVersion -lt $minNode) {
    throw "Node.js $nodeVersion is too old; Remotion needs $minNode or newer. Update Node.js from https://nodejs.org/en/download."
  }
  Log "FFmpeg: $((& ffmpeg -version | Select-Object -First 1))"
  Log "Node.js: v$nodeVersion"

  if (-not (Test-Path (Join-Path $remotion 'package-lock.json'))) {
    throw "The remotion folder is incomplete. Extract the entire ZIP, including remotion\ and scripts\."
  }

  # Stamp the installed dependency set so launches stay fast and work offline.
  $lockHash = (Get-FileHash (Join-Path $remotion 'package-lock.json') -Algorithm SHA256).Hash
  $stampValue = "$lockHash|node$nodeVersion"
  $stamp = Join-Path $remotion 'node_modules\.podcast-ai-studio-installed'
  $installed = (Test-Path $stamp) -and ((Get-Content $stamp -Raw).Trim() -eq $stampValue)

  if ($installed -and -not $Force) {
    Log 'Remotion dependencies are up to date.' 'Green'
  } elseif ($CheckOnly) {
    throw 'Remotion dependencies are not installed.'
  } else {
    Log 'Installing Remotion dependencies (first run can take a few minutes)...' 'Yellow'
    Push-Location $remotion
    try {
      & npm.cmd ci --no-audit --no-fund --loglevel=error
      if ($LASTEXITCODE -ne 0) { throw "npm ci failed (exit $LASTEXITCODE). Check your internet connection or proxy and try again." }
      Log 'Downloading the headless browser used for rendering...' 'Yellow'
      & npx.cmd --no-install remotion browser ensure
      if ($LASTEXITCODE -ne 0) { Log 'Could not pre-download the render browser; Remotion will try again on the first render.' 'Yellow' }
    } finally {
      Pop-Location
    }
    Set-Content -Path $stamp -Value $stampValue
    Log 'Remotion dependencies installed.' 'Green'
  }

  Log 'Setup complete.' 'Green'
  exit 0
} catch {
  Log "Setup failed: $($_.Exception.Message)" 'Red'
  Log "Details are in $log"
  if ($CheckOnly) { exit 2 }
  exit 1
}
