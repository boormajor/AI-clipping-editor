package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/boormajor/podcast-ai-studio/internal/studio"
)

// reviewClip asks the editorial model for judgment on one clip: whether it
// stands alone for a new viewer, a better title and a safe framing plan. It
// never claims to have watched the video; the optional local face/scene
// detectors only add metadata to the prompt.
func (a *application) reviewClip(w http.ResponseWriter, r *http.Request) {
	var q struct {
		ID       string                `json:"id"`
		Provider studio.ProviderConfig `json:"provider"`
	}
	if err := decode(r, &q); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if !q.Provider.IsOnline() {
		fail(w, 422, "Select ChatGPT, Claude or an API provider for editorial review")
		return
	}
	p, rev := a.snapshot()
	c, ok := p.FindClip(q.ID)
	if !ok {
		fail(w, 404, "Clip not found")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()
	if err := a.ensureProviderAuth(ctx, &q.Provider); err != nil {
		fail(w, 401, err.Error())
		return
	}
	before := p.TranscriptBetween(c.Start-18, c.Start)
	core := p.TranscriptBetween(c.Start, c.End)
	after := p.TranscriptBetween(c.End, c.End+14)

	faceInfo, sceneInfo := a.localAudits(ctx, p, c)
	alignment := "Word timings unavailable; caption accuracy has not been verified."
	if len(c.Words) > 0 {
		alignment = fmt.Sprintf("%d measured words available from cloud transcription. Check proper nouns manually.", len(c.Words))
	}
	prompt := `You are the senior video editor for a professional interview podcast. Your viewer has ZERO prior context. Improve the short clip honestly and conservatively. Review the text before, during, and after the suggested clip, and note missing antecedents or jargon. Rewrite the title to introduce the real topic. You are also given automated face-box metadata if available; this is NOT the images and can be wrong. Choose a safe layout: "two-up" if both speakers consistently appear side-by-side, "focus" when one speaker occupies the frame, "dynamic" only if an actual shot plan is warranted. Never invent camera evidence, facts, or speaker identity. NEVER create word timestamps. Return ONLY JSON with this exact structure: {"title":"A specific useful title","contextOK":true,"issues":["..."],"layout":"two-up","shots":[{"start":0,"end":10,"mode":"both"}],"summary":"Why it will make sense to a new viewer"}. Shot times are SECONDS RELATIVE TO THE CLIP, must cover all time, and mode is one of both|left|right. Prefer to show BOTH when unsure. Respond with at most 12 shots.
` +
		"PRECEDING (18s): " + before + "\nCLIP: " + core + "\nFOLLOWING (14s): " + after + "\nCLIP LENGTH: " + fmt.Sprintf("%.2f", c.End-c.Start) + "\nFRAME OBSERVATIONS: " + faceInfo + "\nSCENE CUT OBSERVATIONS: " + sceneInfo + "\nAUDIO: " + alignment + "\nUSER EDITING INSTRUCTIONS: " + p.Config.Instructions
	raw, err := studio.AskModel(ctx, q.Provider, prompt)
	if err != nil {
		fail(w, 422, err.Error())
		return
	}
	var v struct {
		Title     string        `json:"title"`
		ContextOK bool          `json:"contextOK"`
		Issues    []string      `json:"issues"`
		Layout    string        `json:"layout"`
		Shots     []studio.Shot `json:"shots"`
		Summary   string        `json:"summary"`
	}
	if err = json.Unmarshal([]byte(studio.ExtractJSON(raw)), &v); err != nil {
		fail(w, 422, "AI returned an invalid review format: "+err.Error())
		return
	}
	v.Title = studio.Head(strings.TrimSpace(v.Title), 100)
	if v.Title == "" {
		v.Title = c.Title
	}
	if len(v.Issues) > 10 {
		v.Issues = v.Issues[:10]
	}
	if !studio.IsLayout(v.Layout) {
		v.Layout = "two-up"
	}
	v.Shots = normalizeShots(v.Shots, c.End-c.Start)
	if v.Layout != "dynamic" {
		v.Shots = nil
	}
	v.Summary = studio.Head(strings.TrimSpace(v.Summary), 600)
	committed := a.update(rev, func(p *studio.Project) {
		for i := range p.Clips {
			if p.Clips[i].ID == q.ID {
				p.Clips[i].Title = v.Title
				p.Clips[i].Layout = v.Layout
				p.Clips[i].Shots = v.Shots
				p.Clips[i].Reviewed = true
				p.Clips[i].ReviewOK = v.ContextOK
				p.Clips[i].ReviewSummary = v.Summary
			}
		}
	})
	if !committed {
		fail(w, 409, "The project changed during the review; run it again")
		return
	}
	reply(w, map[string]any{"title": v.Title, "contextOK": v.ContextOK, "issues": v.Issues, "layout": v.Layout, "shots": v.Shots, "summary": v.Summary, "faceData": faceInfo != faceUnavailable})
}

// normalizeShots keeps a shot plan only when it is contiguous and covers the
// clip; anything else is discarded so the renderer never gets a broken plan.
func normalizeShots(shots []studio.Shot, dur float64) []studio.Shot {
	if len(shots) == 0 || len(shots) > 12 {
		return nil
	}
	prev := 0.0
	out := make([]studio.Shot, 0, len(shots))
	for _, s := range shots {
		if s.Mode != "both" && s.Mode != "left" && s.Mode != "right" {
			return nil
		}
		if s.Start < prev-.05 || s.Start > prev+.05 || s.End <= s.Start {
			return nil
		}
		if s.End > dur {
			s.End = dur
		}
		s.Start = prev
		out = append(out, s)
		prev = s.End
	}
	if prev < dur-.15 {
		return nil
	}
	return out
}

const faceUnavailable = "Not available (face tracking tool not installed)."

// localAudits runs the optional Python detectors on one downscaled extract of
// the clip. Failures degrade to "unavailable" text; they never block review.
func (a *application) localAudits(ctx context.Context, p studio.Project, c studio.Clip) (faceInfo, sceneInfo string) {
	faceInfo = faceUnavailable
	sceneInfo = "Scene-cut detection unavailable."
	faceScript, faceErr := findScript("face_audit.py")
	sceneScript, sceneErr := findScript("scene_audit.py")
	if faceErr != nil && sceneErr != nil {
		return
	}
	python := qualityPython()
	if !studio.HasTool(python) {
		return
	}
	temp, err := os.MkdirTemp("", "podcast-audit-")
	if err != nil {
		return
	}
	defer os.RemoveAll(temp)
	clipFile := filepath.Join(temp, "clip.mp4")
	if _, err := studio.DefaultTool.Run(ctx, "ffmpeg", "-hide_banner", "-nostdin", "-loglevel", "error", "-y",
		"-ss", fmt.Sprintf("%.3f", c.Start), "-t", fmt.Sprintf("%.3f", c.End-c.Start), "-i", p.VideoPath,
		"-vf", "scale=640:-2", "-an", clipFile); err != nil {
		a.log.Debug("audit extract failed", "error", err)
		return
	}
	run := func(script, out string) (string, bool) {
		if _, err := studio.DefaultTool.Run(ctx, python, script, "--video", clipFile, "--out", out); err != nil {
			a.log.Debug("audit script failed", "script", filepath.Base(script), "error", err)
		}
		b, err := os.ReadFile(out)
		if err != nil || strings.Contains(string(b), `"error"`) {
			return "", false
		}
		return studio.Head(string(b), 6000), true
	}
	if faceErr == nil {
		if s, ok := run(faceScript, filepath.Join(temp, "faces.json")); ok {
			faceInfo = s
		}
	}
	if sceneErr == nil {
		if s, ok := run(sceneScript, filepath.Join(temp, "cuts.json")); ok {
			sceneInfo = s
		}
	}
	return
}

// findScript locates an optional helper script in the bundle's scripts folder.
func findScript(name string) (string, error) {
	root, err := studio.FindRemotion()
	if err != nil {
		return "", err
	}
	file := filepath.Join(filepath.Dir(root), "scripts", name)
	if _, err := os.Stat(file); err != nil {
		return "", err
	}
	return file, nil
}

// qualityPython prefers the optional virtual environment shipped next to the
// bundle and falls back to the system interpreter.
func qualityPython() string {
	if root, err := studio.FindRemotion(); err == nil {
		parent := filepath.Dir(root)
		for _, candidate := range []string{filepath.Join(parent, ".quality-venv", "Scripts", "python.exe"), filepath.Join(parent, ".quality-venv", "bin", "python")} {
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
	}
	return "python"
}
