package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/boormajor/podcast-ai-studio/internal/studio"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// reviewSelected asks the online model for *editorial* judgment, using full
// surrounding context, available real word-alignment and detector metadata.
// It never claims to have watched the video if no vision frames were sent.
func (a *application) reviewSelected(w http.ResponseWriter, r *http.Request) {
	var q struct {
		ID       string                `json:"id"`
		Provider studio.ProviderConfig `json:"provider"`
	}
	if err := decode(r, &q); err != nil {
		fail(w, 400, err.Error())
		return
	}
	a.mu.Lock()
	p := a.project
	a.mu.Unlock()
	var c studio.Clip
	found := false
	for _, x := range p.Clips {
		if x.ID == q.ID {
			c = x
			found = true
			break
		}
	}
	if !found {
		fail(w, 404, "no clip selected")
		return
	}
	if q.Provider.Provider == "local" || q.Provider.Provider == "heuristic" || q.Provider.Provider == "" {
		fail(w, 422, "Select ChatGPT, Claude, or another online AI provider for editorial review")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()
	if q.Provider.Provider == "chatgpt" {
		token, err := a.auth.accessToken(ctx)
		if err != nil {
			fail(w, 401, err.Error())
			return
		}
		q.Provider.APIKey = token
		if q.Provider.Model == "" {
			models, err := a.auth.models(ctx)
			if err != nil || len(models) == 0 {
				fail(w, 422, "Select an available ChatGPT model")
				return
			}
			q.Provider.Model = models[0]["slug"]
		}
	}
	before, core, after := "", "", ""
	for _, cue := range p.Cues {
		switch {
		case cue.End <= c.Start && cue.Start >= c.Start-18:
			before += cue.Text + " "
		case cue.Start >= c.Start && cue.Start < c.End:
			core += cue.Text + " "
		case cue.Start >= c.End && cue.Start <= c.End+14:
			after += cue.Text + " "
		}
	}
	// Optional detector report is produced locally, with no dependency on cloud video uploads.
	faceInfo := "Not available (face tracking tool not installed)."
	if true {
		if script, err := findFaceAudit(); err == nil {
			temp, err := os.MkdirTemp("", "face-audit-")
			if err == nil {
				defer os.RemoveAll(temp)
				clipFile := filepath.Join(temp, "clip.mp4")
				reportFile := filepath.Join(temp, "faces.json")
				ff := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-ss", fmt.Sprintf("%.3f", c.Start), "-t", fmt.Sprintf("%.3f", c.End-c.Start), "-i", p.VideoPath, "-vf", "scale=640:-2", "-an", clipFile)
				if ff.Run() == nil {
					sc := exec.CommandContext(ctx, qualityPython(), script, "--video", clipFile, "--out", reportFile)
					if sc.Run() == nil {
						if b, e := os.ReadFile(reportFile); e == nil {
							faceInfo = string(b)
						}
					}
				}
			}
		}
	}
	sceneInfo := "Scene-cut detection unavailable."
	if sceneScript, sceneErr := findSceneAudit(); sceneErr == nil {
		temp, er := os.MkdirTemp("", "podcast-scenes-")
		if er == nil {
			defer os.RemoveAll(temp)
			sceneFile := filepath.Join(temp, "cuts.json")
			clipFile := filepath.Join(temp, "preview.mp4")
			ff := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-ss", fmt.Sprintf("%.3f", c.Start), "-t", fmt.Sprintf("%.3f", c.End-c.Start), "-i", p.VideoPath, "-vf", "scale=480:-2", "-an", clipFile)
			if ff.Run() == nil {
				cmd := exec.CommandContext(ctx, qualityPython(), sceneScript, "--video", clipFile, "--out", sceneFile)
				if cmd.Run() == nil {
					if b, e := os.ReadFile(sceneFile); e == nil {
						sceneInfo = string(b)
					}
				}
			}
		}
	}
	alignment := "Not run yet; word-level caption accuracy has not been verified."
	if len(c.Words) > 0 {
		alignment = fmt.Sprintf("%d words recognized and time-aligned; agreement to imported transcript: %.1f%%. Check proper nouns manually.", len(c.Words), c.CaptionAgreement*100)
	}
	prompt := `You are the senior video editor for a professional interview podcast. Your viewer has ZERO prior context. Improve the short clip honestly and conservatively. Review the text before, during, and after the suggested clip, and note missing antecedents or jargon. Rewrite the title to introduce the real topic. You are also given automated face-box metadata if available; this is NOT the images and can be wrong. Choose a safe layout: "two-up" if both speakers consistently appear side-by-side, "focus" when one speaker occupies the frame, "dynamic" only if an actual shot plan is warranted. Never invent camera evidence, facts, or speaker identity. If audio alignment has not run, say that captions are not verified. NEVER create word timestamps. Return ONLY JSON with this exact structure: {"title":"A specific useful title","contextOK":true,"issues":["..."],"layout":"two-up","shots":[{"start":0,"end":10,"mode":"both"}],"captionVerified":false,"summary":"Why it will make sense to a new viewer"}. Shot times are SECONDS RELATIVE TO THE CLIP, must cover all time, and mode is one of both|left|right. Prefer to show BOTH when unsure. Respond with at most 12 shots. No invented or unsafe file operations.
` +
		"PRECEDING (18s): " + before + "\nCLIP: " + core + "\nFOLLOWING (14s): " + after + "\nCLIP LENGTH: " + fmt.Sprintf("%.2f", c.End-c.Start) + "\nFRAME OBSERVATIONS: " + faceInfo + "\nSCENE CUT OBSERVATIONS: " + sceneInfo + "\nAUDIO: " + alignment + "\nUSER EDITING INSTRUCTIONS: " + p.Config.Instructions
	raw, err := studio.AskModel(ctx, q.Provider, prompt)
	if err != nil {
		fail(w, 422, err.Error())
		return
	}
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	var v struct {
		Title           string        `json:"title"`
		ContextOK       bool          `json:"contextOK"`
		Issues          []string      `json:"issues"`
		Layout          string        `json:"layout"`
		Shots           []studio.Shot `json:"shots"`
		CaptionVerified bool          `json:"captionVerified"`
		Summary         string        `json:"summary"`
	}
	if err = json.Unmarshal([]byte(strings.TrimSpace(raw)), &v); err != nil {
		fail(w, 422, "AI returned an invalid review format: "+err.Error())
		return
	}
	if len(v.Title) > 100 {
		v.Title = v.Title[:100]
	}
	if strings.TrimSpace(v.Title) == "" {
		v.Title = c.Title
	}
	if len(v.Issues) > 10 {
		v.Issues = v.Issues[:10]
	}
	if v.Layout != "two-up" && v.Layout != "focus" && v.Layout != "dynamic" {
		v.Layout = "two-up"
	}
	if len(v.Shots) > 12 {
		v.Shots = v.Shots[:12]
	}
	dur := c.End - c.Start
	prev := 0.0
	valid := len(v.Shots) > 0
	for _, s := range v.Shots {
		if s.Mode != "both" && s.Mode != "left" && s.Mode != "right" {
			valid = false
			break
		}
		if s.Start < prev-.02 || s.Start > prev+.02 || s.End <= s.Start || s.End > dur+.15 {
			valid = false
			break
		}
		prev = s.End
	}
	if prev < dur-.15 {
		valid = false
	}
	if !valid {
		v.Shots = nil
	}
	if len(c.Words) == 0 {
		v.CaptionVerified = false
	}
	a.mu.Lock()
	for i := range a.project.Clips {
		if a.project.Clips[i].ID == q.ID {
			a.project.Clips[i].Title = v.Title
			a.project.Clips[i].Layout = v.Layout
			a.project.Clips[i].Shots = v.Shots
			break
		}
	}
	a.project.LastUpdated = time.Now()
	a.mu.Unlock()
	reply(w, map[string]any{"title": v.Title, "contextOK": v.ContextOK, "issues": v.Issues, "layout": v.Layout, "shots": v.Shots, "captionVerified": v.CaptionVerified, "summary": v.Summary, "faceData": faceInfo != "Not available (face tracking tool not installed)."})
}
func findFaceAudit() (string, error) {
	root, err := studio.FindRemotion()
	if err != nil {
		return "", err
	}
	file := filepath.Join(filepath.Dir(root), "scripts", "face_audit.py")
	_, err = os.Stat(file)
	if err != nil {
		return "", errors.New("face audit script unavailable")
	}
	return file, nil
}

func findSceneAudit() (string, error) {
	root, e := studio.FindRemotion()
	if e != nil {
		return "", e
	}
	script := filepath.Join(filepath.Dir(root), "scripts", "scene_audit.py")
	if _, e = os.Stat(script); e != nil {
		return "", e
	}
	return script, nil
}
