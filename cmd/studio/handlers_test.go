package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/boormajor/podcast-ai-studio/internal/studio"
)

func testApp(t *testing.T) *application {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	auth, err := newChatAuth(t.TempDir(), logger)
	if err != nil {
		t.Fatal(err)
	}
	return newApplication(t.TempDir(), auth, logger)
}

func do(t *testing.T, h http.HandlerFunc, method, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	r := httptest.NewRequest(method, "/api/x", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h(w, r)
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	return w, m
}

func stubVideo(t *testing.T) string {
	t.Helper()
	vid := filepath.Join(t.TempDir(), "src.mp4")
	if err := os.WriteFile(vid, []byte("stub"), 0o600); err != nil {
		t.Fatal(err)
	}
	return vid
}

func TestConfigValidation(t *testing.T) {
	a := testApp(t)
	if w, _ := do(t, a.configure, "POST", `{"width":1080,"height":1920,"fps":30}`); w.Code != 200 {
		t.Fatalf("valid config rejected: %d %s", w.Code, w.Body)
	}
	if w, _ := do(t, a.configure, "POST", `{"width":1080,"height":1920,"fps":27}`); w.Code != 400 {
		t.Fatalf("invalid fps accepted: %d", w.Code)
	}
	if w, _ := do(t, a.configure, "POST", `{"width":1081,"height":1920,"fps":30}`); w.Code != 400 {
		t.Fatalf("odd width accepted: %d", w.Code)
	}
	if w, _ := do(t, a.configure, "POST", `{"width":1080,"height":1920,"fps":30,"bogus":1}`); w.Code != 400 {
		t.Fatalf("unknown field accepted: %d", w.Code)
	}
}

func TestCancelNotFound(t *testing.T) {
	a := testApp(t)
	if w, _ := do(t, a.cancelJob, "POST", `{"id":"nope"}`); w.Code != 404 {
		t.Fatalf("expected 404 for unknown job, got %d", w.Code)
	}
}

func TestMetadataRejectsOfflineProvider(t *testing.T) {
	a := testApp(t)
	a.project.Clips = []studio.Clip{{ID: "clip-01", Start: 0, End: 5}}
	if w, _ := do(t, a.generateMetadata, "POST", `{"id":"clip-01","provider":{"provider":"heuristic"}}`); w.Code != 422 {
		t.Fatalf("expected 422 for offline provider, got %d %s", w.Code, w.Body)
	}
}

func TestCleanRequiresWords(t *testing.T) {
	a := testApp(t)
	a.project.Clips = []studio.Clip{{ID: "clip-01", Start: 0, End: 5}}
	if w, _ := do(t, a.cleanClip, "POST", `{"id":"clip-01"}`); w.Code != 422 {
		t.Fatalf("clean without words should 422, got %d", w.Code)
	}
	a.project.Clips[0].Words = []studio.Word{{Start: 0.0, End: 0.4, Word: "um"}, {Start: 1.2, End: 1.6, Word: "hello"}, {Start: 3.2, End: 3.6, Word: "world"}}
	w, m := do(t, a.cleanClip, "POST", `{"id":"clip-01"}`)
	if w.Code != 200 {
		t.Fatalf("clean with words failed: %d %s", w.Code, w.Body)
	}
	rep, _ := m["report"].(map[string]any)
	if rep == nil || rep["fillers"].(float64) < 1 {
		t.Fatalf("expected a filler detected: %v", m)
	}
	if m["suggestedStart"].(float64) <= 0 {
		t.Fatalf("expected a leading trim suggestion: %v", m["suggestedStart"])
	}
}

func TestTranscriptEdit(t *testing.T) {
	a := testApp(t)
	a.project.VideoPath = stubVideo(t)
	a.project.Cues = []studio.Cue{{Start: 0, End: 2, Text: "old text"}}
	w, _ := do(t, a.editTranscript, "POST", `{"cues":[{"start":0,"end":2,"text":"new fixed text"}]}`)
	if w.Code != 200 {
		t.Fatalf("transcript edit failed: %d %s", w.Code, w.Body)
	}
	if a.project.Cues[0].Text != "new fixed text" {
		t.Fatalf("cue text not updated: %q", a.project.Cues[0].Text)
	}
}

func TestClipPatchOnlyEditableFields(t *testing.T) {
	a := testApp(t)
	a.project.VideoPath = stubVideo(t)
	a.project.Duration = 100
	a.project.Words = []studio.Word{{Start: 11, End: 11.5, Word: "inside"}, {Start: 60, End: 60.5, Word: "outside"}}
	a.project.Clips = []studio.Clip{{ID: "clip-01", Start: 10, End: 40, Layout: "dynamic", Score: 77, Shots: []studio.Shot{{Start: 0, End: 30, Mode: "both"}}, Words: []studio.Word{{Start: 1, End: 1.5, Word: "inside"}}}}
	// Unknown/non-editable fields are rejected outright.
	if w, _ := do(t, a.updateClip, "POST", `{"id":"clip-01","score":100}`); w.Code != 400 {
		t.Fatalf("score must not be client-editable: %d %s", w.Code, w.Body)
	}
	if w, _ := do(t, a.updateClip, "POST", `{"id":"clip-01","words":[{"start":0,"end":1,"word":"fake"}]}`); w.Code != 400 {
		t.Fatalf("words must not be client-editable: %d", w.Code)
	}
	// Invalid bounds are rejected.
	if w, _ := do(t, a.updateClip, "POST", `{"id":"clip-01","start":50,"end":40}`); w.Code != 400 {
		t.Fatalf("start after end accepted: %d", w.Code)
	}
	// Changing bounds drops the shot plan and re-derives words.
	w, m := do(t, a.updateClip, "POST", `{"id":"clip-01","title":"  New title ","start":10,"end":50}`)
	if w.Code != 200 {
		t.Fatalf("patch failed: %d %s", w.Code, w.Body)
	}
	clip := m["clip"].(map[string]any)
	if clip["title"] != "New title" || clip["end"].(float64) != 50 {
		t.Fatalf("patch not applied: %v", clip)
	}
	if _, has := clip["shots"]; has {
		t.Fatalf("shot plan should be cleared when bounds change: %v", clip)
	}
	if a.project.Clips[0].Score != 77 {
		t.Fatal("score changed")
	}
	if len(a.project.Clips[0].Words) != 1 || a.project.Clips[0].Words[0].Start != 1 {
		t.Fatalf("words should be re-derived from the project: %+v", a.project.Clips[0].Words)
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	a := testApp(t)
	a.project.VideoPath = stubVideo(t)
	a.project.Duration = 30
	a.project.Name = "Persisted"
	a.mu.Lock()
	a.persistProjectLocked()
	a.mu.Unlock()
	a.jobs.restore([]*job{{ID: "j1", Kind: jobRender, Status: statusComplete, ClipID: "clip-01"}})
	a.jobs.mu.Lock()
	a.jobs.persistLocked()
	a.jobs.mu.Unlock()
	b := newApplication(a.stateDir, a.auth, a.log)
	b.loadState()
	if b.project.Name != "Persisted" {
		t.Fatalf("project not restored: %q", b.project.Name)
	}
	if _, ok := b.jobs.get("j1"); !ok {
		t.Fatalf("jobs not restored")
	}
}

func TestLoadStateIgnoresTamperedProject(t *testing.T) {
	a := testApp(t)
	bad := studio.Project{VideoPath: stubVideo(t), Duration: 30, Config: studio.Defaults(), Clips: []studio.Clip{{ID: "../../evil", Start: 0, End: 5}}}
	if err := studio.WriteJSONAtomic(a.projectFile(), bad, false); err != nil {
		t.Fatal(err)
	}
	a.loadState()
	if a.project.VideoPath != "" {
		t.Fatal("tampered project must not be loaded")
	}
}

func TestMiddlewareSecurity(t *testing.T) {
	a := testApp(t)
	a.token = "secret-token"
	h := a.routes([]string{"127.0.0.1:1234", "localhost:1234"})
	get := func(host, path string, cookie bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "http://"+host+path, nil)
		r.Host = host
		if cookie {
			r.AddCookie(&http.Cookie{Name: sessionCookie, Value: "secret-token"})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := get("evil.example:1234", "/api/status", true); w.Code != 421 {
		t.Fatalf("DNS rebinding host must be refused, got %d", w.Code)
	}
	if w := get("127.0.0.1:1234", "/api/status", false); w.Code != 401 {
		t.Fatalf("missing session token must be refused, got %d", w.Code)
	}
	if w := get("127.0.0.1:1234", "/api/status", true); w.Code != 200 {
		t.Fatalf("authorised request failed: %d %s", w.Code, w.Body)
	}
	if w := get("127.0.0.1:1234", "/", false); w.Code != 200 {
		t.Fatalf("static UI should load without a token: %d", w.Code)
	}
	// Session bootstrap sets the cookie and redirects.
	w := get("127.0.0.1:1234", "/session?token=secret-token", false)
	if w.Code != 303 || !strings.Contains(w.Header().Get("Set-Cookie"), sessionCookie+"=secret-token") {
		t.Fatalf("session bootstrap: %d %v", w.Code, w.Header())
	}
	if w := get("127.0.0.1:1234", "/session?token=wrong", false); w.Code != 403 {
		t.Fatalf("wrong token accepted: %d", w.Code)
	}
	// Cross-origin POSTs and non-JSON bodies are refused.
	post := func(origin, ctype string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://127.0.0.1:1234/api/config", strings.NewReader(`{}`))
		r.Host = "127.0.0.1:1234"
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: "secret-token"})
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		r.Header.Set("Content-Type", ctype)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := post("http://evil.example", "application/json"); w.Code != 403 {
		t.Fatalf("cross-origin POST accepted: %d", w.Code)
	}
	if w := post("http://127.0.0.1:1234", "text/plain"); w.Code != 415 {
		t.Fatalf("non-JSON POST accepted: %d", w.Code)
	}
	if w := post("http://127.0.0.1:1234", "application/json"); w.Code != 200 {
		t.Fatalf("same-origin JSON POST failed: %d %s", w.Code, w.Body)
	}
	if w := get("127.0.0.1:1234", "/api/nope", true); w.Code != 404 {
		t.Fatalf("unknown route: %d", w.Code)
	}
}

func TestImportRejectsBadInput(t *testing.T) {
	a := testApp(t)
	if w, _ := do(t, a.importEpisode, "POST", `{"videoPath":"/does/not/exist.mp4"}`); w.Code != 400 {
		t.Fatalf("missing video accepted: %d", w.Code)
	}
	if !studio.HasTool("ffprobe") {
		t.Skip("ffprobe not installed")
	}
	vid := stubVideo(t)
	if w, m := do(t, a.importEpisode, "POST", `{"videoPath":`+q(vid)+`,"speechProvider":"groq"}`); w.Code != 400 || !strings.Contains(m["error"].(string), "key") {
		t.Fatalf("missing speech key should be rejected before probing: %d %v", w.Code, m)
	}
}

func makeVideo(t *testing.T, seconds int) string {
	t.Helper()
	if !studio.HasTool("ffmpeg") || !studio.HasTool("ffprobe") {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	video := filepath.Join(dir, "in.mp4")
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", fmt.Sprintf("color=c=blue:s=320x180:r=25:d=%d", seconds), "-f", "lavfi", "-i", fmt.Sprintf("sine=frequency=400:duration=%d", seconds), "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", video)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("make video: %v %s", err, b)
	}
	return video
}

func waitJob(t *testing.T, a *application, id string) job {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		if j, ok := a.jobs.get(id); ok && !j.active() {
			return j
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("job did not finish")
	return job{}
}

func TestImportAnalyzeRenderFlow(t *testing.T) {
	video := makeVideo(t, 60)
	dir := filepath.Dir(video)
	var srt strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&srt, "%d\n%s --> %s\nThe biggest SEO mistake creators make is ignoring search intent and topical depth.\n\n", i+1, tc(i), tc(i+1))
	}
	srtPath := filepath.Join(dir, "t.srt")
	if err := os.WriteFile(srtPath, []byte(srt.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	a := testApp(t)
	w, m := do(t, a.importEpisode, "POST", `{"videoPath":`+q(video)+`,"transcriptPath":`+q(srtPath)+`,"outputDir":`+q(filepath.Join(dir, "exports"))+`}`)
	if w.Code != 200 {
		t.Fatalf("import failed: %d %s", w.Code, w.Body)
	}
	j := waitJob(t, a, m["job"].(string))
	if j.Status != statusComplete {
		t.Fatalf("import job: %+v", j)
	}
	p, rev := a.snapshot()
	if p.Duration < 59 || len(p.Cues) != 40 || rev == 0 {
		t.Fatalf("project not loaded: duration=%v cues=%d rev=%d", p.Duration, len(p.Cues), rev)
	}
	w, m = do(t, a.analyze, "POST", `{"count":2,"min":15,"max":40,"provider":{"provider":"heuristic"}}`)
	if w.Code != 200 {
		t.Fatalf("analyze failed: %d %s", w.Code, w.Body)
	}
	proj := m["project"].(map[string]any)
	if clips, _ := proj["clips"].([]any); len(clips) == 0 {
		t.Fatalf("no clips produced")
	}
	if w, _ := do(t, a.analyze, "POST", `{"count":0,"min":15,"max":40,"provider":{"provider":"heuristic"}}`); w.Code != 400 {
		t.Fatalf("bad count accepted: %d", w.Code)
	}
	// Render falls back to FFmpeg when Remotion is not installed.
	if studio.RemotionInstalled() {
		t.Skip("Remotion installed; fallback path not exercised here")
	}
	a.project.Config.Width, a.project.Config.Height = 240, 426
	w, m = do(t, a.render, "POST", `{"id":"clip-01"}`)
	if w.Code != 200 {
		t.Fatalf("render failed: %d %s", w.Code, w.Body)
	}
	if first, _ := a.jobs.get(m["job"].(string)); first.active() {
		if w, _ := do(t, a.render, "POST", `{"id":"clip-01"}`); w.Code != 409 {
			t.Fatalf("duplicate render accepted: %d", w.Code)
		}
	}
	j = waitJob(t, a, m["job"].(string))
	if j.Status != statusComplete || j.Renderer != "ffmpeg-fallback" {
		t.Fatalf("render job: %+v", j)
	}
	if _, err := os.Stat(j.Output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(filepath.Base(j.Output), "clip-01-") {
		t.Fatalf("output name should include the clip id and slug: %s", j.Output)
	}
}

func TestRenderCancel(t *testing.T) {
	video := makeVideo(t, 30)
	a := testApp(t)
	a.project = studio.Project{VideoPath: video, Duration: 30, Config: studio.Defaults(), Cues: []studio.Cue{{Start: 0, End: 30, Text: "hello"}}, Clips: []studio.Clip{{ID: "clip-01", Start: 0, End: 30, Layout: "focus", CaptionStyle: "bold", Captions: true}}}
	a.project.Config.Width, a.project.Config.Height, a.project.Config.FPS = 1080, 1920, 60
	w, m := do(t, a.render, "POST", `{"id":"clip-01"}`)
	if w.Code != 200 {
		t.Fatalf("render: %d %s", w.Code, w.Body)
	}
	id := m["job"].(string)
	time.Sleep(300 * time.Millisecond)
	if w, _ := do(t, a.cancelJob, "POST", `{"id":"`+id+`"}`); w.Code != 200 {
		t.Fatalf("cancel: %d %s", w.Code, w.Body)
	}
	j := waitJob(t, a, id)
	if j.Status != statusCancelled && j.Status != statusComplete {
		t.Fatalf("expected cancelled (or already complete), got %+v", j)
	}
}

func TestConcurrentAccessIsRaceFree(t *testing.T) {
	a := testApp(t)
	a.project.VideoPath = stubVideo(t)
	a.project.Duration = 100
	a.project.Cues = []studio.Cue{{Start: 0, End: 2, Text: "x"}}
	a.project.Clips = []studio.Clip{{ID: "clip-01", Start: 0, End: 10, Layout: "focus"}}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(3)
		go func() { defer wg.Done(); do(t, a.status, "GET", "") }()
		go func() { defer wg.Done(); do(t, a.updateClip, "POST", `{"id":"clip-01","title":"t"}`) }()
		go func() {
			defer wg.Done()
			do(t, a.editTranscript, "POST", `{"cues":[{"start":0,"end":2,"text":"y"}]}`)
		}()
	}
	wg.Wait()
	a.shutdown(context.Background())
}

func tc(sec int) string { return fmt.Sprintf("%02d:%02d:%02d,000", sec/3600, (sec%3600)/60, sec%60) }
func q(s string) string { b, _ := json.Marshal(s); return string(b) }
