# Podcast AI Studio first-run setup. ALL core tools are mandatory.
# Run only on a machine you own. Downloads several GB of free packages/models.
param([switch]$SkipModelsForCI)
$ErrorActionPreference='Stop'
$ProgressPreference='SilentlyContinue'
Set-Location (Split-Path -Parent $PSScriptRoot)
$root=(Get-Location).Path
function Say([string]$message) { Write-Host "[Podcast AI] $message" -ForegroundColor Cyan }
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
try {
  Say 'Checking required programs: FFmpeg, Node.js, Python 3.11, Ollama.'
  Check-Tool 'ffmpeg' 'Gyan.FFmpeg'
  Check-Tool 'node' 'OpenJS.NodeJS.LTS'
  Check-Tool 'npm.cmd' 'OpenJS.NodeJS.LTS'
  Check-Tool 'py' 'Python.Python.3.11'
  Check-Tool 'ollama' 'Ollama.Ollama'
  if(-not (Get-Command ffprobe -ErrorAction SilentlyContinue)){ throw 'FFprobe is missing from the installed FFmpeg package.' }

  Say 'Installing Remotion (required final renderer)...'
  Push-Location (Join-Path $root 'remotion')
  if (-not (Test-Path 'node_modules/@remotion/cli')) {
    & npm.cmd install --no-audit --no-fund
    if($LASTEXITCODE -ne 0){throw 'Remotion npm installation failed.'}
  }
  Pop-Location

  $py = Join-Path $root '.quality-venv/Scripts/python.exe'
  if(-not (Test-Path $py)){
    Say 'Creating Python 3.11 quality environment...'
    & py -3.11 -m venv (Join-Path $root '.quality-venv')
    if($LASTEXITCODE -ne 0){throw 'Python 3.11 virtual environment failed. Verify Python 3.11 is installed.'}
  }
  Say 'Installing WhisperX, MediaPipe, OpenCV, PySceneDetect and PyTorch dependencies...'
  & $py -m pip install --disable-pip-version-check --upgrade pip
  if($LASTEXITCODE -ne 0){throw 'Pip update failed.'}
  & $py -m pip install --disable-pip-version-check --prefer-binary whisperx mediapipe==0.10.21 opencv-python-headless 'scenedetect[opencv]'
  if($LASTEXITCODE -ne 0){throw 'Media AI dependency installation failed. See the error output.'}
  & $py -m pip check
  if($LASTEXITCODE -ne 0){throw 'Python dependency conflicts detected. Installer did not complete.'}
  & $py -c 'import whisperx, mediapipe, cv2, scenedetect; print("Media QA imports verified")'
  if($LASTEXITCODE -ne 0){throw 'Python media QA check failed.'}
  if (-not $SkipModelsForCI){
    Say 'Pre-caching WhisperX speech and alignment models (may take many minutes)...'
    & $py (Join-Path $root 'scripts/precache_models.py')
    if($LASTEXITCODE -ne 0){throw 'Audio models failed to download. Retry with internet access.'}
    Say 'Downloading free local model fallback and vision model...'
    & ollama pull qwen2.5:7b
    if($LASTEXITCODE -ne 0){throw 'Ollama language model download failed.'}
    & ollama pull gemma3:4b
    if($LASTEXITCODE -ne 0){throw 'Ollama vision model download failed.'}
  }
  Say 'All required editing tools are installed and validated.'
  exit 0
} catch {
  Write-Host "[SETUP FAILED] $_" -ForegroundColor Red
  exit 1
}
