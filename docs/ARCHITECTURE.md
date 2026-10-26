# Architecture

Podcast AI Studio is a single Windows executable that serves an embedded web UI on `127.0.0.1` and drives three external engines: FFmpeg, a hosted Whisper API, and Remotion (Node + headless Chrome). This document describes the process model, the data flow and the invariants the code relies on.

## Process model

```
┌───────────────────────────────────────────────────────────────────┐
│ PodcastAIStudio.exe (Go)                                          │
│                                                                   │
│  cmd/studio          internal/studio            internal/ui       │
│  ┌──────────────┐    ┌───────────────────┐      ┌────────────┐   │
│  │ HTTP server  │───▶│ transcript parse  │      │ index.html │   │
│  │ middleware   │    │ cloud speech      │      │ css/  js/  │   │
│  │ handlers     │    │ highlights (AI)   │      └────────────┘   │
│  │ job manager  │───▶│ remotion bridge   │──┐   served embedded  │
│  │ chatgpt auth │    │ ffmpeg fallback   │  │                    │
│  └──────────────┘    │ tool runner       │  │                    │
└──────────┬───────────┴───────────────────┴──┼────────────────────┘
           │ loopback HTTP + session cookie    │ exec
   ┌───────▼────────┐                 ┌────────▼─────────┐  ┌──────────┐
   │ Edge app window│                 │ node remotion-cli│  │ ffmpeg / │
   │ (or browser)   │                 │ + headless Chrome│  │ ffprobe  │
   └────────────────┘                 └──────────────────┘  └──────────┘
                                                 ▲
   external: Groq / OpenAI Whisper, OpenAI Responses, Anthropic Messages,
             ChatGPT OAuth, Claude Code CLI, Pexels
```

- **`cmd/studio`** owns process concerns: flags, logging (`log/slog` to `studio.log`), the HTTP server and middleware, the job manager, persistence of the last project and job history, and the ChatGPT OAuth client. State lives in one `application` value guarded by a mutex; handlers work on deep-copied snapshots and commit changes under the lock with a revision check.
- **`internal/studio`** is the domain: transcript parsing (SRT/VTT/TXT/DOCX/JSON/PDF), cloud transcription, candidate ranking and AI selection, project validation, the Remotion bridge, the FFmpeg fallback renderer, and `Tool`, the single way external commands are launched.
- **`internal/ui`** is embedded with `go:embed`. It is plain ES modules and CSS, no build step, served with `Cache-Control: no-cache`.
- **`remotion/`** holds the composition (`src/`), validated by a zod schema that mirrors `remotionProps` in Go.

## Request security

Every request passes `application.middleware` (see `server.go`):

1. `Host` must match the loopback listener (`127.0.0.1:port`, `localhost:port`, `[::1]:port`).
2. `/api/*` requires the per-launch token, delivered as an `HttpOnly` cookie by `/session?token=…` or as `X-Studio-Token`.
3. Mutating requests must have a same-origin `Origin`/`Sec-Fetch-Site`, `Content-Type: application/json` and a body under 8 MB.
4. Panics are recovered and logged.

## Data flow

### Import (`POST /api/import` → job)

1. Validate paths and the speech configuration synchronously so the user gets immediate feedback.
2. Start an `import` job. The job probes the duration with `ffprobe`, then either parses the transcript file or runs `TranscribeVideoAudio`:
   - the episode is planned into ~470 s chunks with 1 s overlap (tiny tails fold into the previous chunk);
   - two workers extract mono 16 kHz MP3 with FFmpeg and upload to Groq or OpenAI; 429/5xx responses retry with back-off honouring `Retry-After`; results are cached per chunk under `%APPDATA%\PodcastAIStudio\cache\speech`;
   - chunk results are merged by word midpoint so overlaps are not double-counted, then sorted.
3. Words are grouped into readable cues (`WordsToCues`). The project is validated and installed with a new revision; the previous editing settings are kept.

### Clip selection (`POST /api/analyze`)

`Candidates` slides over cues building windows between the requested lengths, scoring hook phrases (whole-word), questions, duration sweet spot and sentence endings, then removes overlaps. Online providers receive the top 35 candidates and return ids, titles and reasons; the list is topped up from the ranked pool to reach the requested count (those extras are labelled as not AI-selected). Each clip gets the project's words re-based to the clip (`WordsWithin`).

### Editing (`POST /api/clip`)

The client sends a patch of editable fields only (`clipPatch`). Changing the bounds clears the AI shot plan and re-derives words from the project; `Clip.Validate` runs before commit. Words, scores and shots never come from the client.

### Review (`POST /api/clip/review`)

Builds a prompt from the text before, inside and after the clip, optional face/scene metadata from the Python audits (run once on a downscaled extract), and the user's instructions. The model's layout and shot plan are normalised (`normalizeShots`) and committed only if the project revision is unchanged.

### Render (`POST /api/render` → job)

Renders run one at a time (semaphore in `jobManager`). `RenderRemotion`:

1. cuts the clip from the source with FFmpeg into `remotion/public/jobs/clip-XXXX/source.mp4` (Remotion can only read from `public/`);
2. prepares B-roll (video re-encoded to the clip, images copied);
3. writes `props.json` and runs `node node_modules/@remotion/cli/remotion-cli.js render … --props …` with a 60 s component timeout;
4. parses `Rendered n/m` / `Encoded n/m` lines for progress;
5. if the log shows the caption font could not be downloaded, retries once with Arial.

If Remotion fails for any other reason, `Render` (FFmpeg) produces the clip with burned-in ASS captions, overlays and a static callout, and the job is tagged `ffmpeg-fallback`. Output names are `<clip id>-<title slug>-<job id>.mp4` so repeated renders never collide.

### Persistence

- `last-project.json` and `jobs.json` in the state folder are written atomically after every change and restored at start-up (after validation; interrupted jobs become `failed`).
- `podcast-ai-studio.project.json` in the export folder is the user-facing save file (`POST /api/project/save`, `POST /api/project/open`). Opening re-probes the video.
- Full job logs are written to `logs/<job id>.log`; the API carries the last 1500 characters.

## Caption timing model

- Project words are absolute seconds in the source; clip words are relative to the clip start.
- Cues are absolute; `makeRemotionProps` re-bases and clips them to the clip.
- The renderer groups measured words by pauses (>0.32 s), phrase length (>2.35 s), a maximum word count and sentence endings. Group entrances are skipped when the previous group ended less than 0.18 s earlier so consecutive phrases cut rather than blink.
- When no measured words exist, whole cues are shown with the group animation. Word timings are never interpolated.

## Front end

`js/store.js` holds the state (`project`, `revision`, `jobs`, `selectedID`, `provider`, `options`, `route`) with key-based subscriptions. Views subscribe to what they render:

- `views/import.js` – the three-step import form and job progress.
- `views/clips.js` – find bar, keyboard-navigable clip list, batch review/render.
- `views/editor.js` – draft/patch model with debounced autosave, timeline handles (pointer + keyboard), B-roll and stock search, AI actions.
- `views/preview.js` – source playback cropped by pan and the caption overlay grouped like the renderer.
- `views/settings.js`, `views/connections.js`, `views/queue.js`.

The queue polls `/api/jobs` while anything is active (backing off on errors) and notifies on completion from any page.

## Invariants worth keeping

- `studio.CaptionPresets`, `CaptionFonts` and `Layouts` must match `remotion/src/schema.ts` and `fonts.ts`.
- All external commands go through `studio.Tool` (deadlines + process-tree kill).
- Any data from disk or a client passes `Validate` before it touches `application.project`.
- Secrets (speech keys, API keys, OAuth tokens) are never logged or persisted outside the DPAPI-protected profile.
