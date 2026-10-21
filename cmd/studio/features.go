package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/boormajor/podcast-ai-studio/internal/studio"
)

// ---------- Persistence: survive an app restart ----------

func (a *application) projectFile() string { return filepath.Join(a.stateDir, "last-project.json") }
func (a *application) jobsFile() string    { return filepath.Join(a.stateDir, "jobs.json") }

// loadState restores the last project and render queue at startup. Single
// threaded (called before the server starts), so it takes no lock.
func (a *application) loadState() {
	if a.stateDir == "" {
		return
	}
	if b, e := os.ReadFile(a.projectFile()); e == nil {
		var p studio.Project
		if json.Unmarshal(b, &p) == nil && p.VideoPath != "" {
			if _, e := os.Stat(p.VideoPath); e == nil {
				a.project = p
			}
		}
	}
	if b, e := os.ReadFile(a.jobsFile()); e == nil {
		var list []*job
		if json.Unmarshal(b, &list) == nil {
			for _, j := range list {
				if j == nil || j.ID == "" {
					continue
				}
				if j.Status == "rendering" || j.Status == "queued" {
					j.Status = "failed"
					j.Error = "Interrupted by app restart"
				}
				a.jobs[j.ID] = j
			}
		}
	}
}

// persistProjectLocked writes the current project to disk. Call with a.mu held.
func (a *application) persistProjectLocked() {
	if a.stateDir == "" || a.project.VideoPath == "" {
		return
	}
	if b, e := json.MarshalIndent(a.project, "", "  "); e == nil {
		_ = os.WriteFile(a.projectFile(), b, 0600)
	}
}

// persistJobsLocked writes the render queue to disk. Call with a.mu held.
func (a *application) persistJobsLocked() {
	if a.stateDir == "" {
		return
	}
	list := make([]*job, 0, len(a.jobs))
	for _, j := range a.jobs {
		list = append(list, j)
	}
	if b, e := json.Marshal(list); e == nil {
		_ = os.WriteFile(a.jobsFile(), b, 0600)
	}
}

// ---------- Provider auth helper (shared by metadata) ----------

func (a *application) ensureProviderAuth(ctx context.Context, p *studio.ProviderConfig) error {
	if p.Provider != "chatgpt" {
		return nil
	}
	token, err := a.auth.accessToken(ctx)
	if err != nil {
		return err
	}
	p.APIKey = token
	if strings.TrimSpace(p.Model) == "" {
		models, err := a.auth.models(ctx)
		if err != nil || len(models) == 0 {
			return err
		}
		p.Model = models[0]["slug"]
	}
	return nil
}

// ---------- Transcript / caption text editing ----------

// editTranscript updates the project transcript cues and/or a clip's aligned
// word text (timings are preserved; only the recognized text changes).
func (a *application) editTranscript(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Cues   []studio.Cue  `json:"cues"`
		ClipID string        `json:"clipId"`
		Words  []studio.Word `json:"words"`
	}
	if e := decode(r, &q); e != nil {
		fail(w, 400, e.Error())
		return
	}
	a.mu.Lock()
	if len(q.Cues) > 0 {
		cleaned := make([]studio.Cue, 0, len(q.Cues))
		for _, c := range q.Cues {
			if c.End > c.Start && c.Start >= 0 && strings.TrimSpace(c.Text) != "" {
				cleaned = append(cleaned, studio.Cue{Start: c.Start, End: c.End, Text: strings.TrimSpace(c.Text)})
			}
		}
		if len(cleaned) == 0 {
			a.mu.Unlock()
			fail(w, 400, "No valid cues provided")
			return
		}
		a.project.Cues = cleaned
	}
	if q.ClipID != "" && len(q.Words) > 0 {
		for i := range a.project.Clips {
			if a.project.Clips[i].ID == q.ClipID {
				words := a.project.Clips[i].Words
				for k := range q.Words {
					if k < len(words) {
						words[k].Word = strings.TrimSpace(q.Words[k].Word)
					}
				}
				break
			}
		}
	}
	a.project.LastUpdated = time.Now()
	a.persistProjectLocked()
	a.mu.Unlock()
	a.status(w, r)
}

// ---------- Filler-word & silence analysis ----------

func (a *application) cleanClip(w http.ResponseWriter, r *http.Request) {
	var q struct {
		ID         string  `json:"id"`
		SilenceGap float64 `json:"silenceGap"`
	}
	if e := decode(r, &q); e != nil {
		fail(w, 400, e.Error())
		return
	}
	a.mu.Lock()
	var c studio.Clip
	found := false
	for _, v := range a.project.Clips {
		if v.ID == q.ID {
			c = v
			found = true
			break
		}
	}
	a.mu.Unlock()
	if !found {
		fail(w, 404, "Clip not found")
		return
	}
	if len(c.Words) == 0 {
		fail(w, 422, "Run 'Sync captions to audio' first so word timings are available for cleanup")
		return
	}
	report := studio.AnalyzeClean(c.Words, c.End-c.Start, q.SilenceGap)
	// Translate relative trim offsets into absolute suggested clip bounds.
	reply(w, map[string]any{
		"report":         report,
		"suggestedStart": c.Start + report.TrimStart,
		"suggestedEnd":   c.End - report.TrimEnd,
	})
}

// ---------- YouTube / social metadata ----------

func (a *application) generateMetadata(w http.ResponseWriter, r *http.Request) {
	var q struct {
		ID       string                `json:"id"`
		Platform string                `json:"platform"`
		Provider studio.ProviderConfig `json:"provider"`
	}
	if e := decode(r, &q); e != nil {
		fail(w, 400, e.Error())
		return
	}
	if q.Provider.Provider == "local" || q.Provider.Provider == "heuristic" || q.Provider.Provider == "" {
		fail(w, 422, "Select an online AI provider (ChatGPT, Claude or an API key) to write metadata")
		return
	}
	a.mu.Lock()
	p := a.project
	a.mu.Unlock()
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
		fail(w, 404, "Clip not found")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	if err := a.ensureProviderAuth(ctx, &q.Provider); err != nil {
		fail(w, 401, err.Error())
		return
	}
	var transcript strings.Builder
	for _, cue := range p.Cues {
		if cue.Start < c.End && cue.End > c.Start {
			transcript.WriteString(cue.Text)
			transcript.WriteByte(' ')
		}
	}
	platform := strings.ToLower(strings.TrimSpace(q.Platform))
	if platform == "" {
		platform = "youtube"
	}
	prompt := "You write high-performing, HONEST metadata for a short vertical video clip for " + platform + ". " +
		"Do not clickbait, fabricate claims, or invent facts not supported by the transcript. " +
		"Write a punchy title (<=100 chars), a 2-3 sentence description, 5-8 relevant hashtags (no spaces, no '#'), " +
		"and 6-12 lowercase search tags. Output ONLY JSON: " +
		`{"title":"...","description":"...","hashtags":["..."],"tags":["..."]}` + "\nCLIP TITLE: " + c.Title + "\nTRANSCRIPT: " + transcript.String()
	raw, err := studio.AskModel(ctx, q.Provider, prompt)
	if err != nil {
		fail(w, 422, err.Error())
		return
	}
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	var meta struct {
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Hashtags    []string `json:"hashtags"`
		Tags        []string `json:"tags"`
	}
	if e := json.Unmarshal([]byte(strings.TrimSpace(raw)), &meta); e != nil {
		fail(w, 422, "AI returned invalid metadata JSON: "+e.Error())
		return
	}
	reply(w, map[string]any{"title": meta.Title, "description": meta.Description, "hashtags": meta.Hashtags, "tags": meta.Tags})
}
