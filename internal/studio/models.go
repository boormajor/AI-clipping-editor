// Package studio holds the editing domain: transcripts, clip selection,
// cloud speech, the Remotion bridge and the FFmpeg fallback renderer. It has
// no HTTP dependencies; cmd/studio wires it to the local API.
package studio

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Cue is a timed transcript line (absolute seconds in the source video).
type Cue struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

// Word is a measured word timestamp. Project-level words are absolute; words
// stored on a Clip are relative to the clip start.
type Word struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Word  string  `json:"word"`
}

// Shot is one framing segment of an AI shot plan, relative to the clip start.
type Shot struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Mode  string  `json:"mode"` // both, left, right or full
	Pan   float64 `json:"pan,omitempty"`
}

// Clip is a candidate short-form excerpt and its edit decisions.
type Clip struct {
	ID               string  `json:"id"`
	Title            string  `json:"title"`
	Start            float64 `json:"start"`
	End              float64 `json:"end"`
	Score            float64 `json:"score"`
	Reason           string  `json:"reason"`
	Selected         bool    `json:"selected"`
	Pan              float64 `json:"pan"`    // 0 is left, 0.5 centre, 1 right
	Layout           string  `json:"layout"` // focus, two-up or dynamic
	Captions         bool    `json:"captions"`
	CaptionStyle     string  `json:"captionStyle"`
	Callout          string  `json:"callout"`
	Broll            []Broll `json:"broll"`
	Words            []Word  `json:"words,omitempty"`
	Shots            []Shot  `json:"shots,omitempty"`
	CaptionAgreement float64 `json:"captionAgreement,omitempty"`
	Reviewed         bool    `json:"reviewed,omitempty"`
	ReviewOK         bool    `json:"reviewOK,omitempty"`
	ReviewSummary    string  `json:"reviewSummary,omitempty"`
}

// Broll is an overlay asset placed on a clip's timeline.
type Broll struct {
	Path     string  `json:"path"`
	Start    float64 `json:"start"` // relative to clip
	Duration float64 `json:"duration"`
	Mode     string  `json:"mode,omitempty"` // cover (default) or pip
}

// Config holds project-wide editing and export preferences.
type Config struct {
	Preset          string  `json:"preset"`
	Captions        bool    `json:"captions"`
	Reframe         bool    `json:"reframe"`
	Zoom            bool    `json:"zoom"`
	Broll           bool    `json:"broll"`
	Stock           bool    `json:"stock"`
	Graphics        bool    `json:"graphics"`
	Transitions     bool    `json:"transitions"`
	Sfx             bool    `json:"sfx"`
	ProgressBar     bool    `json:"progressBar"`
	CaptionStyle    string  `json:"captionStyle"`
	CaptionEmphasis bool    `json:"captionEmphasis"`
	CaptionFont     string  `json:"captionFont"`
	CaptionSize     int     `json:"captionSize"`
	CaptionColor    string  `json:"captionColor,omitempty"`
	AccentColor     string  `json:"accentColor,omitempty"`
	CaptionOffset   float64 `json:"captionOffset"`   // seconds; positive delays captions
	CaptionWords    int     `json:"captionWords"`    // maximum words per caption group
	CaptionPosition float64 `json:"captionPosition"` // 0.1..0.95 of frame height
	Width           int     `json:"width"`
	Height          int     `json:"height"`
	FPS             int     `json:"fps"`
	GPU             bool    `json:"gpu"`
	Instructions    string  `json:"instructions"`
}

// Project is the unit of work: one source video and its derived clips.
type Project struct {
	Name           string    `json:"name"`
	VideoPath      string    `json:"videoPath"`
	TranscriptPath string    `json:"transcriptPath"`
	OutputDir      string    `json:"outputDir"`
	Duration       float64   `json:"duration"`
	Cues           []Cue     `json:"cues"`
	Words          []Word    `json:"words,omitempty"`
	Clips          []Clip    `json:"clips"`
	Config         Config    `json:"config"`
	LastUpdated    time.Time `json:"lastUpdated"`
}

// ProviderConfig selects the editorial AI provider for one request.
type ProviderConfig struct {
	Provider string `json:"provider"` // heuristic, chatgpt, claude-code, openai, anthropic
	Model    string `json:"model"`
	APIKey   string `json:"apiKey"` // transient, never persisted
	Endpoint string `json:"endpoint"`
}

// IsOnline reports whether the provider calls a hosted model.
func (p ProviderConfig) IsOnline() bool {
	switch strings.ToLower(strings.TrimSpace(p.Provider)) {
	case "", "local", "heuristic":
		return false
	}
	return true
}

// CaptionPresets lists every caption template the renderer understands. The
// Remotion schema (remotion/src/schema.ts) must list the same names.
var CaptionPresets = []string{"kinetic", "editorial", "bold", "minimal", "karaoke", "boxed", "podcast", "yellow", "highlight", "popline", "wordbox", "wipe", "slide", "bounce", "typewriter"}

// CaptionFonts lists the fonts the renderer can load (see remotion/src/fonts.ts).
var CaptionFonts = []string{"Inter", "Montserrat", "Poppins", "Roboto", "Archivo", "Oswald", "Anton", "Bebas Neue", "Arial", "Verdana", "Georgia"}

// Layouts lists the supported frame layouts.
var Layouts = []string{"focus", "two-up", "dynamic"}

// ValidFPS lists the export frame rates the encoders are tuned for.
var ValidFPS = []int{24, 25, 30, 60}

// Defaults returns the configuration a new project starts with.
func Defaults() Config {
	return Config{Preset: "simple", Captions: true, Reframe: true, CaptionStyle: "kinetic", CaptionFont: "Inter", CaptionSize: 76, CaptionWords: 3, CaptionPosition: 0.82, CaptionColor: "#FFFFFF", AccentColor: "#FACC15", Width: 1080, Height: 1920, FPS: 30, GPU: true}
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// IsCaptionPreset reports whether s names a known caption template.
func IsCaptionPreset(s string) bool { return contains(CaptionPresets, s) }

// IsLayout reports whether s names a known frame layout.
func IsLayout(s string) bool { return contains(Layouts, s) }

// IsHexColor reports whether s is a #RRGGBB colour.
func IsHexColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, r := range s[1:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}

// Normalize fills missing fields with defaults so older project files and
// partial API payloads stay usable.
func (c *Config) Normalize() {
	d := Defaults()
	if c.Preset == "" {
		c.Preset = d.Preset
	}
	if !IsCaptionPreset(c.CaptionStyle) {
		c.CaptionStyle = d.CaptionStyle
	}
	if !contains(CaptionFonts, c.CaptionFont) {
		c.CaptionFont = d.CaptionFont
	}
	if c.CaptionSize == 0 {
		c.CaptionSize = d.CaptionSize
	}
	if c.CaptionWords == 0 {
		c.CaptionWords = d.CaptionWords
	}
	if c.CaptionPosition == 0 {
		c.CaptionPosition = d.CaptionPosition
	}
	if !IsHexColor(c.CaptionColor) {
		c.CaptionColor = d.CaptionColor
	}
	if !IsHexColor(c.AccentColor) {
		c.AccentColor = d.AccentColor
	}
	if c.Width == 0 || c.Height == 0 {
		c.Width, c.Height = d.Width, d.Height
	}
	if c.FPS == 0 {
		c.FPS = d.FPS
	}
}

// Validate returns the first problem with a configuration.
func (c Config) Validate() error {
	if c.Width < 240 || c.Height < 240 || c.Width > 3840 || c.Height > 3840 {
		return errors.New("resolution must be between 240 and 3840 pixels on each side")
	}
	if c.Width%2 != 0 || c.Height%2 != 0 {
		return errors.New("width and height must be even numbers for H.264 encoding")
	}
	valid := false
	for _, f := range ValidFPS {
		if c.FPS == f {
			valid = true
		}
	}
	if !valid {
		return errors.New("frame rate must be 24, 25, 30 or 60")
	}
	if c.CaptionSize < 24 || c.CaptionSize > 150 {
		return errors.New("caption size must be between 24 and 150")
	}
	if c.CaptionWords < 1 || c.CaptionWords > 12 {
		return errors.New("words on screen must be between 1 and 12")
	}
	if c.CaptionOffset < -10 || c.CaptionOffset > 10 {
		return errors.New("caption offset must be between -10 and 10 seconds")
	}
	if c.CaptionPosition < 0.1 || c.CaptionPosition > 0.95 {
		return errors.New("caption position must be between 0.1 and 0.95")
	}
	if !IsCaptionPreset(c.CaptionStyle) {
		return fmt.Errorf("unknown caption template %q", c.CaptionStyle)
	}
	if !contains(CaptionFonts, c.CaptionFont) {
		return fmt.Errorf("unknown caption font %q", c.CaptionFont)
	}
	if !IsHexColor(c.CaptionColor) || !IsHexColor(c.AccentColor) {
		return errors.New("colours must be #RRGGBB")
	}
	if len(c.Instructions) > 4000 {
		return errors.New("AI instructions must be under 4000 characters")
	}
	return nil
}

// Validate checks a clip's edit decisions against the project duration.
func (c Clip) Validate(duration float64) error {
	if !IsClipID(c.ID) {
		return errors.New("clip id must be letters, digits, dashes or underscores")
	}
	if c.Start < 0 || c.End <= c.Start || c.End > duration+0.5 {
		return errors.New("clip start and end must lie within the episode and end after start")
	}
	if c.End-c.Start < 1 {
		return errors.New("clips must be at least one second long")
	}
	if c.Pan < 0 || c.Pan > 1 {
		return errors.New("pan must be between 0 and 1")
	}
	if c.Layout != "" && !IsLayout(c.Layout) {
		return fmt.Errorf("unknown layout %q", c.Layout)
	}
	if c.CaptionStyle != "" && !IsCaptionPreset(c.CaptionStyle) {
		return fmt.Errorf("unknown caption template %q", c.CaptionStyle)
	}
	if len([]rune(c.Title)) > 160 {
		return errors.New("title must be under 160 characters")
	}
	if len([]rune(c.Callout)) > 120 {
		return errors.New("callout must be under 120 characters")
	}
	if len(c.Broll) > 8 {
		return errors.New("at most 8 B-roll overlays per clip")
	}
	dur := c.End - c.Start
	for _, b := range c.Broll {
		if strings.TrimSpace(b.Path) == "" {
			return errors.New("B-roll overlay needs a file path")
		}
		if b.Start < 0 || b.Duration <= 0 || b.Start > dur {
			return errors.New("B-roll overlay must start inside the clip with a positive duration")
		}
		if b.Mode != "" && b.Mode != "cover" && b.Mode != "pip" {
			return fmt.Errorf("unknown B-roll mode %q", b.Mode)
		}
	}
	if len(c.Shots) > 12 {
		return errors.New("at most 12 shots per clip")
	}
	for _, s := range c.Shots {
		if s.Start < 0 || s.End <= s.Start || s.End > dur+0.15 {
			return errors.New("shot ranges must lie inside the clip")
		}
		if s.Mode != "both" && s.Mode != "left" && s.Mode != "right" && s.Mode != "full" {
			return fmt.Errorf("unknown shot mode %q", s.Mode)
		}
	}
	return nil
}

// FindClip returns the clip with the given id.
func (p Project) FindClip(id string) (Clip, bool) {
	for _, c := range p.Clips {
		if c.ID == id {
			return c, true
		}
	}
	return Clip{}, false
}

// WordsWithin returns the project's measured words inside [start,end),
// re-based so they are relative to start.
func (p Project) WordsWithin(start, end float64) []Word {
	if len(p.Words) == 0 {
		return nil
	}
	var out []Word
	for _, w := range p.Words {
		if w.Start >= start && w.End <= end {
			out = append(out, Word{Start: w.Start - start, End: w.End - start, Word: w.Word})
		}
	}
	return out
}

// TranscriptBetween joins cue text overlapping [start,end).
func (p Project) TranscriptBetween(start, end float64) string {
	var b strings.Builder
	for _, cue := range p.Cues {
		if cue.Start < end && cue.End > start {
			b.WriteString(cue.Text)
			b.WriteByte(' ')
		}
	}
	return strings.TrimSpace(b.String())
}
