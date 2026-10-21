# Podcast AI Studio — v0.4 Quality Pipeline (Alpha)

Windows-first, open-source podcast clipping studio. Import a finished single-camera/split-screen recording and timestamped SRT/VTT, connect ChatGPT Plus or Claude Pro through the existing supported sign-in integration, pick short highlights, improve context, align speech, edit shots, and export **through Remotion**. Nothing publishes automatically.

> **Alpha, not a complete commercial OpusClip replacement.** The Windows workflow and real subscription access require on-device verification. Automated quality checks make suggestions and can still be wrong; review the final render. The GUI's video preview is a rough source preview, not a live Remotion composition.

## Installation (Windows)

1. In [GitHub Actions](https://github.com/boormajor/AI-clipping-editor/actions), download the most recent **successful** `PodcastAIStudio-Windows-x64-alpha` build artifact.
2. Extract the ENTIRE ZIP, including `remotion/`, `scripts/`, the EXE and `Install-and-Run.bat`.
3. Double-click `Install-and-Run.bat`. It checks **FFmpeg** and requires **Node.js 20+**. The launcher runs `npm install` inside `remotion/` on first launch (internet needed). Node/Remotion installation can take several minutes. Keep the console window open.
4. In **AI connections**, use either **Continue with ChatGPT** or **Claude Code login**, subject to plan eligibility and provider limits. API-key modes are available for separately billed API usage. Do not paste account cookies, passwords or subscription tokens into API key fields.
5. Import the finished video and a timestamped SRT or VTT exported from Descript. Choose a folder for exports, and click **Import episode**.
6. Choose **Find best moments** with an online model. Select a clip, and use **AI improve clip** for better standalone context, title and a proposed shot plan. Review what it changed.
7. For best captions, run **Install-Quality-Tools.bat** once, then click **Sync captions to audio** in the clip editor. This runs WhisperX on the *actual audio*, not guessed word timing. Review the transcript agreement and any technical terms.
8. Choose caption template, font, colors, shot layout, local/Pexels B-roll and effects. Press **Apply clip changes**, then **Render with Remotion**. Open **Render queue** to find the MP4. Save your project for another editing session.

### Included and optional tools

| Tool | Purpose | Included / Cost |
|---|---|---|
| Remotion | **Primary** final video renderer, motion typography, stacked/two-person shots, B-roll compositions (an FFmpeg fallback renders burned-caption clips if Remotion is not installed) | npm install required for motion captions; separate license conditions apply |
| FFmpeg | Source trimming, soundtrack preparation, codec conversion | Free and open source; installed separately |
| ChatGPT Plus sign-in / Claude Code | Online LLM for editorial clip selection, review, titles and shot proposals | Subject to plan eligibility and usage limits; API keys optional |
| WhisperX | Optional speech recognition **and** word-level forced alignment; transcript agreement check | Free code/models; large model downloads and GPU/RAM usage |
| MediaPipe + OpenCV | Optional sample-frame face occupancy and edge-risk measurements | Free code; installed separately |
| PySceneDetect | Optional scene-cut metadata to inform online AI review | Free code; installed separately |
| Pexels | Optional manual free-stock search and downloading | Free API key; usage terms and licensing apply |

**Free tools do not guarantee free cloud inference, content licenses, or Remotion commercial rights.** Remotion's free license covers individuals and qualifying small teams; larger teams or commercial automation products may need a paid license. Read [Remotion licensing](https://www.remotion.dev/docs/license/pricing) before commercial use.

### AI quality boundaries

- **Online-first editorial review:** An authorized ChatGPT/Claude model gets the current clip transcript, about 18 seconds before and 14 after, observed face bounding-box metadata (if MediaPipe is installed), real alignment coverage (if WhisperX ran), and scene-cut metadata (if PySceneDetect is installed). It writes a title, flags context problems and may create an editable shot schedule. No unsupported claims that the cloud model watched the recording; the current subscription adapter does **not** send video frames or audio to the online model.
- **Caption honesty:** With WhisperX, the Remotion animated captions follow actual recognized **word times** and use the recognizer's words. Without WhisperX, subtitles follow the original SRT cue boundaries; words are NOT falsely highlighted as if individually aligned. `Sync captions` may require correcting proper nouns and marketing jargon.
- **Safer two-speaker framing:** `Both speakers` stacks left/right halves for an existing side-by-side source. `Dynamic` uses a draft shot schedule with both-speaker safety bias; the AI reviewer can propose edits. There is **no reliable automatic speaker-to-camera attribution** yet from a mixed video.
- **Professional polish:** Remotion adds a range of animated caption styles — kinetic pop, word-by-word reveal, typewriter, word-box highlight, karaoke wipe and fill, and slide/bounce entrances — alongside boxed/editorial/minimal templates, configurable color/font/size, title graphics, fixed or AI-proposed shots, editable layer decisions, and user-supplied B-roll. Per-word animations (reveal, pop, wipe, word-box, karaoke) only engage when real WhisperX word timing is present; without it the whole SRT cue is shown with the chosen entrance, never fabricated per-word timing. Seamless word-perfect alignment, universal face tracking and automatic creative stock selection cannot be promised without correct tools and human review.
- **Quality vs speed:** Transcription and rendering may take much longer than a clip's runtime. No original video/transcript files are overwritten; temporary staged media is deleted after rendering.

## Optional quality tools (Windows)

Run `Install-Quality-Tools.bat` after installing **Python 3.11** and a suitable PyTorch CUDA build for your GPU. It installs WhisperX, MediaPipe and PySceneDetect into a virtual environment in `.quality-venv`. The runtime tries the quality environment automatically when present. Large model weights download on first alignment. WhisperX CPU fallback is slower. Check tool licenses and Python/CUDA compatibility.

To install manually from project root:

```powershell
py -3.11 -m venv .quality-venv
.quality-venv\Scripts\python.exe -m pip install whisperx mediapipe==0.10.21 opencv-python-headless 'scenedetect[opencv]'
```

Do not commit `.quality-venv/`, credentials, real recordings or generated clips to GitHub.

## Current implementation and roadmap

**Implemented in source:** Remotion rendering backend and motion-caption composition with an automatic FFmpeg fallback (burns the original SRT cue boundaries when Node/Remotion is unavailable, never fabricated word timings), caption font/color settings, optional real audio alignment, optional frame/scene observations, editorial review endpoint, two-person and shot-plan layouts, project persistence, safe source processing, local preview, API/plan-based LLM selection.

**Requires real Windows testing:** npm/Chrome/codec initialization, WhisperX CUDA installation, account sign-in sessions, large videos, rendering throughput. GitHub CI verifies Go tests and Remotion bundling; this is not the same as real editing quality validation.

**Not implemented yet:** true online multimodal video+audio understanding through subscription plans; automatic cloud video upload workflow; built-in visual keyframe editor; continuous face tracking; perfect audio-to-face speaker detection; fully automatic licensed B-roll selection and placement; multi-pass render critique and repair; complete sound design; drag/drop multi-track editor. Further work is required for commercial tool parity.

## Developers

```bash
go test ./...
cd remotion
npm install
npx remotion compositions src/index.ts
# Build native launcher from repository root:
go build -o PodcastAIStudio.exe ./cmd/studio
```

The Go backend starts on 127.0.0.1; React/Remotion code is in `remotion/src`. Imported videos and cloud auth credentials are never committed. Remotion is a separate runtime project: **the Windows EXE alone is insufficient**.

License: MIT for our application source only. Third-party packages, model weights, Pexels assets and Remotion carry their own licenses.
