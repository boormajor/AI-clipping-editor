package studio

import "time"

type Cue struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}
type Clip struct {
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	Start        float64 `json:"start"`
	End          float64 `json:"end"`
	Score        float64 `json:"score"`
	Reason       string  `json:"reason"`
	Selected     bool    `json:"selected"`
	Pan          float64 `json:"pan"`    // 0 is left, 0.5 center, 1 right
	Layout       string  `json:"layout"` // focus or two-up; two-up assumes left/right source split
	Captions     bool    `json:"captions"`
	CaptionStyle string  `json:"captionStyle"`
	Callout      string  `json:"callout"`
	Broll        []Broll `json:"broll"`
}
type Broll struct {
	Path     string  `json:"path"`
	Start    float64 `json:"start"` // relative to clip
	Duration float64 `json:"duration"`
}
type Config struct {
	Preset        string  `json:"preset"`
	Captions      bool    `json:"captions"`
	Reframe       bool    `json:"reframe"`
	Zoom          bool    `json:"zoom"`
	Broll         bool    `json:"broll"`
	Stock         bool    `json:"stock"`
	Graphics      bool    `json:"graphics"`
	Transitions   bool    `json:"transitions"`
	Sfx           bool    `json:"sfx"`
	CaptionStyle  string  `json:"captionStyle"`
	CaptionFont   string  `json:"captionFont"`
	CaptionSize   int     `json:"captionSize"`
	CaptionOffset float64 `json:"captionOffset"` // seconds; shift transcripts to sync with soundtrack
	Width         int     `json:"width"`
	Height        int     `json:"height"`
	FPS           int     `json:"fps"`
	GPU           bool    `json:"gpu"`
	Instructions  string  `json:"instructions"`
}
type Project struct {
	Name           string    `json:"name"`
	VideoPath      string    `json:"videoPath"`
	TranscriptPath string    `json:"transcriptPath"`
	OutputDir      string    `json:"outputDir"`
	Duration       float64   `json:"duration"`
	Cues           []Cue     `json:"cues"`
	Clips          []Clip    `json:"clips"`
	Config         Config    `json:"config"`
	LastUpdated    time.Time `json:"lastUpdated"`
}
type ProviderConfig struct {
	Provider string `json:"provider"` // local, ollama, openai, anthropic
	Model    string `json:"model"`
	APIKey   string `json:"apiKey"` // transient, never persisted
	Endpoint string `json:"endpoint"`
}

func Defaults() Config {
	return Config{Preset: "simple", Captions: true, Reframe: true, CaptionStyle: "bold", CaptionFont: "Arial", CaptionSize: 76, Width: 1080, Height: 1920, FPS: 30, GPU: true}
}
