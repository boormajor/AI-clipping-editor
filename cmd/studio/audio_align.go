package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/boormajor/podcast-ai-studio/internal/studio"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func findAlignmentScript() (string, error) {
	rem, err := studio.FindRemotion()
	if err != nil {
		return "", err
	}
	path := filepath.Join(filepath.Dir(rem), "scripts", "align_captions.py")
	if _, err = os.Stat(path); err != nil {
		return "", fmt.Errorf("alignment script missing: %w", err)
	}
	return path, nil
}

// alignSelected compares real audio with the transcript using WhisperX ASR.
// No cloud API or fabricated subtitle timings. It performs only on user request.
func (a *application) alignSelected(w http.ResponseWriter, r *http.Request) {
	var q struct {
		ID    string `json:"id"`
		Model string `json:"model"`
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
	for _, clip := range p.Clips {
		if clip.ID == q.ID {
			c = clip
			found = true
			break
		}
	}
	if !found {
		fail(w, 404, "clip not found")
		return
	}
	script, err := findAlignmentScript()
	if err != nil {
		fail(w, 422, err.Error())
		return
	}
	python := qualityPython()
	if _, err = exec.LookPath(python); err != nil {
		fail(w, 422, "Python is required: install Python 3.11+ and WhisperX. See README.")
		return
	}
	tmp, err := os.MkdirTemp("", "podcast-align-")
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	defer os.RemoveAll(tmp)
	audio := filepath.Join(tmp, "clip.wav")
	ref := filepath.Join(tmp, "reference.txt")
	out := filepath.Join(tmp, "words.json")
	var transcript strings.Builder
	for _, cue := range p.Cues {
		if cue.Start < c.End && cue.End > c.Start {
			transcript.WriteString(cue.Text)
			transcript.WriteByte(' ')
		}
	}
	if err = os.WriteFile(ref, []byte(transcript.String()), 0600); err != nil {
		fail(w, 500, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Minute)
	defer cancel()
	ffmpeg := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-nostdin", "-loglevel", "error", "-y", "-ss", strconv.FormatFloat(c.Start, 'f', 3, 64), "-t", strconv.FormatFloat(c.End-c.Start, 'f', 3, 64), "-i", p.VideoPath, "-vn", "-ac", "1", "-ar", "16000", audio)
	if bytes, e := ffmpeg.CombinedOutput(); e != nil {
		fail(w, 422, fmt.Sprintf("could not extract clip audio: %v: %s", e, string(bytes)))
		return
	}
	model := strings.TrimSpace(q.Model)
	if model == "" {
		model = "small"
	}
	cmd := exec.CommandContext(ctx, python, script, "--audio", audio, "--reference", ref, "--output", out, "--model", model)
	var log bytes.Buffer
	cmd.Stdout = &log
	cmd.Stderr = &log
	if err = cmd.Run(); err != nil {
		fail(w, 422, "WhisperX failed: "+log.String())
		return
	}
	data, e := os.ReadFile(out)
	if e != nil {
		fail(w, 500, e.Error())
		return
	}
	var result struct {
		Words              []studio.Word `json:"words"`
		ReferenceAgreement float64       `json:"referenceAgreement"`
		Warning            string        `json:"warning"`
		Method             string        `json:"method"`
	}
	if e = json.Unmarshal(data, &result); e != nil {
		fail(w, 500, e.Error())
		return
	}
	if len(result.Words) == 0 {
		fail(w, 422, "no reliably aligned words; original transcript remains unchanged")
		return
	}
	a.mu.Lock()
	for i := range a.project.Clips {
		if a.project.Clips[i].ID == q.ID {
			a.project.Clips[i].Words = result.Words
			a.project.Clips[i].CaptionAgreement = result.ReferenceAgreement
			break
		}
	}
	a.project.LastUpdated = time.Now()
	a.mu.Unlock()
	reply(w, map[string]any{"words": len(result.Words), "agreement": result.ReferenceAgreement, "warning": result.Warning, "method": result.Method})
}

// Resolve the packaged optional quality-tool venv before falling back to PATH.
func qualityPython() string {
	root, e := studio.FindRemotion()
	if e == nil {
		parent := filepath.Dir(root)
		candidates := []string{filepath.Join(parent, ".quality-venv", "Scripts", "python.exe"), filepath.Join(parent, ".quality-venv", "bin", "python")}
		for _, candidate := range candidates {
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
	}
	return "python"
}
