# Podcast AI Studio

**Local-first podcast clipping studio for Windows** — a free, source-available Go desktop launcher with a polished local GUI, transcript-based highlight discovery, optional Ollama/OpenAI/Claude analysis, manual timeline adjustments, and FFmpeg vertical exports.

> **Status: v0.1.0 alpha.** This is a genuinely executable first build, not a full replacement for professional editors or OpusClip. Features marked *planned* in the GUI are not implemented yet. The project is built around original-media safety: imported footage is never modified. All renders go to a separate output folder.

## Get started on Windows

1. Download the archive and extract it somewhere you can write files.
2. **Double-click `Install-and-Run.bat`**. It checks whether `ffmpeg`/`ffprobe` are installed and offers to install the free FFmpeg package with Windows Package Manager (`winget`). Restart the batch file or Windows terminal after installing if PATH has not refreshed.
3. The batch file starts `PodcastAIStudio.exe`. The app opens in an **Edge app window** (or your default browser if Edge isn't found). Keep the console window open while you work; closing it stops the local app.
4. In Studio, click **Browse** next to a finished podcast recording and a **timestamped SRT or VTT transcript** (recommended export from Descript). TXT, DOCX, PDF, Markdown, and JSON parsing is available, but the current version cannot safely invent timings for untimed text. See [Transcript formats](#transcript-formats).
5. Select **AI connections**: built-in heuristic (free and offline), Ollama (free local LLM; install Ollama yourself), OpenAI API, or Anthropic API. API keys are transmitted only for requests you explicitly start and **never saved to the project JSON**.
6. Pick clip count/duration, click **Find best moments**, adjust chosen clips, and click **Render clip**. MP4 files appear in your selected export folder.
7. Save the project to `podcast-ai-studio.project.json` in the export folder for later editing.

The Windows EXE is **only the application**; external FFmpeg remains a separate free dependency. The included installer helps set it up. No model downloads, stock footage, or paid accounts are included in the EXE.

## Editing controls

| Feature | Status in v0.1.0 | Notes |
| --- | --- | --- |
| Windows .exe, local GUI | **Working; compiled** | Small native launcher, local server; Edge in app mode; default browser fallback |
| SRT/VTT, timecoded text, Whisper-style JSON | **Working; tested** | Import parsed into timecoded cues |
| DOCX and PDF extraction | **Partial** | Reads content; timed transcript still required. PDF requires `pdftotext` from Poppler |
| Automatic plain-text alignment | **Planned** | Will need a locally installed Whisper-based aligner; untimed clips are deliberately blocked |
| AI highlight suggestions | **Working** | Free heuristic; optional Ollama, OpenAI, Anthropic; human should review selections |
| AI accounts | **API keys only** | ChatGPT Plus/Claude Pro subscriptions do not automatically provide API credits |
| Adjustable clip count and duration | **Working** | User specifies maximum count and length bounds |
| Editable clip trim points | **Working** | Numeric in/out and visual transcript timeline handles |
| Vertical video reframing | **Working, manual focus** | Adjustable horizontal focal point, center by default; does not track faces yet |
| Local live preview | **Working** | Embedded player, subtitle preview, cue timing |
| Subtitle style templates | **Working, basic styles** | Bold, minimal, yellow, highlight/accent, karaoke/accent, podcast. Word-by-word highlighting is planned |
| Subtitle burning | **Working; tested** | Styled ASS subtitles burned into video with FFmpeg |
| B-roll | **Working, local layer placement** | Optional local overlays at chosen offsets; original spoken audio remains |
| Stock footage | **Working, manually selected** | Optional free Pexels API key; searches/videos are never downloaded automatically |
| Callout graphics | **Working, basic** | Optional text callout overlays using FFmpeg's drawtext filter; limited templates |
| Automatic zoom | **Basic effect** | One fixed punch-in around seconds 1.5–3.5 if enabled; dynamic ML timing planned |
| Transitions and sound effects | **Planned** | Switches stored in project, do not create effects yet |
| Auto face/speaker tracking | **Planned** | Ideal for mixed-camera/split-screen podcast exports |
| Automatically chosen B-roll | **Planned** | Pexels stock is currently searched/added manually |
| Professional multi-track editing UI | **Early prototype** | Timed clip trim and B-roll layers; full drag/drop timeline is planned |
| Render queue | **Working** | Renders in background with success/failure shown; NVIDIA NVENC with software fallback |
| GPU acceleration | **Working when NVENC enabled** | Most processing still uses CPU FFmpeg filtering |
| Project save/reopen | **Working** | JSON stored separate from source media |
| 9:16 1080×1920 / 30 FPS | **Default** | Resolution and 24/25/30/60 FPS customizable |
| 60 FPS and 16:9 layouts | **Configurable** | Output dimensions are user-selectable |

## Three editing presets

- **Simple** — clip selection, captions, and vertical reframing. Every clip is editable.
- **Advanced** — also enables zooms, callouts, local B-roll, and other advanced preferences. Some switches remain planned; the interface labels them accordingly.
- **Custom** — enable or disable captions, reframing, B-roll, stock, motion graphics, zooms, transitions, and SFX separately. Changes are saved to the project.

### Transcript formats

- **Recommended:** `.srt` or `.vtt`, with cue start and end times. Also accepts timecoded `.txt` or `.md` lines and Whisper-like `.json` with `segments`.
- **Accepted for text extraction, but not directly clip-ready:** `.txt`, `.docx`, and `.pdf` without timestamps. PDF requires Poppler's `pdftotext`. The current application **refuses to make up timestamps**; export SRT/VTT first. Forced alignment is a next milestone.
- Word timestamps are not required for basic subtitle timing, but fine-grained karaoke animations will require finer alignment.

### Stock footage and attribution

Free stock search is powered by [Pexels API](https://www.pexels.com/api/), subject to its API usage terms and license. The app requires your Pexels API key, displays attribution, and downloads only after you click a particular result. Always check asset licensing against your use case.

### Privacy

- The app binds **only to `127.0.0.1`**, uses local same-origin checks, and serves local media only from the loaded project and export job outputs.
- OpenAI and Anthropic receive **candidate transcript passages and the prompt**, not video files. They may process text remotely.
- Ollama uses `http://127.0.0.1:11434`. Built-in heuristic needs no network.
- Stock search and downloads contact Pexels **only when manually initiated**.
- Source media are opened read-only by FFmpeg. Output files are placed in the export folder; exporting a clip again with the same ID will replace **that rendered output** but never the original video.
- User-provided API keys are held only in browser runtime state (not saved in project JSON), so avoid sharing your computer session while a key is entered.

## Development

Requires Go 1.23+, FFmpeg + ffprobe (on your PATH). No runtime npm, Python, database, or paid SDK dependencies for the built-in mode.

```bash
go test ./...
go run ./cmd/studio
```

Windows build from any Go-supported OS:

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o PodcastAIStudio.exe ./cmd/studio
```

The UI is vanilla HTML/CSS/JavaScript embedded in the Go executable via `embed.FS`. The local HTTP API is in `cmd/studio`, and transcript/AI/edit/render tools are under `internal/studio`.

## GitHub publishing

When GitHub repository creation isn't available from a connector, run the included `scripts/Publish-GitHub.ps1` **yourself** from an extracted project folder. It authenticates via GitHub CLI, asks before creating a **private** `podcast-ai-studio` repository, and pushes source files. No user password or token is stored by the project.

A GitHub Actions workflow in `.github/workflows/windows-build.yml` compiles and tests the Windows EXE whenever code is pushed. It uploads a Windows artifact. Release packaging can be extended to include an installer later.

## Planned development stages

1. **MVP hardening:** broader timestamp alignment via local Whisper, Windows end-to-end test on an RTX 3070, resilient clip previews, persistent editing presets and progress indicators.
2. **Smart editor:** face/speaker tracking, scene-change detection, cue-aware zoom timing, draggable clip/B-roll tracks, undo/redo, reusable editing template packs.
3. **Advanced effects:** automatic semantic stock/B-roll selection, audio ducking and SFX, animated word-level subtitle presets, transitions, keyframes and motion graphic themes.
4. **Product polish:** project organizer, automatic source relinking, installer with dependency manager, batch export, background queue cancellation, robust crash recovery.

## Notes

No Descript account access is requested or required. The default app does not modify Descript, Premiere, or other software projects. This is not an official product of OpenAI, Anthropic, Descript, or Pexels.

## License

The original project code is licensed under the MIT License. FFmpeg, Ollama, Pexels content, and any model dependencies have separate licenses and terms.
