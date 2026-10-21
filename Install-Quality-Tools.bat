@echo off
setlocal
cd /d "%~dp0"
where py >nul 2>nul
if errorlevel 1 (
 echo Python launcher not found. Install Python 3.11 from https://www.python.org/downloads/
 pause
 exit /b 1
)
if not exist .quality-venv\Scripts\python.exe (
 py -3.11 -m venv .quality-venv
 if errorlevel 1 (
   echo Please install Python 3.11, not an incompatible preview.
   pause
   exit /b 1
 )
)
echo Installing optional media AI dependencies, including WhisperX.
echo This may download gigabytes of model/runtime libraries and take some time.
.quality-venv\Scripts\python.exe -m pip install --upgrade pip
.quality-venv\Scripts\python.exe -m pip install whisperx mediapipe==0.10.21 opencv-python-headless "scenedetect[opencv]"
if errorlevel 1 (
 echo Installation failed. Read README.md and check your Python CUDA/PyTorch setup.
 pause
 exit /b 1
)
echo Quality tool packages installed. Return to Podcast AI Studio and try Sync captions.
pause
