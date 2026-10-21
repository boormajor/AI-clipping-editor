# Podcast AI Studio — Online Speech + Remotion (Alpha)

A Windows-first podcast clipping editor. Import a video, transcribe it online with **real word timestamps**, select clips using ChatGPT or Claude, customize captions and render with Remotion.

**Alpha:** editorial review and dynamic framing are not perfect. Always watch the rendered MP4 before publishing. The preview panel shows a simplified source preview rather than the final animated Remotion composition.

## Install

1. Open [GitHub Actions](https://github.com/boormajor/AI-clipping-editor/actions), select the most recent **successful** Windows build, and download the `PodcastAIStudio-Windows-x64-alpha` artifact.
2. Extract the **entire** ZIP, including the `remotion/` and `scripts/` folders.
3. Double-click `Install-and-Run.bat`. Setup installs/checks **FFmpeg**, **Node.js**, and **Remotion**. No Ollama, WhisperX, or local AI model is installed or downloaded. Keep the console open while editing.

## Transcribe video without an SRT

1. In **Studio**, select the podcast MP4/MOV/MKV file. **Leave Transcript empty.**
2. Under **Cloud transcription**, choose:
   - **Groq Whisper** (cloud developer API; free tier may be available subject to limits): get a key at [console.groq.com](https://console.groq.com/keys).
   - **OpenAI Whisper API** (separate pay-as-you-go API billing): get a key from [platform.openai.com](https://platform.openai.com/api-keys).
3. Paste the selected provider's speech API key into the app. This key is used only for the import request and is not written into your project JSON. Choose language or Automatic.
4. Choose export folder and click **Import episode**. The app extracts compressed audio with FFmpeg, divides long files into overlapping chunks, transcribes those chunks through the selected hosted API, and assembles the returned measured word timestamps.
5. Click **Find best moments**. Choose ChatGPT or Claude for editorial selection in **AI Connections** (your existing plan sign-in, where eligible). These providers evaluate *text*; the separate speech service is needed to transcribe the audio.

**Important:** ChatGPT Plus and Claude Pro subscriptions do not supply a general Groq/OpenAI audio-transcription API key. You need to create a separate speech API key. Free Groq tier is subject to availability and usage limits.

An existing timestamped SRT/VTT can still be imported optionally. That path doesn't automatically create per-word timestamps.

## Control captions

Open **Editing settings** and change **Words on screen at once** to any value from **1 to 12**. The default is **3 words**. This is the **maximum** words in a Remotion caption group; natural speech pauses and duration limits can make a group shorter.

The generated speech transcript includes measured start/end times for every word, so karaoke/pop/highlight effects can follow the speech. This improves timing but audio recognition can still make mistakes, particularly names and technical terminology. Correct recognized text in the editor when necessary.

Other options include caption style (kinetic, karaoke, boxed, word-box, typewriter, etc.), font, size, emphasis, primary and accent colors, framing, and aspect ratio.

## Architecture

- **Go** local Windows launcher and project API.
- **Remotion** required motion-caption and video composition engine (FFmpeg fallback remains available for failures).
- **FFmpeg** audio extraction and video pre-processing.
- **Groq Whisper or OpenAI Whisper API** remote speech recognition with real word timestamps.
- **ChatGPT / Claude** for transcript-based clip selection and editorial review through supported authenticated connections.
- No Ollama local model or WhisperX setup.

Cloud transcription sends **audio excerpts of your video to the selected speech service**. Online editorial AI receives transcript excerpts. Check your cloud provider's privacy terms and API pricing.

This release does **not** autonomously watch and rework every rendered frame. It is still under active development.

## Developer checks

```bash
go test ./...
cd remotion
npm ci
npx tsc --noEmit
```

The Windows build is created by the repository's GitHub Actions workflow.
