@echo off
setlocal
cd /d "%~dp0"
title Podcast AI Studio - Setup
where ffmpeg >nul 2>nul
if errorlevel 1 (
 echo [SETUP] FFmpeg not found on PATH.
 echo FFmpeg is free and required for video rendering.
 where winget >nul 2>nul
 if errorlevel 1 (
    echo Please install FFmpeg manually from https://ffmpeg.org/download.html
    pause
    exit /b 1
 )
 choice /c YN /m "Install free FFmpeg via Windows Package Manager now"
 if errorlevel 2 exit /b 1
 winget install --id Gyan.FFmpeg --exact --accept-package-agreements --accept-source-agreements
 echo.
 echo If the installation succeeded, restart this batch file so PATH refreshes.
 pause
 exit /b 0
)
where ffprobe >nul 2>nul
if errorlevel 1 (
 echo [ERROR] ffprobe is missing. Please install the full FFmpeg package.
 pause
 exit /b 1
)
if not exist "PodcastAIStudio.exe" (
 echo [ERROR] PodcastAIStudio.exe is missing from this folder.
 pause
 exit /b 1
)
echo Launching Podcast AI Studio ...
echo Keep this window open to run the local app. Close it to quit.
"%~dp0PodcastAIStudio.exe"
if errorlevel 1 pause
