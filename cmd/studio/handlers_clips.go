package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/boormajor/podcast-ai-studio/internal/studio"
)

// ensureProviderAuth fills in the ChatGPT plan token (server-side only) and a
// default model for providers that need them.
func (a *application) ensureProviderAuth(ctx context.Context, p *studio.ProviderConfig) error {
	if p.Provider != "chatgpt" {
		return nil
	}
	token, err := a.auth.accessToken(ctx)
	if err != nil {
		return err
	}
	p.APIKey = token // never exposed to the browser
	if strings.TrimSpace(p.Model) == "" {
		models, err := a.auth.models(ctx)
		if err != nil {
			return errors.New("could not list ChatGPT models: " + err.Error())
		}
		if len(models) == 0 {
			return errors.New("no eligible ChatGPT models are available for this account")
		}
		p.Model = models[0]["slug"]
	}
	return nil
}

func (a *application) analyze(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Count    int                   `json:"count"`
		Min      float64               `json:"min"`
		Max      float64               `json:"max"`
		Provider studio.ProviderConfig `json:"provider"`
	}
	if err := decode(r, &q); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if err := studio.ValidateSelection(q.Count, q.Min, q.Max); err != nil {
		fail(w, 400, err.Error())
		return
	}
	p, rev := a.snapshot()
	if len(p.Cues) == 0 {
		fail(w, 400, "Import an episode first")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	if err := a.ensureProviderAuth(ctx, &q.Provider); err != nil {
		fail(w, 401, err.Error())
		return
	}
	clips, err := studio.SelectHighlights(ctx, p.Cues, q.Count, q.Min, q.Max, q.Provider, p.Config.Instructions)
	if err != nil {
		fail(w, 422, err.Error())
		return
	}
	for i := range clips {
		clips[i].Words = p.WordsWithin(clips[i].Start, clips[i].End)
		clips[i].CaptionStyle = p.Config.CaptionStyle
		clips[i].Captions = p.Config.Captions
	}
	if !a.update(rev, func(p *studio.Project) { p.Clips = clips }) {
		fail(w, 409, "The project changed while clips were being selected; run it again")
		return
	}
	reply(w, a.statusPayload())
}

// clipPatch lists the fields a client may change on a clip. Measured words,
// scores and AI shot plans are owned by the server.
type clipPatch struct {
	ID           string          `json:"id"`
	Title        *string         `json:"title,omitempty"`
	Start        *float64        `json:"start,omitempty"`
	End          *float64        `json:"end,omitempty"`
	Pan          *float64        `json:"pan,omitempty"`
	Layout       *string         `json:"layout,omitempty"`
	Captions     *bool           `json:"captions,omitempty"`
	CaptionStyle *string         `json:"captionStyle,omitempty"`
	Callout      *string         `json:"callout,omitempty"`
	Selected     *bool           `json:"selected,omitempty"`
	Broll        *[]studio.Broll `json:"broll,omitempty"`
	ClearShots   bool            `json:"clearShots,omitempty"`
}

func (a *application) updateClip(w http.ResponseWriter, r *http.Request) {
	var q clipPatch
	if err := decode(r, &q); err != nil {
		fail(w, 400, err.Error())
		return
	}
	p, rev := a.snapshot()
	c, ok := p.FindClip(q.ID)
	if !ok {
		fail(w, 404, "Clip does not exist")
		return
	}
	boundsChanged := false
	if q.Title != nil {
		c.Title = strings.TrimSpace(*q.Title)
	}
	if q.Start != nil && *q.Start != c.Start {
		c.Start = *q.Start
		boundsChanged = true
	}
	if q.End != nil && *q.End != c.End {
		c.End = *q.End
		boundsChanged = true
	}
	if q.Pan != nil {
		c.Pan = *q.Pan
	}
	if q.Layout != nil {
		c.Layout = *q.Layout
	}
	if q.Captions != nil {
		c.Captions = *q.Captions
	}
	if q.CaptionStyle != nil {
		c.CaptionStyle = *q.CaptionStyle
	}
	if q.Callout != nil {
		c.Callout = strings.TrimSpace(*q.Callout)
	}
	if q.Selected != nil {
		c.Selected = *q.Selected
	}
	if q.Broll != nil {
		c.Broll = *q.Broll
		for i := range c.Broll {
			path, err := studio.AbsRegularFile(c.Broll[i].Path)
			if err != nil {
				fail(w, 400, "B-roll: "+err.Error())
				return
			}
			c.Broll[i].Path = path
		}
	}
	if c.Broll == nil {
		c.Broll = []studio.Broll{}
	}
	if boundsChanged || q.ClearShots {
		// A shot plan and measured words are only meaningful for the bounds they
		// were computed for; re-derive words from the project and drop the plan.
		c.Shots = nil
		c.Words = p.WordsWithin(c.Start, c.End)
		c.Reviewed = false
		c.ReviewOK = false
		c.ReviewSummary = ""
	}
	if err := c.Validate(p.Duration); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if !a.update(rev, func(p *studio.Project) {
		for i := range p.Clips {
			if p.Clips[i].ID == c.ID {
				p.Clips[i] = c
			}
		}
	}) {
		fail(w, 409, "The project changed; reload and try again")
		return
	}
	reply(w, map[string]any{"clip": c})
}

// editTranscript replaces cue text (timings are kept) and optionally the
// recognised text of a clip's measured words.
func (a *application) editTranscript(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Cues   []studio.Cue  `json:"cues"`
		ClipID string        `json:"clipId"`
		Words  []studio.Word `json:"words"`
	}
	if err := decode(r, &q); err != nil {
		fail(w, 400, err.Error())
		return
	}
	cleaned := make([]studio.Cue, 0, len(q.Cues))
	for _, c := range q.Cues {
		if c.End > c.Start && c.Start >= 0 && strings.TrimSpace(c.Text) != "" {
			cleaned = append(cleaned, studio.Cue{Start: c.Start, End: c.End, Text: strings.TrimSpace(c.Text)})
		}
	}
	if len(q.Cues) > 0 && len(cleaned) == 0 {
		fail(w, 400, "No valid cues provided")
		return
	}
	a.update(0, func(p *studio.Project) {
		if len(cleaned) > 0 {
			p.Cues = cleaned
		}
		if q.ClipID != "" && len(q.Words) > 0 {
			for i := range p.Clips {
				if p.Clips[i].ID != q.ClipID {
					continue
				}
				words := p.Clips[i].Words
				for k := range q.Words {
					if k < len(words) {
						words[k].Word = strings.TrimSpace(q.Words[k].Word)
					}
				}
			}
		}
	})
	reply(w, a.statusPayload())
}

func (a *application) cleanClip(w http.ResponseWriter, r *http.Request) {
	var q struct {
		ID         string  `json:"id"`
		SilenceGap float64 `json:"silenceGap"`
	}
	if err := decode(r, &q); err != nil {
		fail(w, 400, err.Error())
		return
	}
	p, _ := a.snapshot()
	c, ok := p.FindClip(q.ID)
	if !ok {
		fail(w, 404, "Clip not found")
		return
	}
	if len(c.Words) == 0 {
		fail(w, 422, "Dead-air cleanup needs word timings. Import the episode with cloud transcription to get them.")
		return
	}
	report := studio.AnalyzeClean(c.Words, c.End-c.Start, q.SilenceGap)
	reply(w, map[string]any{"report": report, "suggestedStart": c.Start + report.TrimStart, "suggestedEnd": c.End - report.TrimEnd})
}

func (a *application) generateMetadata(w http.ResponseWriter, r *http.Request) {
	var q struct {
		ID       string                `json:"id"`
		Platform string                `json:"platform"`
		Provider studio.ProviderConfig `json:"provider"`
	}
	if err := decode(r, &q); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if !q.Provider.IsOnline() {
		fail(w, 422, "Select an online AI provider (ChatGPT, Claude or an API key) to write metadata")
		return
	}
	p, _ := a.snapshot()
	c, ok := p.FindClip(q.ID)
	if !ok {
		fail(w, 404, "Clip not found")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	if err := a.ensureProviderAuth(ctx, &q.Provider); err != nil {
		fail(w, 401, err.Error())
		return
	}
	platform := strings.ToLower(strings.TrimSpace(q.Platform))
	if platform == "" {
		platform = "youtube"
	}
	prompt := "You write high-performing, HONEST metadata for a short vertical video clip for " + platform + ". " +
		"Do not clickbait, fabricate claims, or invent facts not supported by the transcript. " +
		"Write a punchy title (<=100 chars), a 2-3 sentence description, 5-8 relevant hashtags (no spaces, no '#'), " +
		"and 6-12 lowercase search tags. Output ONLY JSON: " +
		`{"title":"...","description":"...","hashtags":["..."],"tags":["..."]}` + "\nCLIP TITLE: " + c.Title + "\nTRANSCRIPT: " + p.TranscriptBetween(c.Start, c.End)
	raw, err := studio.AskModel(ctx, q.Provider, prompt)
	if err != nil {
		fail(w, 422, err.Error())
		return
	}
	var meta struct {
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Hashtags    []string `json:"hashtags"`
		Tags        []string `json:"tags"`
	}
	if err := json.Unmarshal([]byte(studio.ExtractJSON(raw)), &meta); err != nil {
		fail(w, 422, "AI returned invalid metadata JSON: "+err.Error())
		return
	}
	reply(w, meta)
}

func (a *application) render(w http.ResponseWriter, r *http.Request) {
	var q struct {
		ID string `json:"id"`
	}
	if err := decode(r, &q); err != nil {
		fail(w, 400, err.Error())
		return
	}
	p, _ := a.snapshot()
	c, ok := p.FindClip(q.ID)
	if !ok {
		fail(w, 404, "Clip not found")
		return
	}
	if err := c.Validate(p.Duration); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if !a.tools.get()["ffmpeg"] {
		fail(w, 400, "FFmpeg is required to render. Run Install-and-Run.bat.")
		return
	}
	if a.jobs.activeFor(jobRender, c.ID) {
		fail(w, 409, "This clip is already rendering")
		return
	}
	j := &job{ClipID: c.ID, Title: c.Title}
	a.jobs.start(jobRender, j, studio.RenderTimeout, func(ctx context.Context, report func(int, string)) (string, string, error) {
		progress := func(pct int) { report(pct, "Rendering with Remotion") }
		out, log, err := studio.RenderRemotion(ctx, p, c, j.ID, progress)
		if err == nil {
			a.jobs.setRenderer(j, "remotion")
			return out, log, nil
		}
		if ctx.Err() != nil {
			return "", log, err
		}
		a.log.Warn("Remotion render failed; using FFmpeg fallback", "clip", c.ID, "error", err)
		report(10, "Remotion unavailable; rendering with FFmpeg")
		fbOut, fbLog, fbErr := studio.Render(ctx, p, c, j.ID)
		combined := "[remotion] " + studio.Tail(log, 1200) + "\n[ffmpeg] " + studio.Tail(fbLog, 1200)
		if fbErr != nil {
			return "", combined, errors.New(err.Error() + "; FFmpeg fallback also failed: " + fbErr.Error())
		}
		a.jobs.setRenderer(j, "ffmpeg-fallback")
		return fbOut, combined, nil
	})
	reply(w, map[string]any{"job": j.ID, "status": statusQueued})
}

func (m *jobManager) setRenderer(j *job, renderer string) {
	m.mu.Lock()
	j.Renderer = renderer
	m.mu.Unlock()
}
