package studio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigValidate(t *testing.T) {
	cfg := Defaults()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("defaults must validate: %v", err)
	}
	cases := map[string]func(c *Config){
		"odd width":        func(c *Config) { c.Width = 1081 },
		"bad fps":          func(c *Config) { c.FPS = 27 },
		"small font":       func(c *Config) { c.CaptionSize = 10 },
		"too many words":   func(c *Config) { c.CaptionWords = 40 },
		"offset":           func(c *Config) { c.CaptionOffset = 42 },
		"unknown preset":   func(c *Config) { c.CaptionStyle = "comic" },
		"unknown font":     func(c *Config) { c.CaptionFont = "Wingdings" },
		"bad colour":       func(c *Config) { c.AccentColor = "yellow" },
		"long instruction": func(c *Config) { c.Instructions = strings.Repeat("x", 5000) },
	}
	for name, mutate := range cases {
		c := Defaults()
		mutate(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestConfigNormalizeFillsDefaults(t *testing.T) {
	var c Config
	c.Normalize()
	if c.CaptionStyle != "kinetic" || c.Width != 1080 || c.CaptionFont != "Inter" || c.CaptionPosition == 0 {
		t.Fatalf("normalize did not fill defaults: %+v", c)
	}
	c.CaptionColor = "nope"
	c.Normalize()
	if c.CaptionColor != "#FFFFFF" {
		t.Fatal("invalid colour must fall back to default")
	}
}

func TestClipValidate(t *testing.T) {
	good := Clip{ID: "clip-01", Start: 10, End: 40, Pan: 0.5, Layout: "focus", CaptionStyle: "bold"}
	if err := good.Validate(100); err != nil {
		t.Fatal(err)
	}
	bad := []Clip{
		{ID: "../evil", Start: 0, End: 5},
		{ID: "clip-01", Start: 5, End: 4},
		{ID: "clip-01", Start: 0, End: 500},
		{ID: "clip-01", Start: 0, End: 10, Pan: 2},
		{ID: "clip-01", Start: 0, End: 10, Layout: "spiral"},
		{ID: "clip-01", Start: 0, End: 10, Broll: []Broll{{Path: "", Start: 0, Duration: 1}}},
		{ID: "clip-01", Start: 0, End: 10, Shots: []Shot{{Start: 0, End: 30, Mode: "both"}}},
	}
	for i, c := range bad {
		if err := c.Validate(100); err == nil {
			t.Errorf("case %d should fail: %+v", i, c)
		}
	}
}

func TestProjectCloneIsDeep(t *testing.T) {
	p := Project{Clips: []Clip{{ID: "clip-01", Broll: []Broll{{Path: "a", Duration: 1}}, Words: []Word{{Word: "hi"}}}}, Cues: []Cue{{Text: "x"}}}
	c := p.Clone()
	c.Clips[0].Words[0].Word = "changed"
	c.Clips[0].Broll[0].Path = "b"
	c.Cues[0].Text = "y"
	if p.Clips[0].Words[0].Word != "hi" || p.Clips[0].Broll[0].Path != "a" || p.Cues[0].Text != "x" {
		t.Fatal("clone shares memory with the original")
	}
}

func TestProjectValidateRejectsTamperedFile(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "v.mp4")
	if err := os.WriteFile(video, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := Project{VideoPath: video, Duration: 60, Config: Defaults(), Clips: []Clip{{ID: "x\" & calc & \"", Start: 0, End: 5}}}
	if err := p.Validate(); err == nil {
		t.Fatal("clip id with shell characters must be rejected")
	}
	p.Clips = []Clip{{ID: "clip-01", Start: 0, End: 5, Broll: []Broll{{Path: "relative.mp4", Start: 0, Duration: 1}}}}
	if err := p.Validate(); err == nil {
		t.Fatal("relative B-roll paths must be rejected")
	}
	p.Clips = []Clip{{ID: "clip-01", Start: 0, End: 5}}
	p.OutputDir = "exports"
	if err := p.Validate(); err == nil {
		t.Fatal("relative export folder must be rejected")
	}
	p.OutputDir = ""
	if err := p.Validate(); err != nil {
		t.Fatalf("valid project rejected: %v", err)
	}
	if p.Clips[0].Layout != "focus" || p.Clips[0].CaptionStyle != "kinetic" {
		t.Fatalf("validate should fill clip defaults: %+v", p.Clips[0])
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "v.mp4")
	if err := os.WriteFile(video, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := Project{Name: "Round", VideoPath: video, OutputDir: filepath.Join(dir, "out"), Duration: 30, Config: Defaults(), Clips: []Clip{{ID: "clip-01", Start: 1, End: 20, Layout: "focus", CaptionStyle: "bold"}}}
	path, err := p.Save()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "Round" || len(loaded.Clips) != 1 {
		t.Fatalf("round trip lost data: %+v", loaded)
	}
}

func TestHelpers(t *testing.T) {
	if !IsClipID("clip-01") || IsClipID("a/b") || IsClipID("") {
		t.Fatal("clip id check")
	}
	if got := OutputFileName(Clip{ID: "clip-03", Title: "Why SEO Still Works!"}, "abcdef1234"); got != "clip-03-why-seo-still-works-abcdef.mp4" {
		t.Fatalf("file name %q", got)
	}
	if Tail("abcdefgh", 3) != "…fgh" || Head("abcdefgh", 3) != "abc…" {
		t.Fatal("tail/head")
	}
	if ExtractJSON("Sure! ```json\n{\"a\":1}\n``` done") != `{"a":1}` {
		t.Fatal("extract json")
	}
}

func TestCandidatesRespectRange(t *testing.T) {
	var cues []Cue
	for i := 0; i < 120; i++ {
		text := "People keep asking how to grow a podcast audience with search."
		if i%5 == 4 {
			text += " That is the whole strategy."
		}
		cues = append(cues, Cue{Start: float64(i * 2), End: float64(i*2 + 2), Text: text})
	}
	pool := Candidates(cues, 20, 40)
	if len(pool) == 0 {
		t.Fatal("no candidates")
	}
	for _, c := range pool {
		if d := c.End - c.Start; d < 20 || d > 40 {
			t.Fatalf("candidate outside range: %.1f", d)
		}
	}
	if err := ValidateSelection(0, 20, 40); err == nil {
		t.Fatal("count 0 must fail")
	}
	if err := ValidateSelection(3, 50, 20); err == nil {
		t.Fatal("max < min must fail")
	}
}

func TestRankUsesWordBoundaries(t *testing.T) {
	plain := rank("He said that again and certain people stayed. "+strings.Repeat("filler words here ", 10), 40)
	withAI := rank("AI changes everything for podcasters and creators. "+strings.Repeat("filler words here ", 10), 40)
	if withAI <= plain {
		t.Fatalf("expected the real keyword to score higher: %.1f vs %.1f", withAI, plain)
	}
}
