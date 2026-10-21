package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"

	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/boormajor/podcast-ai-studio/internal/studio"
	"github.com/boormajor/podcast-ai-studio/internal/ui"
)

type application struct {
	auth    *chatAuth
	mu      sync.Mutex
	project studio.Project
	jobs    map[string]*job
}
type job struct {
	ID      string    `json:"id"`
	Status  string    `json:"status"`
	ClipID  string    `json:"clipId"`
	Output  string    `json:"output,omitempty"`
	Error   string    `json:"error,omitempty"`
	Log     string    `json:"log,omitempty"`
	Started time.Time `json:"started"`
}
type apiError struct {
	Error string `json:"error"`
}

func main() {
	host := flag.String("host", "127.0.0.1", "local bind address; public hosting is not supported")
	port := flag.Int("port", 0, "HTTP port, 0 selects a random port")
	noBrowser := flag.Bool("no-browser", false, "don't open the desktop app window")
	flag.Parse()
	if *host != "127.0.0.1" {
		log.Fatal("only localhost binding is supported for protecting local video files")
	}
	auth, authErr := newChatAuth()
	if authErr != nil {
		log.Fatal("ChatGPT credential storage: ", authErr)
	}
	a := &application{auth: auth, project: studio.Project{Name: "Untitled Podcast", Config: studio.Defaults(), Clips: []studio.Clip{}, Cues: []studio.Cue{}}, jobs: map[string]*job{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", a.status)
	mux.HandleFunc("GET /api/auth/status", a.authStatus)
	mux.HandleFunc("POST /api/auth/chatgpt/start", a.authChatGPTStart)
	mux.HandleFunc("GET /api/auth/chatgpt/models", a.authChatGPTModels)
	mux.HandleFunc("POST /api/auth/chatgpt/disconnect", a.authChatGPTDisconnect)
	mux.HandleFunc("GET /api/auth/claude/status", a.authClaudeStatus)
	mux.HandleFunc("POST /api/auth/claude/login", a.authClaudeLogin)
	mux.HandleFunc("POST /api/load", a.load)
	mux.HandleFunc("POST /api/config", a.configure)
	mux.HandleFunc("POST /api/analyze", a.analyze)
	mux.HandleFunc("POST /api/clip", a.clip)
	mux.HandleFunc("POST /api/render", a.render)
	mux.HandleFunc("POST /api/align", a.alignSelected)
	mux.HandleFunc("POST /api/review", a.reviewSelected)
	mux.HandleFunc("GET /api/jobs", a.getJobs)
	mux.HandleFunc("POST /api/save", a.save)
	mux.HandleFunc("POST /api/open", a.open)
	mux.HandleFunc("POST /api/pick", a.pick)
	mux.HandleFunc("POST /api/vision", a.reviewVision)
	mux.HandleFunc("POST /api/stock/search", a.searchStock)
	mux.HandleFunc("POST /api/stock/download", a.downloadStock)
	mux.HandleFunc("GET /api/media", a.media)
	mux.Handle("GET /", http.FileServer(http.FS(ui.Assets())))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; img-src 'self' data: https://images.pexels.com; media-src 'self' blob:; connect-src 'self'; frame-ancestors 'none'")
		if r.Method == "POST" {
			origin := r.Header.Get("Origin")
			if origin != "" && origin != "http://"+r.Host {
				fail(w, 403, "Only the local app may change projects")
				return
			}
			if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
				fail(w, 415, "Expected application/json")
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 2*1024*1024)
		}
		mux.ServeHTTP(w, r)
	})
	listener, e := net.Listen("tcp", net.JoinHostPort(*host, strconv.Itoa(*port)))
	if e != nil {
		log.Fatal(e)
	}
	addr := "http://" + listener.Addr().String()
	log.Println("Podcast AI Studio at", addr)
	if !*noBrowser {
		go func() { time.Sleep(450 * time.Millisecond); openWindow(addr) }()
	}
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 8 * time.Second, IdleTimeout: 70 * time.Second}
	if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
func openWindow(url string) {
	switch runtime.GOOS {
	case "windows":
		// An installed Edge PWA-like app window feels native and requires no bundled Chromium.
		choices := []string{`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`, `C:\Program Files\Microsoft\Edge\Application\msedge.exe`}
		for _, exe := range choices {
			if _, e := os.Stat(exe); e == nil {
				if exec.Command(exe, "--app="+url, "--window-size=1440,900").Start() == nil {
					return
				}
			}
		}
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		_ = exec.Command("open", url).Start()
	default:
		_ = exec.Command("xdg-open", url).Start()
	}
}
func decode(r *http.Request, v any) error {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	return d.Decode(v)
}
func reply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, code int, s string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(apiError{Error: s})
}
func (a *application) status(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	p := a.project
	a.mu.Unlock()
	reply(w, map[string]any{"project": p, "tools": map[string]bool{"ffmpeg": has("ffmpeg"), "ffprobe": has("ffprobe"), "pdftotext": has("pdftotext"), "ollama": has("ollama"), "whisper": has("whisper-cli"), "remotion": studio.RemotionInstalled()}, "version": "0.4.0-alpha"})
}
func has(v string) bool { _, e := exec.LookPath(v); return e == nil }
func (a *application) load(w http.ResponseWriter, r *http.Request) {
	var q struct {
		VideoPath      string `json:"videoPath"`
		TranscriptPath string `json:"transcriptPath"`
		OutputDir      string `json:"outputDir"`
		Name           string `json:"name"`
	}
	if e := decode(r, &q); e != nil {
		fail(w, 400, e.Error())
		return
	}
	for _, path := range []string{q.VideoPath, q.TranscriptPath} {
		info, e := os.Stat(path)
		if e != nil || info.IsDir() {
			fail(w, 400, "Cannot read source file: "+path)
			return
		}
	}
	if !has("ffprobe") {
		fail(w, 400, "Install FFmpeg and add ffprobe to PATH before importing")
		return
	}
	duration, e := studio.Probe(q.VideoPath)
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	cues, plain, e := studio.LoadTranscript(q.TranscriptPath)
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	if len(cues) == 0 {
		fail(w, 422, "Your transcript has no usable timestamps. Export SRT or VTT from Descript, or align this document using an external transcript-alignment tool before importing. Text sample: "+limit(plain, 100))
		return
	}
	name := strings.TrimSpace(q.Name)
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(q.VideoPath), filepath.Ext(q.VideoPath))
	}
	a.mu.Lock()
	a.project = studio.Project{Name: name, VideoPath: q.VideoPath, TranscriptPath: q.TranscriptPath, OutputDir: q.OutputDir, Duration: duration, Cues: cues, Clips: []studio.Clip{}, Config: studio.Defaults(), LastUpdated: time.Now()}
	a.mu.Unlock()
	a.status(w, r)
}
func limit(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
func (a *application) configure(w http.ResponseWriter, r *http.Request) {
	var config studio.Config
	if e := decode(r, &config); e != nil {
		fail(w, 400, e.Error())
		return
	}
	if config.Width < 240 || config.Height < 240 || config.Width > 3840 || config.Height > 3840 || !(config.FPS == 24 || config.FPS == 25 || config.FPS == 30 || config.FPS == 60) {
		fail(w, 400, "Invalid export resolution or FPS")
		return
	}
	a.mu.Lock()
	a.project.Config = config
	a.project.LastUpdated = time.Now()
	a.mu.Unlock()
	a.status(w, r)
}
func (a *application) analyze(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Count    int                   `json:"count"`
		Min      float64               `json:"min"`
		Max      float64               `json:"max"`
		Provider studio.ProviderConfig `json:"provider"`
	}
	if e := decode(r, &q); e != nil {
		fail(w, 400, e.Error())
		return
	}
	a.mu.Lock()
	p := a.project
	a.mu.Unlock()
	if len(p.Cues) == 0 {
		fail(w, 400, "Import a video and timestamped transcript first")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 110*time.Second)
	defer cancel()
	if q.Provider.Provider == "chatgpt" {
		token, err := a.auth.accessToken(ctx)
		if err != nil {
			fail(w, 401, err.Error())
			return
		}
		q.Provider.APIKey = token // server-side only: never exposed to the browser
		if strings.TrimSpace(q.Provider.Model) == "" {
			models, err := a.auth.models(ctx)
			if err != nil {
				fail(w, 400, "Could not list models for ChatGPT plan: "+err.Error())
				return
			}
			if len(models) == 0 {
				fail(w, 400, "No eligible ChatGPT models are available for this account")
				return
			}
			q.Provider.Model = models[0]["slug"]
		}
	}
	clips, e := studio.SelectHighlights(ctx, p.Cues, q.Count, q.Min, q.Max, q.Provider, p.Config.Instructions)
	if e != nil {
		fail(w, 422, e.Error())
		return
	}
	for i := range clips {
		clips[i].CaptionStyle = p.Config.CaptionStyle
		clips[i].Captions = p.Config.Captions
	}
	a.mu.Lock()
	a.project.Clips = clips
	a.project.LastUpdated = time.Now()
	a.mu.Unlock()
	a.status(w, r)
}
func (a *application) clip(w http.ResponseWriter, r *http.Request) {
	var c studio.Clip
	if e := decode(r, &c); e != nil {
		fail(w, 400, e.Error())
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	found := false
	for i := range a.project.Clips {
		if a.project.Clips[i].ID == c.ID {
			if c.Start < 0 || c.End <= c.Start || c.End > a.project.Duration+.5 {
				fail(w, 400, "Invalid clip timestamps")
				return
			}
			if c.Pan < 0 || c.Pan > 1 {
				fail(w, 400, "Pan must be 0..1")
				return
			}
			if len(c.Broll) > 8 {
				fail(w, 400, "At most 8 local B-roll overlays per clip")
				return
			}
			a.project.Clips[i] = c
			found = true
			break
		}
	}
	if !found {
		fail(w, 404, "Clip does not exist")
		return
	}
	a.project.LastUpdated = time.Now()
	reply(w, map[string]any{"ok": true})
}
func (a *application) render(w http.ResponseWriter, r *http.Request) {
	var q struct {
		ID string `json:"id"`
	}
	if e := decode(r, &q); e != nil {
		fail(w, 400, e.Error())
		return
	}
	a.mu.Lock()
	p := a.project
	var c studio.Clip
	found := false
	for _, v := range p.Clips {
		if v.ID == q.ID {
			c = v
			found = true
			break
		}
	}
	if !found {
		a.mu.Unlock()
		fail(w, 404, "Clip not found")
		return
	}
	var b [8]byte
	_, _ = rand.Read(b[:])
	id := hex.EncodeToString(b[:])
	j := &job{ID: id, ClipID: c.ID, Status: "queued", Started: time.Now()}
	a.jobs[id] = j
	a.mu.Unlock()
	go func() {
		a.mu.Lock()
		j.Status = "rendering"
		a.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), studio.RenderTimeout())
		defer cancel()
		out, debug, err := studio.RenderRemotion(ctx, p, c)
		a.mu.Lock()
		defer a.mu.Unlock()
		j.Log = limit(debug, 1000)
		if err != nil {
			j.Status = "failed"
			j.Error = err.Error()
		} else {
			j.Status = "complete"
			j.Output = out
		}
	}()
	reply(w, map[string]any{"id": id, "status": "queued"})
}
func (a *application) getJobs(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	list := []*job{}
	for _, j := range a.jobs {
		cp := *j
		list = append(list, &cp)
	}
	reply(w, list)
}
func saveProject(p studio.Project) (string, error) {
	if p.VideoPath == "" {
		return "", errors.New("import a project first")
	}
	out := p.OutputDir
	if out == "" {
		out = filepath.Join(filepath.Dir(p.VideoPath), "PodcastAI-Exports")
	}
	if e := os.MkdirAll(out, 0755); e != nil {
		return "", e
	}
	// Project includes original paths but never embeds or overwrites source media or API credentials.
	path := filepath.Join(out, "podcast-ai-studio.project.json")
	b, e := json.MarshalIndent(p, "", "  ")
	if e != nil {
		return "", e
	}
	return path, os.WriteFile(path, b, 0600)
}
func (a *application) save(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	p := a.project
	a.mu.Unlock()
	path, e := saveProject(p)
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	reply(w, map[string]any{"path": path})
}
func (a *application) open(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Path string `json:"path"`
	}
	if e := decode(r, &q); e != nil {
		fail(w, 400, e.Error())
		return
	}
	b, e := os.ReadFile(q.Path)
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	var p studio.Project
	if e = json.Unmarshal(b, &p); e != nil {
		fail(w, 400, e.Error())
		return
	}
	if p.VideoPath == "" {
		fail(w, 400, "Invalid project: missing video")
		return
	}
	if _, e = os.Stat(p.VideoPath); e != nil {
		fail(w, 400, "Video source missing: "+p.VideoPath)
		return
	}
	a.mu.Lock()
	a.project = p
	a.mu.Unlock()
	a.status(w, r)
}
func (a *application) pick(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Kind string `json:"kind"`
	}
	if e := decode(r, &q); e != nil {
		fail(w, 400, e.Error())
		return
	}
	if runtime.GOOS != "windows" {
		fail(w, 400, "Native file picker is available on Windows; paste a file path here on other systems")
		return
	}
	if q.Kind != "video" && q.Kind != "transcript" && q.Kind != "folder" && q.Kind != "project" && q.Kind != "asset" {
		fail(w, 400, "Invalid picker type")
		return
	}
	// PowerShell is used only for a trusted local file-selection dialog. No user-supplied shell commands.
	filter := "All files (*.*)|*.*"
	switch q.Kind {
	case "video":
		filter = "Video files (*.mp4;*.mov;*.mkv;*.webm)|*.mp4;*.mov;*.mkv;*.webm|All files (*.*)|*.*"
	case "transcript":
		filter = "Transcripts (*.srt;*.vtt;*.txt;*.md;*.docx;*.pdf;*.json)|*.srt;*.vtt;*.txt;*.md;*.docx;*.pdf;*.json|All files (*.*)|*.*"
	case "project":
		filter = "Podcast Studio (*.json)|*.json"
	case "asset":
		filter = "Video (*.mp4;*.mov;*.mkv)|*.mp4;*.mov;*.mkv"
	}
	script := "Add-Type -AssemblyName System.Windows.Forms; $dlg=New-Object System.Windows.Forms.OpenFileDialog; $dlg.Filter='" + filter + "'; if($dlg.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK){[Console]::Write($dlg.FileName)}"
	if q.Kind == "folder" {
		script = "Add-Type -AssemblyName System.Windows.Forms; $dlg=New-Object System.Windows.Forms.FolderBrowserDialog; if($dlg.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK){[Console]::Write($dlg.SelectedPath)}"
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-STA", "-Command", script)
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	cmd = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-STA", "-Command", script)
	out, e := cmd.CombinedOutput()
	if e != nil {
		fail(w, 500, "Picker failed: "+limit(string(out), 300))
		return
	}
	reply(w, map[string]any{"path": strings.TrimSpace(string(out))})
}
func (a *application) searchStock(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Query  string `json:"query"`
		APIKey string `json:"apiKey"`
	}
	if e := decode(r, &q); e != nil {
		fail(w, 400, e.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	results, e := studio.SearchStock(ctx, q.APIKey, q.Query)
	if e != nil {
		fail(w, 422, e.Error())
		return
	}
	reply(w, map[string]any{"videos": results})
}
func (a *application) downloadStock(w http.ResponseWriter, r *http.Request) {
	var q struct {
		URL string `json:"url"`
		ID  int    `json:"id"`
	}
	if e := decode(r, &q); e != nil {
		fail(w, 400, e.Error())
		return
	}
	a.mu.Lock()
	p := a.project
	a.mu.Unlock()
	if p.VideoPath == "" {
		fail(w, 400, "import a podcast first")
		return
	}
	dir := p.OutputDir
	if dir == "" {
		dir = filepath.Join(filepath.Dir(p.VideoPath), "PodcastAI-Exports")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 155*time.Second)
	defer cancel()
	path, e := studio.DownloadStock(ctx, q.URL, filepath.Join(dir, "stock-assets"), q.ID)
	if e != nil {
		fail(w, 422, e.Error())
		return
	}
	reply(w, map[string]any{"path": path})
}
func (a *application) media(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	requested := r.URL.Query().Get("id")
	path := a.project.VideoPath
	if requested != "" {
		path = ""
		if j, ok := a.jobs[requested]; ok && j.Status == "complete" {
			path = j.Output
		}
	}
	a.mu.Unlock()
	if path == "" {
		fail(w, 404, "Media unavailable")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	http.ServeFile(w, r, path)
}

// Subscription account access stays exclusively within the local Go process.
func (a *application) authStatus(w http.ResponseWriter, r *http.Request) { reply(w, a.auth.status()) }
func (a *application) authChatGPTStart(w http.ResponseWriter, r *http.Request) {
	url, err := a.auth.start()
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	reply(w, map[string]string{"url": url})
	go openSignInBrowser(url)
}
func (a *application) authChatGPTModels(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	models, e := a.auth.models(ctx)
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	reply(w, map[string]any{"models": models})
}
func (a *application) authChatGPTDisconnect(w http.ResponseWriter, r *http.Request) {
	if e := a.auth.disconnect(); e != nil {
		fail(w, 500, e.Error())
		return
	}
	reply(w, map[string]bool{"ok": true})
}
func (a *application) authClaudeStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	connected, message := studio.ClaudeCodeStatus(ctx)
	reply(w, map[string]any{"connected": connected, "message": message})
}
func openSignInBrowser(url string) {
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		_ = exec.Command("open", url).Start()
	default:
		_ = exec.Command("xdg-open", url).Start()
	}
}

func (a *application) authClaudeLogin(w http.ResponseWriter, r *http.Request) {
	if !has("claude") {
		fail(w, 400, "Install the official Claude Code CLI first: https://code.claude.com/docs/en/setup")
		return
	}
	if runtime.GOOS != "windows" {
		fail(w, 400, "Open a terminal and run: claude auth login")
		return
	}
	// Fixed command only. No caller-supplied strings enter the shell.
	if err := exec.Command("cmd", "/c", "start", "", "cmd", "/k", "claude", "auth", "login").Start(); err != nil {
		fail(w, 500, "Unable to open Claude login terminal: "+err.Error())
		return
	}
	reply(w, map[string]bool{"ok": true})
}
