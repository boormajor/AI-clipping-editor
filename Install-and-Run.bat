@echo off
setlocal
cd /d "%~dp0"
title Podcast AI Studio

if not exist "PodcastAIStudio.exe" (
  echo Missing PodcastAIStudio.exe. Extract the whole ZIP before running this file.
  pause
  exit /b 1
)
if not exist "remotion\package.json" (
  echo Missing the remotion folder. Extract the whole ZIP before running this file.
  pause
  exit /b 1
)

echo Checking FFmpeg, Node.js and Remotion (fast when already installed)...
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\Install-All-Tools.ps1"
if errorlevel 1 (
  echo.
  echo Setup did not finish. Read the message above, fix it, and run this file again.
  pause
  exit /b 1
)

echo.
echo Starting Podcast AI Studio. Keep this window open while you edit.
"%~dp0PodcastAIStudio.exe"
if errorlevel 1 (
  echo.
  echo Podcast AI Studio exited with an error. See the log in %LOCALAPPDATA%\PodcastAIStudio\studio.log
  pause
)
