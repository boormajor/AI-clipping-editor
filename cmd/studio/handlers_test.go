package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boormajor/podcast-ai-studio/internal/studio"
)

func testApp(t *testing.T) *application {
	t.Helper()
	return &application{
		jobs:     map[string]*job{},
		cancels:  map[string]context.CancelFunc{},
		project:  studio.Project{Name: "Test", Config: studio.Defaults(), Clips: []studio.Clip{}, Cues: []studio.Cue{}},
		stateDir: t.TempDir(),
	}
}

func do(t *testing.T, h http.HandlerFunc, method, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	r := httptest.NewRequest(method, "/api/x", strings.NewReader(body))
	w := httptest.NewRecorder()
	h(w, r)
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	return w, m
}

func TestConfigValidation(t *testing.T) {
	a := testApp(t)
	if w, _ := do(t, a.configure, "POST", `{"width":1080,"height":1920,"fps":30}`); w.Code != 200 {
		t.Fatalf("valid config rejected: %d %s", w.Code, w.Body)
	}
	if w, _ := do(t, a.configure, "POST", `{"width":1080,"height":1920,"fps":27}`); w.Code != 400 {
		t.Fatalf("invalid fps accepted: %d", w.Code)
	}
}

func TestRenderCancelNotFound(t *testing.T) {
	a := testApp(t)
	if w, _ := do(t, a.renderCancel, "POST", `{"id":"nope"}`); w.Code != 404 {
		t.Fatalf("expected 404 for unknown render, got %d", w.Code)
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
	a.project.Clips[0].Words = []studio.Word{
		{Start: 0.0, End: 0.4, Word: "um"},
		{Start: 1.2, End: 1.6, Word: "hello"},
		{Start: 3.2, End: 3.6, Word: "world"},
	}
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
	a.project.VideoPath = "x.mp4" // so persistProjectLocked has something to write
	a.project.Cues = []studio.Cue{{Start: 0, End: 2, Text: "old text"}}
	w, _ := do(t, a.editTranscript, "POST", `{"cues":[{"start":0,"end":2,"text":"new fixed text"}]}`)
	if w.Code != 200 {
		t.Fatalf("transcript edit failed: %d %s", w.Code, w.Body)
	}
	if a.project.Cues[0].Text != "new fixed text" {
		t.Fatalf("cue text not updated: %q", a.project.Cues[0].Text)
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	a := testApp(t)
	// loadState only restores a project whose source video still exists.
	vid := filepath.Join(t.TempDir(), "src.mp4")
	if err := os.WriteFile(vid, []byte("stub"), 0600); err != nil {
		t.Fatal(err)
	}
	a.project.VideoPath = vid
	a.project.Name = "Persisted"
	a.mu.Lock()
	a.persistProjectLocked()
	a.jobs["j1"] = &job{ID: "j1", Status: "complete", ClipID: "clip-01"}
	a.persistJobsLocked()
	a.mu.Unlock()
	// A fresh app pointed at the same state dir should restore both.
	b := &application{jobs: map[string]*job{}, cancels: map[string]context.CancelFunc{}, stateDir: a.stateDir}
	b.loadState()
	if b.project.Name != "Persisted" {
		t.Fatalf("project not restored: %q", b.project.Name)
	}
	if _, ok := b.jobs["j1"]; !ok {
		t.Fatalf("jobs not restored")
	}
}

func TestLoadAnalyzeHeuristicFlow(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	dir := t.TempDir()
	video := filepath.Join(dir, "in.mp4")
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c=blue:s=320x180:r=25:d=60", "-f", "lavfi", "-i", "sine=frequency=400:duration=60", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", video)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("make video: %v %s", err, b)
	}
	// SRT with enough timed content for a 25s+ clip candidate.
	var srt strings.Builder
	for i := 0; i < 40; i++ {
		s := i
		e := i + 1
		srt.WriteString(itoa(i+1) + "\n" + tc(s) + " --> " + tc(e) + "\nThe biggest SEO mistake creators make is ignoring search intent and topical depth.\n\n")
	}
	srtPath := filepath.Join(dir, "t.srt")
	if err := os.WriteFile(srtPath, []byte(srt.String()), 0600); err != nil {
		t.Fatal(err)
	}
	a := testApp(t)
	w, m := do(t, a.load, "POST", `{"videoPath":`+q(video)+`,"transcriptPath":`+q(srtPath)+`,"outputDir":`+q(dir)+`,"name":""}`)
	if w.Code != 200 {
		t.Fatalf("load failed: %d %s", w.Code, w.Body)
	}
	proj, _ := m["project"].(map[string]any)
	if proj == nil || proj["duration"].(float64) < 1 {
		t.Fatalf("project not loaded: %v", m)
	}
	w, m = do(t, a.analyze, "POST", `{"count":2,"min":15,"max":40,"provider":{"provider":"heuristic"}}`)
	if w.Code != 200 {
		t.Fatalf("analyze failed: %d %s", w.Code, w.Body)
	}
	proj, _ = m["project"].(map[string]any)
	clips, _ := proj["clips"].([]any)
	if len(clips) == 0 {
		t.Fatalf("no clips produced")
	}
}

// tiny helpers to avoid strconv churn in the test body
func itoa(n int) string { return strings.TrimSpace(formatInt(n)) }
func formatInt(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
func tc(sec int) string { // SRT timecode HH:MM:SS,000
	h := sec / 3600
	m := (sec % 3600) / 60
	s := sec % 60
	return pad2(h) + ":" + pad2(m) + ":" + pad2(s) + ",000"
}
func pad2(n int) string {
	if n < 10 {
		return "0" + formatInt(n)
	}
	return formatInt(n)
}
func q(s string) string { b, _ := json.Marshal(s); return string(b) }
