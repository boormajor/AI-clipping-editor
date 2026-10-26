# Example transcripts

Files you can import to try the editor without a speech API key:

- `example.srt` – SubRip with timestamps (cue-level captions).
- `example.vtt` – the same content as WebVTT.
- `example.segments.json` – Whisper-style `{"segments": [{"start","end","text"}]}`.

Timed transcripts give cue-level captions. For word-by-word caption templates import the video with cloud transcription instead, which returns a measured timestamp for every word.

Plain `.txt`, `.md`, `.docx` and `.pdf` transcripts are accepted only when every line carries a timestamp (`[00:01:23] text`); the app never guesses timings for untimed text.
