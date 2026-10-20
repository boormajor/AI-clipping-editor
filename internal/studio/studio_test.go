package studio

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSRT(t *testing.T) {
	data := []byte("1\n00:00:01,000 --> 00:00:03,000\nThis is a valuable marketing idea.\n\n2\n00:00:03,100 --> 00:00:05,700\nHere is another good example.\n")
	cues, plain, err := ParseTranscript("demo.srt", data)
	if err != nil || plain != "timed" || len(cues) != 2 {
		t.Fatalf("bad parse: %#v %q %v", cues, plain, err)
	}
	if cues[1].Start != 3.1 || cues[1].End != 5.7 {
		t.Fatalf("bad cue timings: %#v", cues[1])
	}
}
func TestPlainTextNeverInventsTimestamps(t *testing.T) {
	cues, raw, err := ParseTranscript("demo.txt", []byte("An important podcast episode without timestamps."))
	if err != nil || len(cues) != 0 || raw == "" {
		t.Fatal("plain text must never produce fake timecodes")
	}
}
func TestCandidateSelection(t *testing.T) {
	cues := []Cue{}
	for i := 0; i < 35; i++ {
		cues = append(cues, Cue{float64(i * 2), float64(i*2 + 2), "The biggest SEO mistake happens when people ignore their customers. We should explain why and how to fix it."})
	}
	clips, err := SelectHighlights(context.Background(), cues, 2, 15, 33, ProviderConfig{Provider: "heuristic"}, "")
	if err != nil || len(clips) != 2 {
		t.Fatalf("selection: %v %v", clips, err)
	}
	if clips[0].End <= clips[0].Start {
		t.Fatal("invalid clip")
	}
}
func TestFFmpegRender(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("FFmpeg not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	dir := t.TempDir()
	video := filepath.Join(dir, "input.mp4")
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c=blue:s=320x180:r=25:d=4", "-f", "lavfi", "-i", "sine=frequency=500:duration=4", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", video)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate sample: %v: %s", err, b)
	}
	p := Project{VideoPath: video, OutputDir: filepath.Join(dir, "exports"), Duration: 4, Cues: []Cue{{0, 2, "This is our first caption"}, {2, 4, "Our second informative caption"}}, Config: Config{Width: 360, Height: 640, FPS: 25, Captions: true, Reframe: true, Graphics: false}}
	c := Clip{ID: "clip-01", Start: 0, End: 4, Pan: .5, Captions: true, CaptionStyle: "bold"}
	path, debug, err := Render(context.Background(), p, c)
	if err != nil {
		t.Fatalf("render: %v\n%s", err, debug)
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatal(err)
	}
	duration, err := Probe(path)
	if err != nil || duration < 3.5 {
		t.Fatalf("output duration %f err %v", duration, err)
	}
}
func TestBrollOverlay(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip()
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "sample.mp4")
	broll := filepath.Join(dir, "broll.mp4")
	for _, info := range []struct{ path, color string }{{file, "blue"}, {broll, "red"}} {
		cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c="+info.color+":s=320x180:r=25:d=3", "-c:v", "libx264", info.path)
		if b, e := cmd.CombinedOutput(); e != nil {
			t.Fatalf("setup %s %v %s", info.path, e, b)
		}
	}
	p := Project{VideoPath: file, OutputDir: dir, Duration: 3, Cues: []Cue{{0, 3, "Test subtitle on top of B-roll"}}, Config: Config{Width: 360, Height: 640, FPS: 25, Broll: true, Captions: true, Graphics: true}}
	c := Clip{ID: "brolltest", Start: 0, End: 3, Pan: .5, Captions: true, Callout: "SEO TIPS", Broll: []Broll{{Path: broll, Start: .5, Duration: 1.5}}}
	_, debug, e := Render(context.Background(), p, c)
	if e != nil {
		t.Fatalf("Broll failed: %v\n%s", e, debug)
	}
}
func TestEscapeAss(t *testing.T) {
	s := EscapeAss("hello {bad} \\ world")
	if !strings.Contains(s, "\\{") {
		t.Fatal(s)
	}
}

func TestUnescapeSpeakerMarkers(t *testing.T) {
	data := []byte("1\n00:00:00,000 --> 00:00:02,000\n&amp;gt;&amp;gt; Why does topical authority matter?\n")
	cues, _, err := ParseTranscript("episode.srt", data)
	if err != nil || len(cues) != 1 {
		t.Fatalf("bad transcript: %v %#v", err, cues)
	}
	if cues[0].Text != "Why does topical authority matter?" {
		t.Fatalf("escaped title cue: %q", cues[0].Text)
	}
}
func TestASSHonorsCueBoundariesAndOffset(t *testing.T) {
	file := filepath.Join(t.TempDir(), "captions.ass")
	cfg := Defaults()
	cfg.Width = 360
	cfg.Height = 640
	cfg.CaptionFont = "Montserrat"
	cfg.CaptionSize = 38
	cfg.CaptionOffset = .45
	cue := []Cue{{Start: 1, End: 4, Text: "The first sentence aligns with the timestamp exactly."}}
	if err := WriteASS(file, cue, Clip{Start: 0, End: 6, CaptionStyle: "bold"}, cfg); err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(file)
	if !strings.Contains(string(out), "0:00:01.45,0:00:04.45") {
		t.Fatalf("timing drift: %s", out)
	}
	if strings.Count(string(out), "Dialogue:") != 1 {
		t.Fatalf("should not invent word boundaries: %s", out)
	}
	if !strings.Contains(string(out), "Montserrat,38") {
		t.Fatal("custom font/size ignored")
	}
}
func TestDualSpeakerRender(t *testing.T) {
	if _, e := exec.LookPath("ffmpeg"); e != nil {
		t.Skip("FFmpeg missing")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "pair.mp4")
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=s=640x360:r=25:d=2", "-f", "lavfi", "-i", "sine=frequency=500:duration=2", "-c:v", "libx264", "-c:a", "aac", "-shortest", src)
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("input: %v %s", e, out)
	}
	cfg := Defaults()
	cfg.Width = 360
	cfg.Height = 640
	cfg.FPS = 25
	cfg.GPU = false
	p := Project{VideoPath: src, OutputDir: filepath.Join(dir, "out"), Duration: 2, Cues: []Cue{{Start: 0, End: 2, Text: "Testing dual person layout"}}, Config: cfg}
	c := Clip{ID: "dual", Start: 0, End: 2, Layout: "two-up", Captions: true, CaptionStyle: "minimal"}
	rendered, logs, err := Render(context.Background(), p, c)
	if err != nil {
		t.Fatalf("render failed: %v %s", err, logs)
	}
	if _, e := os.Stat(rendered); e != nil {
		t.Fatal(e)
	}
}

func TestCleanClipTitle(t *testing.T) {
	for _, input := range []string{"&gt;&gt; How do EMDs affect topical authority?", "&amp;gt;&amp;gt; How do EMDs affect topical authority?"} {
		title := cleanTitle(input)
		if title != "How do EMDs affect topical authority?" {
			t.Fatalf("dirty title: %q", title)
		}
	}
}
