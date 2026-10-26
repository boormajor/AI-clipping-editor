package studio

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MaxProjectFileBytes caps project files read from disk.
const MaxProjectFileBytes = 64 * 1024 * 1024

// Clone returns a deep copy so callers can read or encode a snapshot without
// holding the application lock while other requests mutate the original.
func (p Project) Clone() Project {
	out := p
	out.Cues = append(make([]Cue, 0, len(p.Cues)), p.Cues...)
	out.Words = append([]Word(nil), p.Words...)
	out.Clips = make([]Clip, len(p.Clips))
	for i, c := range p.Clips {
		out.Clips[i] = c.Clone()
	}
	return out
}

// Clone deep-copies a clip.
func (c Clip) Clone() Clip {
	out := c
	out.Broll = append(make([]Broll, 0, len(c.Broll)), c.Broll...)
	out.Words = append([]Word(nil), c.Words...)
	out.Shots = append([]Shot(nil), c.Shots...)
	return out
}

// Validate checks a project loaded from disk or supplied by a client. It
// normalises the configuration in place and rejects anything that could not
// have been produced by this application.
func (p *Project) Validate() error {
	if strings.TrimSpace(p.VideoPath) == "" {
		return errors.New("project has no source video")
	}
	abs, err := AbsRegularFile(p.VideoPath)
	if err != nil {
		return fmt.Errorf("source video: %w", err)
	}
	p.VideoPath = abs
	if p.TranscriptPath != "" {
		if t, err := filepath.Abs(p.TranscriptPath); err == nil {
			p.TranscriptPath = t
		}
	}
	if p.OutputDir != "" {
		if !filepath.IsAbs(p.OutputDir) {
			return errors.New("export folder must be an absolute path")
		}
		p.OutputDir = filepath.Clean(p.OutputDir)
	}
	if p.Duration <= 0 || p.Duration > 48*3600 {
		return errors.New("project duration is invalid")
	}
	if len(p.Cues) > 200000 || len(p.Words) > 2000000 || len(p.Clips) > 200 {
		return errors.New("project is unreasonably large")
	}
	p.Config.Normalize()
	if err := p.Config.Validate(); err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	seen := map[string]bool{}
	for i := range p.Clips {
		c := &p.Clips[i]
		if !IsClipID(c.ID) {
			return fmt.Errorf("clip %d has an invalid id", i+1)
		}
		if seen[c.ID] {
			return fmt.Errorf("duplicate clip id %s", c.ID)
		}
		seen[c.ID] = true
		if c.Layout == "" {
			c.Layout = "focus"
		}
		if c.CaptionStyle == "" {
			c.CaptionStyle = p.Config.CaptionStyle
		}
		if err := c.Validate(p.Duration); err != nil {
			return fmt.Errorf("%s: %w", c.ID, err)
		}
		dur := c.End - c.Start
		for _, w := range c.Words {
			if w.Start < -0.01 || w.End > dur+0.5 || w.End < w.Start {
				return fmt.Errorf("%s has word timings outside the clip", c.ID)
			}
		}
		for j := range c.Broll {
			if !filepath.IsAbs(c.Broll[j].Path) {
				return fmt.Errorf("%s: B-roll paths must be absolute", c.ID)
			}
		}
	}
	for _, cue := range p.Cues {
		if cue.Start < 0 || cue.End < cue.Start {
			return errors.New("transcript contains invalid timestamps")
		}
	}
	if p.Name == "" {
		p.Name = strings.TrimSuffix(filepath.Base(p.VideoPath), filepath.Ext(p.VideoPath))
	}
	return nil
}

// ProjectFileName is the name saved projects use inside the export folder.
const ProjectFileName = "podcast-ai-studio.project.json"

// Save writes the project to its export folder and returns the file path.
// Source media and credentials are never embedded.
func (p Project) Save() (string, error) {
	if p.VideoPath == "" {
		return "", errors.New("import an episode first")
	}
	dir, err := AbsDir(p.ExportDir())
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, ProjectFileName)
	if err := WriteJSONAtomic(path, p, true); err != nil {
		return "", err
	}
	return path, nil
}

// Load reads and validates a project file.
func Load(path string) (Project, error) {
	var p Project
	if filepath.Ext(path) != ".json" {
		return p, errors.New("project files end in .json")
	}
	if err := ReadJSONLimited(path, MaxProjectFileBytes, &p); err != nil {
		return p, fmt.Errorf("could not read project: %w", err)
	}
	if err := p.Validate(); err != nil {
		return p, err
	}
	if _, err := os.Stat(p.VideoPath); err != nil {
		return p, fmt.Errorf("source video is missing: %s", p.VideoPath)
	}
	return p, nil
}
