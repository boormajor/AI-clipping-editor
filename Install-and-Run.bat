@echo off
setlocal
cd /d "%~dp0"
title Podcast AI Studio - Setup
if not exist "PodcastAIStudio.exe" (echo ERROR: Extract the complete bundle first.&pause&exit /b 1)
if not exist "remotion\package.json" (echo ERROR: Remotion files are missing.&pause&exit /b 1)
echo Required to render: FFmpeg, Node.js and Remotion (installed automatically).
echo Optional quality tools (WhisperX, MediaPipe, PySceneDetect, Ollama) add word-accurate
echo captions and local AI. They are large downloads and can be skipped now and added later
echo with Install-Quality-Tools.bat. The studio still opens if the optional tools are skipped.
echo.
choice /C YN /M "Install core tools now (and attempt the optional ones)"
if errorlevel 2 exit /b 1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\Install-All-Tools.ps1"
if errorlevel 1 (echo Core dependency setup failed. Correct the errors above and rerun this file.&pause&exit /b 1)
"%~dp0PodcastAIStudio.exe"
if errorlevel 1 pause
