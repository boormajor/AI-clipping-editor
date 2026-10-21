@echo off
setlocal
cd /d "%~dp0"
title Podcast AI Studio - Online AI Edition
if not exist "PodcastAIStudio.exe" (echo Missing executable. Extract the whole ZIP.&pause&exit /b 1)
if not exist "remotion\package.json" (echo Missing Remotion folder.&pause&exit /b 1)
echo Checking FFmpeg, Node.js and Remotion. Local AI models are not installed.
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\Install-All-Tools.ps1"
if errorlevel 1 (echo Setup failed. See message above.&pause&exit /b 1)
"%~dp0PodcastAIStudio.exe"
if errorlevel 1 pause
