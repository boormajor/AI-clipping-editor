@echo off
setlocal
cd /d "%~dp0"
title Podcast AI Studio - Mandatory Setup
if not exist "PodcastAIStudio.exe" (echo ERROR: Extract the complete bundle first.&pause&exit /b 1)
if not exist "remotion\package.json" (echo ERROR: Remotion files are missing.&pause&exit /b 1)
echo All editing dependencies are required: Remotion, FFmpeg, WhisperX, MediaPipe, PySceneDetect, Python, Node.js, Ollama.
echo Installation can download several gigabytes and may take a long time.
choice /C YN /M "Install or check all dependencies now"
if errorlevel 2 exit /b 1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\Install-All-Tools.ps1"
if errorlevel 1 (echo Dependency setup failed. Correct the errors and rerun this file.&pause&exit /b 1)
"%~dp0PodcastAIStudio.exe"
if errorlevel 1 pause
