# Contributing

Thanks for helping improve Podcast AI Studio. This guide covers setting up a development environment, the checks every change must pass, and how releases are cut.

## Development setup

Requirements: Go (version in `go.mod`), Node.js 22, FFmpeg on `PATH`, Python 3.11+ (only for the optional audit scripts and the UI smoke test).

```bash
git clone https://github.com/boormajor/AI-clipping-editor
cd AI-clipping-editor
cd remotion && npm ci && cd ..
go run ./cmd/studio -remotion ./remotion -no-token -log-level debug
```

`-no-token` lets you open `http://127.0.0.1:<port>` directly; without it use the `/session?token=…` link the app prints. `-remotion` points the renderer at the checkout instead of a folder beside the executable.

Useful loops:

- Front end: edit files under `internal/ui`, restart the Go process (assets are embedded at build time).
- Renderer: `cd remotion && npm run studio` opens the Remotion Studio with the `PodcastClip` composition and its default props; `npm run smoke` renders a synthetic clip.
- Backend: `go test -race ./...`.

## Checks

The `make check` target (or the individual commands) mirrors CI:

| Check | Command |
|---|---|
| Formatting | `gofmt -l .` must print nothing |
| Static analysis | `go vet ./...`, `staticcheck ./...`, `govulncheck ./...` |
| Tests | `go test -race ./...` |
| Front end | `node scripts/check-ui-ids.mjs` |
| Renderer | `cd remotion && npm run typecheck && npm run smoke` |
| UI smoke | `python scripts/ui_smoke.py <built binary>` (needs `pip install playwright && playwright install chromium`) |
| PowerShell | `Invoke-ScriptAnalyzer -Path scripts -Recurse` |

## Code layout

```
cmd/studio/          Go entry point, HTTP routes, job manager, auth (see docs/ARCHITECTURE.md)
internal/studio/     Domain logic: transcripts, cloud speech, clip selection, Remotion and FFmpeg rendering
internal/ui/         Embedded front end (no build step: ES modules + CSS)
remotion/            Remotion composition (TypeScript) and its smoke test
scripts/             Installer, bundle build, optional Python audits, CI helpers
docs/                Architecture, API reference, screenshots
```

## Conventions

- Go: standard formatting, errors wrapped with context, no global mutable state outside `application`. External commands go through `studio.Tool` so cancellation kills the process tree. Anything a client sends is validated with `Config.Validate`, `Clip.Validate` or `Project.Validate`.
- Front end: no frameworks, no HTML strings built from data (use `h()` and `textContent`), every interactive element reachable by keyboard, every input labelled. Values the server enumerates come from `/api/options`.
- Renderer: props are validated by the zod schema in `remotion/src/schema.ts`; keep `internal/studio/models.go` (`CaptionPresets`, `CaptionFonts`, `Layouts`) in sync with it.
- Never fabricate word timings. Word-level effects only use measured timestamps.
- Keep secrets out of logs and project files.

## Pull requests

1. Open an issue first for anything larger than a bug fix so the approach can be agreed.
2. Branch from `main`, keep commits focused, and add or update tests with behaviour changes.
3. Update `CHANGELOG.md` under *Unreleased* and the docs if the API, UI or installer changed.
4. Make sure CI is green. A maintainer will review and squash-merge.

## Releases

1. Move the *Unreleased* notes in `CHANGELOG.md` under a new version heading with today's date.
2. Tag and push: `git tag v1.2.3 && git push origin v1.2.3`.
3. The `Release` workflow builds the Windows bundle, stamps the version into the binary, generates `SHA256SUMS.txt`, and publishes a GitHub Release with the changelog section as notes.

## Reporting problems

Use the issue templates. For security issues follow [SECURITY.md](SECURITY.md) instead of opening a public issue.
