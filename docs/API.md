# Local HTTP API

The studio UI talks to the Go process over `http://127.0.0.1:<port>`. The API is local and unversioned but documented here for people who script the app or write an alternative front end.

## Conventions

- Every `/api/*` request needs the session token: the cookie set by `GET /session?token=…`, or the header `X-Studio-Token: <token>`. The token is printed by the console at launch. Start with `-no-token` to disable it in development.
- Mutating requests are `POST` with `Content-Type: application/json`. Unknown JSON fields are rejected (`400`).
- Errors are `{"error": "message", "code": "optional-code"}` with a 4xx/5xx status. Codes in use: `unauthorized`, `no-picker`.
- Times are seconds (float). Project-level `words` and `cues` are absolute; a clip's `words` and `shots` are relative to its `start`.
- "Status payload" below means:

```json
{
  "project": { ...Project },
  "revision": 3,
  "tools": {"ffmpeg": true, "ffprobe": true, "node": true, "remotion": true, "claude": false, "python": true},
  "version": "1.0.0", "commit": "abc1234", "os": "windows",
  "stateDir": "C:\\Users\\me\\AppData\\Roaming\\PodcastAIStudio",
  "logPath": "...\\studio.log"
}
```

## Project

| Route | Body | Returns |
|---|---|---|
| `GET /api/status` | – | Status payload |
| `GET /api/options` | – | Enumerations: `captionPresets`, `captionFonts`, `layouts`, `fps`, `speechProviders`, `speechLanguages`, `clipLimits {minSeconds,maxSeconds,maxCount}`, `defaults` (Config) |
| `POST /api/import` | `{videoPath, transcriptPath?, outputDir?, name?, speechProvider, speechKey, speechLanguage}` | `{"job": "<id>"}` — watch `/api/jobs`; on completion the project is replaced. `speechKey` is required unless `transcriptPath` is a timed transcript. `409` if an import is already running. |
| `POST /api/project/new` | `{}` | Status payload (settings are kept) |
| `POST /api/project/save` | `{}` | `{"path": ".../podcast-ai-studio.project.json"}` |
| `POST /api/project/open` | `{path}` | Status payload. The file is validated and the video re-probed. |
| `POST /api/config` | Config object | Status payload. Validated: even dimensions 240–3840, fps ∈ {24,25,30,60}, caption size 24–150, words 1–12, offset ±10 s, position 0.1–0.95, known template/font, `#RRGGBB` colours, instructions < 4000 chars. |

## Clips

| Route | Body | Returns |
|---|---|---|
| `POST /api/analyze` | `{count, min, max, provider}` | Status payload with new `clips`. `count` 1–30, lengths 10–240 s. `provider` is `{provider, model?, apiKey?}` with provider ∈ `heuristic`, `chatgpt`, `claude-code`, `openai`, `anthropic`. |
| `POST /api/clip` | `{id, title?, start?, end?, pan?, layout?, captions?, captionStyle?, callout?, selected?, broll?, clearShots?}` | `{"clip": Clip}`. Only these fields are editable; changing `start`/`end` clears the shot plan and re-derives words. `409` if the project changed meanwhile. |
| `POST /api/clip/review` | `{id, provider}` | `{title, contextOK, issues[], layout, shots[], summary, faceData}` and the clip is updated. Online providers only. |
| `POST /api/clip/clean` | `{id, silenceGap?}` | `{report {words, fillers, fillerWords[], silences, longestSilence, deadAir, trimStart, trimEnd}, suggestedStart, suggestedEnd}`. Needs measured words (`422` otherwise). |
| `POST /api/clip/metadata` | `{id, platform?, provider}` | `{title, description, hashtags[], tags[]}` |
| `POST /api/transcript` | `{cues?: [{start,end,text}], clipId?, words?: [{word}]}` | Status payload. Replaces cue text (timings kept) and/or the recognised text of a clip's words by index. |

### Clip object

```json
{
  "id": "clip-01", "title": "Why topical depth beats volume",
  "start": 612.4, "end": 655.9, "score": 83.5, "reason": "...", "selected": true,
  "pan": 0.5, "layout": "focus", "captions": true, "captionStyle": "kinetic", "callout": "",
  "broll": [{"path": "C:\\clips\\b.mp4", "start": 3, "duration": 4, "mode": "cover"}],
  "words": [{"start": 0.12, "end": 0.4, "word": "Why"}],
  "shots": [{"start": 0, "end": 12, "mode": "both"}],
  "reviewed": true, "reviewOK": true, "reviewSummary": "..."
}
```

`layout` ∈ `focus` (single speaker, cropped at `pan`), `two-up` (split), `dynamic` (uses `shots`; mode ∈ `both|left|right|full`). `broll.mode` ∈ `cover|pip`.

## Jobs and rendering

| Route | Body | Returns |
|---|---|---|
| `POST /api/render` | `{id}` | `{"job": "<id>", "status": "queued"}`. `409` if that clip is already rendering. Renders run one at a time; Remotion first, FFmpeg fallback. |
| `GET /api/jobs` | – | Array of jobs, newest first |
| `POST /api/jobs/cancel` | `{id}` | `{"ok": true}` or `404` |
| `POST /api/reveal` | `{jobId}` | Opens the output's folder in the file manager |
| `GET /api/media` | `?id=<job>` optional | Streams the source video (no `id`) or a finished render |

### Job object

```json
{
  "id": "6977fb19c0527e0b", "kind": "render", "status": "running",
  "clipId": "clip-01", "title": "Why topical depth beats volume",
  "progress": 42, "stage": "Rendering with Remotion",
  "output": "...\\clip-01-why-topical-depth-6977fb.mp4", "renderer": "remotion",
  "error": "", "log": "<last 1500 chars>",
  "started": "2026-10-26T10:00:00Z", "finished": null
}
```

`kind` ∈ `render|import`; `status` ∈ `queued|running|complete|failed|cancelled`; `renderer` ∈ `remotion|ffmpeg-fallback`. Full logs are in `<stateDir>/logs/<id>.log`.

## Files, stock footage, auth, lifecycle

| Route | Body | Returns |
|---|---|---|
| `POST /api/pick` | `{kind}` with kind ∈ `video|transcript|project|asset|folder` | `{"path": "..."}` (Windows only; `400` with code `no-picker` elsewhere) |
| `POST /api/stock/search` | `{query, apiKey}` | `{videos: [{id, preview, url, width, height, credit}]}` (Pexels) |
| `POST /api/stock/download` | `{url, id}` | `{"path": "<export>/stock-assets/pexels-<id>-<hash>.mp4"}` |
| `GET /api/auth/status` | – | `{connected, email, pending, error, clientRegistered}` for ChatGPT |
| `POST /api/auth/chatgpt/start` | `{}` | `{"url": "..."}` and the browser opens the OpenAI sign-in |
| `GET /api/auth/chatgpt/models` | – | `{models: [{slug, name}]}` |
| `POST /api/auth/chatgpt/disconnect` | `{}` | `{"ok": true}` |
| `GET /api/auth/claude/status` | – | `{connected, message}` from `claude auth status` |
| `POST /api/auth/claude/login` | `{}` | Opens a terminal running `claude auth login` (Windows) |
| `POST /api/quit` | `{}` | `{"ok": true}`; running jobs are cancelled and the process exits |

## Example: script a render

```bash
TOKEN=...   # from the console
H="X-Studio-Token: $TOKEN"
curl -s -H "$H" -H 'Content-Type: application/json' -d '{"videoPath":"C:/ep/42.mp4","transcriptPath":"C:/ep/42.srt"}' http://127.0.0.1:PORT/api/import
curl -s -H "$H" http://127.0.0.1:PORT/api/jobs            # wait for the import to complete
curl -s -H "$H" -H 'Content-Type: application/json' -d '{"count":5,"min":25,"max":60,"provider":{"provider":"heuristic"}}' http://127.0.0.1:PORT/api/analyze
curl -s -H "$H" -H 'Content-Type: application/json' -d '{"id":"clip-01"}' http://127.0.0.1:PORT/api/render
```
