# Explicit, user-initiated publishing helper; never runs from Podcast AI Studio.
param([string]$RepositoryName = 'podcast-ai-studio')
$ErrorActionPreference = 'Stop'
Set-Location (Resolve-Path (Join-Path $PSScriptRoot '..'))
if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
  throw 'Git is required. Install https://git-scm.com/download/win first.'
}
if (-not (Get-Command gh -ErrorAction SilentlyContinue)) {
  throw 'GitHub CLI is required. Install https://cli.github.com/ or run: winget install GitHub.cli'
}
gh auth status *> $null
if ($LASTEXITCODE -ne 0) { gh auth login }
Write-Host "This script will create a NEW PRIVATE GitHub repository '$RepositoryName' and push this folder."
$answer = Read-Host 'Type CREATE to continue'
if ($answer -ne 'CREATE') { Write-Host 'Canceled'; exit 0 }
if (-not (Test-Path .git)) { git init -b main }
git add .
git commit -m 'Initial Podcast AI Studio alpha' 2>$null
if ($LASTEXITCODE -ne 0) { Write-Host 'No new changes to commit; continuing.' }
gh repo create $RepositoryName --private --source . --remote origin --push
if ($LASTEXITCODE -ne 0) { throw 'Repository creation or push failed. Check gh auth and existing repository name.' }
Write-Host 'Repository created and source pushed.'
