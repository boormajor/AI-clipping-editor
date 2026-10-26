# Changelog

All notable changes to Podcast AI Studio are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [1.0.0] - 2026-10-26

First production release. Everything below the 0.4 alpha has been reworked.

### Added
- Background import job with per-part progress, cancellation, automatic retries with back-off, two parallel uploads and an on-disk transcription cache keyed by file and settings.
- Per-launch session token (exchanged for an HttpOnly cookie), `Host` validation against DNS rebinding, `Sec-Fetch-Site`/`Origin` checks and a panic-safe middleware.
- Project and clip validation on every entry point (open, restore, patch), including safe clip identifiers and absolute media paths; projects re-probe the source video when opened.
- Deep-copied project snapshots and a revision counter so long AI calls never write into a different project.
- Shared tool runner with context deadlines, `WaitDelay` and process-tree termination (Windows `taskkill /T`, POSIX process groups) so cancelled renders do not leave Chrome or FFmpeg behind.
- Render queue with one render at a time, unique output names (`clip-01-title-<job>.mp4`), full per-job logs under `%APPDATA%\PodcastAIStudio\logs`, and "Show in folder".
- Structured logging to `studio.log`, graceful shutdown on Ctrl+C or *Quit*, version stamping via `-ldflags`, `-version`, `-state-dir`, `-remotion`, `-log-level` flags.
- `/api/options` so the UI builds its controls from the server's enumerations; `/api/project/new`, `/api/reveal`, `/api/quit`, `/api/jobs/cancel`.
- Remotion composition rewritten in modules with a zod schema, Google Fonts loading (Inter, Montserrat, Poppins, Roboto, Archivo, Oswald, Anton, Bebas Neue) with a system-font retry when offline, aspect-aware split layouts, continuous focus panning, cross-faded B-roll (full-frame or picture-in-picture, video or image), scaled typography and stroke, configurable caption position and an optional progress bar.
- `npm run smoke`: renders a synthetic clip and checks the output; run in CI.
- Completely new front end: ES modules, a store with subscriptions, hash routing, autosaving editor with validation and dirty state, keyboard-navigable clip list, timeline handles with keyboard support, in-app dialogs, stacked toasts with persistent errors, global job polling with a queue badge, responsive layout, focus styles and labelled controls.
- Idempotent installer with a dependency stamp, winget detection, Node version check, setup log and headless-browser pre-download.
- CI on Linux (gofmt, vet, staticcheck, govulncheck, race tests, UI checks, Remotion type-check and render smoke test, Playwright UI smoke test) and Windows (PSScriptAnalyzer, bundle build); tagged releases publish a ZIP with `SHA256SUMS.txt`; Dependabot.
- Documentation: architecture, API reference, contributing guide, security policy, third-party notices, issue and pull-request templates, `.editorconfig`, `.gitattributes`, `Makefile`.

### Changed
- `/api/load` is now `/api/import` and returns a job; `/api/clip` accepts a patch of editable fields only (words, scores and shot plans are server-owned); review, clean and metadata routes moved under `/api/clip/*`; `/api/render/cancel` became `/api/jobs/cancel`.
- Candidate windows now search up to the maximum length and prefer sentence boundaries; hook phrases match whole words.
- Anthropic requests allow 4096 output tokens; model JSON is extracted robustly from fenced or prose-wrapped replies.
- Config validation covers even dimensions, caption size, words, offset, position, template, font and colours; existing settings survive a new import.
- A corrupt ChatGPT credential file is set aside instead of preventing start-up; token lifetimes default to an hour when the server omits them.

### Removed
- The dead `/api/align` route, `autopolish.js`, `Publish-GitHub.ps1`, the WhisperX references and the `(planned)` feature toggles.

### Fixed
- A silent chunk (music, long pauses, a short tail) no longer aborts the whole transcription.
- "Centre" focus rendered the right-hand speaker; focus is now a continuous crop that matches the preview.
- Selecting a clip crashed the editor because of a missing element, leaving the form half-filled.
- Caption groups blinked between back-to-back phrases; consecutive groups now cut without a fade.
- The FFmpeg fallback zoom did nothing and `drawtext` expanded `%{}` sequences in callouts.
- Two renders of one clip overwrote each other's output.

## [0.4.0] - 2026-10-21

Alpha with cloud transcription, ChatGPT/Claude selection and the first Remotion renderer.

[Unreleased]: https://github.com/boormajor/AI-clipping-editor/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/boormajor/AI-clipping-editor/releases/tag/v1.0.0
[0.4.0]: https://github.com/boormajor/AI-clipping-editor/commits/962d1ba
