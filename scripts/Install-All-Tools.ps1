# Online-only setup: FFmpeg, Node.js, Remotion. No Ollama, WhisperX or model downloads.
$ErrorActionPreference='Stop'
Set-Location (Split-Path -Parent $PSScriptRoot)
function Ensure($cmd,$wingetId) {
 if(Get-Command $cmd -ErrorAction SilentlyContinue){return}
 if(-not (Get-Command winget -ErrorAction SilentlyContinue)){throw "Install $wingetId or Windows Package Manager."}
 & winget install --id $wingetId --exact --accept-package-agreements --accept-source-agreements
 if($LASTEXITCODE -ne 0){throw "$wingetId failed to install"}
 $env:Path=[Environment]::GetEnvironmentVariable('Path','Machine')+';'+[Environment]::GetEnvironmentVariable('Path','User')+';'+$env:Path
 if(-not (Get-Command $cmd -ErrorAction SilentlyContinue)){throw "Restart the terminal after installing $wingetId"}
}
Ensure 'ffmpeg' 'Gyan.FFmpeg'
Ensure 'ffprobe' 'Gyan.FFmpeg'
Ensure 'node' 'OpenJS.NodeJS.LTS'
Ensure 'npm.cmd' 'OpenJS.NodeJS.LTS'
Set-Location 'remotion'
& npm.cmd ci --no-audit --no-fund
if($LASTEXITCODE -ne 0){throw "Remotion dependencies failed"}
Write-Host 'Online-only core setup complete.' -ForegroundColor Green
