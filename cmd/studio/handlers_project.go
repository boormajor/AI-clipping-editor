package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/boormajor/podcast-ai-studio/internal/studio"
)

// statusResponse is the payload of GET /api/status and of every handler that
// returns the updated project.
type statusResponse struct {
	Project  studio.Project  `json:"project"`
	Revision uint64          `json:"revision"`
	Tools    map[string]bool `json:"tools"`
	Version  string          `json:"version"`
	Commit   string          `json:"commit"`
	OS       string          `json:"os"`
	StateDir string          `json:"stateDir"`
	LogPath  string          `json:"logPath"`
}

func (a *application) statusPayload() statusResponse {
	p, rev := a.snapshot()
	return statusResponse{Project: p, Revision: rev, Tools: a.tools.get(), Version: version, Commit: commit, OS: runtime.GOOS, StateDir: a.stateDir, LogPath: a.logPath}
}

func (a *application) status(w http.ResponseWriter, _ *http.Request) { reply(w, a.statusPayload()) }

// options publishes the enumerations the UI builds its controls from, so the
// front end never drifts from what the renderer accepts.
func (a *application) options(w http.ResponseWriter, _ *http.Request) {
	reply(w, map[string]any{
		"captionPresets":  studio.CaptionPresets,
		"captionFonts":    studio.CaptionFonts,
		"layouts":         studio.Layouts,
		"fps":             studio.ValidFPS,
		"speechProviders": studio.SpeechProviders,
		"speechLanguages": studio.SpeechLanguages,
		"clipLimits":      map[string]int{"minSeconds": studio.MinClipSeconds, "maxSeconds": studio.MaxClipSeconds, "maxCount": studio.MaxClipCount},
		"defaults":        studio.Defaults(),
	})
}

type importRequest struct {
	VideoPath      string `json:"videoPath"`
	TranscriptPath string `json:"transcriptPath"`
	OutputDir      string `json:"outputDir"`
	Name           string `json:"name"`
	SpeechProvider string `json:"speechProvider"`
	SpeechKey      string `json:"speechKey"`
	SpeechLanguage string `json:"speechLanguage"`
}

// importEpisode validates the sources, then transcribes in a background job
// so the UI can show progress and cancel a long upload.
func (a *application) importEpisode(w http.ResponseWriter, r *http.Request) {
	var q importRequest
	if err := decode(r, &q); err != nil {
		fail(w, 400, err.Error())
		return
	}
	video, err := studio.AbsRegularFile(q.VideoPath)
	if err != nil {
		fail(w, 400, "Podcast video: "+err.Error())
		return
	}
	transcript := ""
	if strings.TrimSpace(q.TranscriptPath) != "" {
		if transcript, err = studio.AbsRegularFile(q.TranscriptPath); err != nil {
			fail(w, 400, "Transcript: "+err.Error())
			return
		}
	}
	outputDir := ""
	if strings.TrimSpace(q.OutputDir) != "" {
		if outputDir, err = studio.AbsDir(q.OutputDir); err != nil {
			fail(w, 400, "Export folder: "+err.Error())
			return
		}
	}
	if !studio.HasTool("ffprobe") || !studio.HasTool("ffmpeg") {
		fail(w, 400, "FFmpeg is not installed. Run Install-and-Run.bat or add ffmpeg and ffprobe to PATH.")
		return
	}
	speech := studio.CloudSpeech{Provider: q.SpeechProvider, APIKey: strings.TrimSpace(q.SpeechKey), Language: q.SpeechLanguage, CacheDir: a.cacheDir}
	if transcript == "" {
		if err := studio.ValidateSpeech(speech); err != nil {
			fail(w, 400, err.Error())
			return
		}
	}
	if a.jobs.activeFor(jobImport, "") {
		fail(w, 409, "An import is already running. Wait for it to finish or cancel it from the queue.")
		return
	}
	name := strings.TrimSpace(q.Name)
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(video), filepath.Ext(video))
	}
	current, _ := a.snapshot()
	config := current.Config // keep the user's editing preferences across imports

	j := &job{Title: name}
	a.jobs.start(jobImport, j, 2*time.Hour, func(ctx context.Context, report func(int, string)) (string, string, error) {
		report(1, "Reading video")
		duration, err := studio.Probe(ctx, video)
		if err != nil {
			return "", "", err
		}
		var cues []studio.Cue
		var words []studio.Word
		if transcript != "" {
			report(10, "Parsing transcript")
			parsed, plain, err := studio.LoadTranscript(ctx, transcript)
			if err != nil {
				return "", "", err
			}
			if len(parsed) == 0 {
				return "", "", fmt.Errorf("the transcript has no usable timestamps (%s). Leave the transcript empty to use cloud transcription", studio.Head(plain, 80))
			}
			cues = parsed
		} else {
			words, err = studio.TranscribeVideoAudio(ctx, video, duration, speech, func(done, total int, stage string) {
				pct := 10
				if total > 0 {
					pct = 10 + done*85/total
				}
				report(pct, stage)
			})
			if err != nil {
				return "", "", err
			}
			cues = studio.WordsToCues(words)
		}
		report(98, "Finishing")
		p := studio.Project{Name: name, VideoPath: video, TranscriptPath: transcript, OutputDir: outputDir, Duration: duration, Cues: cues, Words: words, Clips: []studio.Clip{}, Config: config}
		if err := p.Validate(); err != nil {
			return "", "", err
		}
		a.replaceProject(p)
		a.log.Info("episode imported", "name", name, "duration", duration, "cues", len(cues), "words", len(words))
		return video, "", nil
	})
	reply(w, map[string]any{"job": j.ID})
}

func (a *application) newProject(w http.ResponseWriter, _ *http.Request) {
	p, _ := a.snapshot()
	fresh := emptyProject()
	fresh.Config = p.Config
	a.replaceProject(fresh)
	if a.stateDir != "" {
		_ = os.Remove(a.projectFile())
	}
	reply(w, a.statusPayload())
}

func (a *application) configure(w http.ResponseWriter, r *http.Request) {
	var config studio.Config
	if err := decode(r, &config); err != nil {
		fail(w, 400, err.Error())
		return
	}
	config.Normalize()
	if err := config.Validate(); err != nil {
		fail(w, 400, err.Error())
		return
	}
	a.update(0, func(p *studio.Project) { p.Config = config })
	reply(w, a.statusPayload())
}

func (a *application) save(w http.ResponseWriter, _ *http.Request) {
	p, _ := a.snapshot()
	path, err := p.Save()
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	reply(w, map[string]any{"path": path})
}

func (a *application) open(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Path string `json:"path"`
	}
	if err := decode(r, &q); err != nil {
		fail(w, 400, err.Error())
		return
	}
	path, err := studio.AbsRegularFile(q.Path)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	p, err := studio.Load(path)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	// Re-measure the source so clip bounds are checked against reality, not
	// the number stored in the file.
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	if d, err := studio.Probe(ctx, p.VideoPath); err == nil {
		p.Duration = d
		if err := p.Validate(); err != nil {
			fail(w, 400, "project no longer matches its video: "+err.Error())
			return
		}
	}
	a.replaceProject(p)
	reply(w, a.statusPayload())
}

func (a *application) getJobs(w http.ResponseWriter, _ *http.Request) { reply(w, a.jobs.list()) }

func (a *application) cancelJob(w http.ResponseWriter, r *http.Request) {
	var q struct {
		ID string `json:"id"`
	}
	if err := decode(r, &q); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if !a.jobs.cancel(q.ID) {
		fail(w, 404, "No running job with that id")
		return
	}
	reply(w, map[string]bool{"ok": true})
}

// media streams the source video or a finished render to the preview player.
func (a *application) media(w http.ResponseWriter, r *http.Request) {
	requested := r.URL.Query().Get("id")
	path := ""
	if requested == "" {
		p, _ := a.snapshot()
		path = p.VideoPath
	} else if j, ok := a.jobs.get(requested); ok && j.Status == statusComplete && j.Kind == jobRender {
		path = j.Output
	}
	if path == "" {
		fail(w, 404, "Media unavailable")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", "video/mp4")
	http.ServeFile(w, r, path)
}

// pick opens a native Windows file or folder dialog. The PowerShell script is
// a fixed string; no caller-supplied text reaches the shell.
func (a *application) pick(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Kind string `json:"kind"`
	}
	if err := decode(r, &q); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if runtime.GOOS != "windows" {
		failCode(w, 400, "no-picker", "The native file picker is only available on Windows; paste a file path instead.")
		return
	}
	filters := map[string]string{
		"video":      "Video files (*.mp4;*.mov;*.mkv;*.webm)|*.mp4;*.mov;*.mkv;*.webm|All files (*.*)|*.*",
		"transcript": "Transcripts (*.srt;*.vtt;*.txt;*.md;*.docx;*.pdf;*.json)|*.srt;*.vtt;*.txt;*.md;*.docx;*.pdf;*.json|All files (*.*)|*.*",
		"project":    "Podcast AI Studio project (*.json)|*.json",
		"asset":      "Video or image (*.mp4;*.mov;*.mkv;*.png;*.jpg;*.jpeg;*.webp)|*.mp4;*.mov;*.mkv;*.png;*.jpg;*.jpeg;*.webp",
		"folder":     "",
	}
	filter, ok := filters[q.Kind]
	if !ok {
		fail(w, 400, "Invalid picker type")
		return
	}
	script := "Add-Type -AssemblyName System.Windows.Forms; $dlg=New-Object System.Windows.Forms.OpenFileDialog; $dlg.Filter='" + filter + "'; if($dlg.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK){[Console]::Write($dlg.FileName)}"
	if q.Kind == "folder" {
		script = "Add-Type -AssemblyName System.Windows.Forms; $dlg=New-Object System.Windows.Forms.FolderBrowserDialog; $dlg.ShowNewFolderButton=$true; if($dlg.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK){[Console]::Write($dlg.SelectedPath)}"
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-STA", "-Command", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fail(w, 500, "Picker failed: "+studio.Head(string(out), 300))
		return
	}
	reply(w, map[string]any{"path": strings.TrimSpace(string(out))})
}

// reveal opens the folder containing a rendered file in the OS file manager.
func (a *application) reveal(w http.ResponseWriter, r *http.Request) {
	var q struct {
		JobID string `json:"jobId"`
	}
	if err := decode(r, &q); err != nil {
		fail(w, 400, err.Error())
		return
	}
	j, ok := a.jobs.get(q.JobID)
	if !ok || j.Status != statusComplete || j.Output == "" {
		fail(w, 404, "No finished output for that job")
		return
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer.exe", "/select,", j.Output)
	case "darwin":
		cmd = exec.Command("open", "-R", j.Output)
	default:
		cmd = exec.Command("xdg-open", filepath.Dir(j.Output))
	}
	if err := cmd.Start(); err != nil {
		fail(w, 500, "Could not open the folder: "+err.Error())
		return
	}
	reply(w, map[string]bool{"ok": true})
}

// quit shuts the app down from the UI.
func (a *application) quit(w http.ResponseWriter, _ *http.Request) {
	reply(w, map[string]bool{"ok": true})
	go func() {
		time.Sleep(300 * time.Millisecond)
		requestShutdown()
	}()
}
