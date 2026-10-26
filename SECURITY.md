# Security policy

## Reporting a vulnerability

Please do not open a public issue for security problems. Use GitHub's private vulnerability reporting on this repository (*Security → Report a vulnerability*) and include steps to reproduce. You will get an acknowledgement within a few days and a fix or mitigation plan once the report is confirmed.

## Threat model

Podcast AI Studio is a desktop application that runs a local web server. The main risks are other software on the same machine or malicious web pages reaching that server, and secrets leaking into files.

What the app does about it:

- **Loopback only.** The server binds to `127.0.0.1` on a random port and refuses any other bind address.
- **Host validation.** Requests whose `Host` header is not the loopback address and port are rejected, which defeats DNS-rebinding attacks from web pages.
- **Session token.** Every launch generates a random token. The app window is opened with a one-time `/session?token=…` link that converts it into an `HttpOnly; SameSite=Strict` cookie; every `/api/*` request must carry it. Other local processes and browser tabs cannot call the API without the token. `-no-token` disables this and is for development only.
- **Cross-site protections.** Mutating requests must be same-origin JSON (`Origin`, `Sec-Fetch-Site`, `Content-Type` checks) with a bounded body. Responses carry a strict Content-Security-Policy, `X-Frame-Options: DENY` and related headers.
- **Input validation.** Project files, clip edits and settings are validated before use: clip identifiers are restricted to `[A-Za-z0-9_-]`, media paths must be absolute regular files, numeric fields are range-checked, and projects re-probe their video when opened. Remotion is launched through `node` directly, never through a `.cmd` shim, so arguments are not subject to `cmd.exe` interpretation.
- **Process hygiene.** External tools run with deadlines; cancelling a job kills the whole process tree. Downloaded stock footage must come from `videos.pexels.com` and redirects are confined to that host.
- **Secrets.** The ChatGPT OAuth session is encrypted with Windows DPAPI (`chatgpt-profile.dat`); on other platforms it is stored with `0600` permissions in plain text. Speech, OpenAI, Anthropic and Pexels keys are held in memory for the session only and never written to project files or logs. The ChatGPT plan token never reaches the browser.
- **Data flow.** Only compressed audio chunks (transcription) and transcript text (editorial AI) leave the machine, and only to the provider you selected.

Known limitations:

- The Windows executable is not code-signed yet, so SmartScreen warns on first run. Verify the ZIP against `SHA256SUMS.txt` from the GitHub Release.
- The optional Python audit scripts and the Google Fonts download run with the permissions of your user account.

## Supported versions

Only the latest release receives security fixes.
