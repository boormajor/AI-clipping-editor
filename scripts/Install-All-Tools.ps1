# Podcast AI Studio first-run setup.
#   REQUIRED core tools: FFmpeg, Node.js, Remotion (npm install).
#   OPTIONAL quality tools: Python 3.11 + WhisperX / MediaPipe / PySceneDetect, Ollama models.
# The optional section never blocks the app from launching: failures are reported
# as warnings and setup still exits 0 so Install-and-Run.bat opens the studio.
# Run only on a machine you own. Downloads can be several GB.
param([switch]$SkipModelsForCI,[switch]$SkipOptional)
$ErrorActionPreference='Stop'
$ProgressPreference='SilentlyContinue'
Set-Location (Split-Path -Parent $PSScriptRoot)
$root=(Get-Location).Path
function Say([string]$message) { Write-Host "[Podcast AI] $message" -ForegroundColor Cyan }
function Warn([string]$message) { Write-Host "[Podcast AI] $message" -ForegroundColor Yellow }
function Refresh-Path {
  $env:Path=[Environment]::GetEnvironmentVariable('Path','Machine')+';'+[Environment]::GetEnvironmentVariable('Path','User')+';'+$env:Path
}
function Check-Tool([string]$cmd,[string]$wingetId){
  if (Get-Command $cmd -ErrorAction SilentlyContinue) { return }
  if (-not (Get-Command winget -ErrorAction SilentlyContinue)) { throw "Windows Package Manager is missing. Install Microsoft App Installer before retrying ($wingetId)." }
  Say "Installing $wingetId..."
  & winget install --id $wingetId --exact --accept-package-agreements --accept-source-agreements --silent
  if ($LASTEXITCODE -ne 0) { throw "Failed to install $wingetId (exit $LASTEXITCODE)." }
  Refresh-Path
  if (-not (Get-Command $cmd -ErrorAction SilentlyContinue)) {
    throw "$wingetId was installed but is not on PATH yet. Close this window and rerun Install-and-Run.bat."
  }
}

# ---------- REQUIRED CORE (failure aborts setup) ----------
try {
  Say 'Checking REQUIRED tools: FFmpeg and Node.js.'
  Check-Tool 'ffmpeg' 'Gyan.FFmpeg'
  Check-Tool 'node' 'OpenJS.NodeJS.LTS'
  Check-Tool 'npm.cmd' 'OpenJS.NodeJS.LTS'
  if(-not (Get-Command ffprobe -ErrorAction SilentlyContinue)){ throw 'FFprobe is missing from the installed FFmpeg package.' }

  Say 'Installing Remotion (required final renderer)...'
  Push-Location (Join-Path $root 'remotion')
  if (-not (Test-Path 'node_modules/@remotion/cli')) {
    & npm.cmd install --no-audit --no-fund
    if($LASTEXITCODE -ne 0){ Pop-Location; throw 'Remotion npm installation failed.' }
  }
  Pop-Location
  Say 'Required core tools are installed. The studio can run now.'
} catch {
  Write-Host "[SETUP FAILED] $_" -ForegroundColor Red
  exit 1
}

# ---------- OPTIONAL QUALITY TOOLS (failure only warns) ----------
if ($SkipOptional) {
  Say 'Skipping optional quality tools (--SkipOptional). Install them later with Install-Quality-Tools.bat.'
  exit 0
}
$optionalFailed=$false
try {
  Say 'Installing OPTIONAL quality tools: Python 3.11, WhisperX, MediaPipe, PySceneDetect, Ollama.'
  Say 'These enable word-accurate captions, face/scene checks and local AI. They are not required to render clips.'
  Check-Tool 'py' 'Python.Python.3.11'

  $py = Join-Path $root '.quality-venv/Scripts/python.exe'
  if(-not (Test-Path $py)){
    Say 'Creating Python 3.11 quality environment...'
    & py -3.11 -m venv (Join-Path $root '.quality-venv')
    if($LASTEXITCODE -ne 0){ throw 'Python 3.11 virtual environment failed. Verify Python 3.11 is installed.' }
  }
  Say 'Installing WhisperX, MediaPipe, OpenCV and PySceneDetect...'
  & $py -m pip install --disable-pip-version-check --upgrade pip
  if($LASTEXITCODE -ne 0){ throw 'Pip update failed.' }
  & $py -m pip install --disable-pip-version-check --prefer-binary whisperx mediapipe==0.10.21 opencv-python-headless 'scenedetect[opencv]'
  if($LASTEXITCODE -ne 0){ throw 'Media AI dependency installation failed. See the error output.' }
  & $py -m pip check
  if($LASTEXITCODE -ne 0){ throw 'Python dependency conflicts detected.' }
  & $py -c 'import whisperx, mediapipe, cv2, scenedetect; print("Media QA imports verified")'
  if($LASTEXITCODE -ne 0){ throw 'Python media QA check failed.' }
  if (-not $SkipModelsForCI){
    Say 'Pre-caching WhisperX speech and alignment models (may take many minutes)...'
    & $py (Join-Path $root 'scripts/precache_models.py')
    if($LASTEXITCODE -ne 0){ throw 'Audio models failed to download. Retry with internet access.' }
  }
} catch {
  $optionalFailed=$true
  Warn "Optional media AI tools did not finish: $_"
  Warn 'You can still import, edit and render clips. Rerun Install-Quality-Tools.bat later to add them.'
}

# Ollama + local models are optional and installed best-effort.
if (-not $SkipModelsForCI) {
  try {
    Check-Tool 'ollama' 'Ollama.Ollama'
    Say 'Downloading free local language and vision models for offline AI...'
    & ollama pull qwen2.5:7b
    if($LASTEXITCODE -ne 0){ throw 'Ollama language model download failed.' }
    & ollama pull gemma3:4b
    if($LASTEXITCODE -ne 0){ throw 'Ollama vision model download failed.' }
  } catch {
    $optionalFailed=$true
    Warn "Optional Ollama local models were not installed: $_"
    Warn 'Local (offline) AI review is unavailable until Ollama and its models are installed; online providers and the free heuristic still work.'
  }
}

if ($optionalFailed) {
  Say 'Core setup complete. Some optional quality tools were skipped (see warnings above).'
} else {
  Say 'All required and optional tools are installed and validated.'
}
exit 0
