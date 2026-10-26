# Third-party notices

Podcast AI Studio is released under the MIT License (see `LICENSE`). It builds on and talks to the following third-party software and services, each under its own terms.

## Bundled or installed with the app

| Component | Licence / terms | Notes |
|---|---|---|
| [Remotion](https://remotion.dev) (`remotion`, `@remotion/*`) | [Remotion License](https://github.com/remotion-dev/remotion/blob/main/LICENSE.md) | Free for individuals, non-profits and companies with up to three people. **Larger companies need a company licence** from Remotion to render videos with this app. |
| [React](https://react.dev) | MIT | Renderer runtime. |
| [zod](https://zod.dev) | MIT | Props validation. |
| Google Fonts (Inter, Montserrat, Poppins, Roboto, Archivo, Oswald, Anton, Bebas Neue) | SIL Open Font License 1.1 | Downloaded at render time by `@remotion/google-fonts`. |
| Chrome Headless Shell | BSD-style (Chromium) | Downloaded by Remotion for rendering. |
| [FFmpeg](https://ffmpeg.org) | LGPL 2.1+ / GPL 2+ depending on build | Installed separately through `winget` (Gyan.FFmpeg); not redistributed in this repository. |
| [Node.js](https://nodejs.org) | MIT | Installed separately through `winget`. |
| Optional: MediaPipe, OpenCV, PySceneDetect | Apache 2.0 / Apache 2.0 / BSD 3-clause | Only if you install `scripts/requirements.txt`. |

## Services you may connect

| Service | Terms |
|---|---|
| Groq Whisper, OpenAI Whisper / Responses API, Anthropic API | The provider's API terms; usage is billed to your account. |
| ChatGPT sign-in | OpenAI's terms for authorised apps. Eligible requests count towards your plan limits. |
| Claude Code | Anthropic's consumer terms for your subscription. |
| [Pexels](https://www.pexels.com/license/) | Pexels licence for stock footage; attribution appreciated. |

Content you create with Podcast AI Studio is yours. Check your podcast guests' consent and the platform rules where you publish.
