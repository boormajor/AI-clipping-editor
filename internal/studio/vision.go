package studio

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// FrameReview is a suggestion requiring human verification, not a verified face count.
type FrameReview struct {
	Layout   string `json:"recommendation"`
	Faces    string `json:"visiblePeople"`
	Cropping string `json:"cropRisks"`
	Reason   string `json:"reason"`
}

// InspectFrames extracts short JPEG samples locally and sends them ONLY to a
// local Ollama vision model; no video/audio/frames are uploaded to cloud AI.
func InspectFrames(ctx context.Context, video string, clip Clip, model string) (FrameReview, error) {
	if video == "" || clip.End <= clip.Start {
		return FrameReview{}, errors.New("invalid video or clip")
	}
	if strings.TrimSpace(model) == "" {
		model = "gemma3:4b"
	}
	dur := clip.End - clip.Start
	relative := []float64{.3, .5, .8}
	var images []string
	for _, n := range relative {
		at := clip.Start + dur*n
		cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-ss", fmt.Sprintf("%.3f", at), "-i", video, "-frames:v", "1", "-vf", "scale=640:-2", "-f", "image2pipe", "-vcodec", "mjpeg", "pipe:1")
		raw, err := cmd.Output()
		if err != nil {
			return FrameReview{}, fmt.Errorf("cannot inspect frame at %.1fs: %w", at, err)
		}
		if len(raw) == 0 || len(raw) > 4*1024*1024 {
			return FrameReview{}, errors.New("failed to extract a usable frame")
		}
		images = append(images, base64.StdEncoding.EncodeToString(raw))
	}
	prompt := `You are a technical video framing reviewer. Three chronological frames come from the same 16:9 podcast recording. Look carefully for faces cut at the edges, participants missing when the recording shows both, and whether the source has a left/right split screen. Do not infer who is speaking from silent images. Do not invent frame details you cannot verify. Recommend "two-up" ONLY if the frames consistently show two speakers side-by-side (left/right); otherwise recommend "focus" for single-speaker crops or "manual" when the layout changes. Return ONLY a JSON object with: {"recommendation":"focus|two-up|manual","visiblePeople":"...","cropRisks":"...","reason":"..."}. Explicitly mention that image samples cannot validate caption/audio timing.`
	payload := map[string]any{"model": model, "stream": false, "format": "json", "messages": []any{map[string]any{"role": "user", "content": prompt, "images": images}}}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", "http://127.0.0.1:11434/api/chat", bytes.NewReader(body))
	if err != nil {
		return FrameReview{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := http.Client{Timeout: 100 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return FrameReview{}, fmt.Errorf("Ollama not reachable: %w; install Ollama and run ollama pull %s", err, model)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if resp.StatusCode != 200 {
		return FrameReview{}, fmt.Errorf("Ollama vision returned %s: %s", resp.Status, truncate(string(data), 260))
	}
	var wrapped struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err = json.Unmarshal(data, &wrapped); err != nil {
		return FrameReview{}, err
	}
	var review FrameReview
	if err = json.Unmarshal([]byte(strings.TrimSpace(wrapped.Message.Content)), &review); err != nil {
		return FrameReview{}, fmt.Errorf("vision model returned invalid JSON: %w", err)
	}
	switch review.Layout {
	case "focus", "two-up", "manual":
	default:
		review.Layout = "manual"
	}
	if r := []rune(review.Reason); len(r) > 450 {
		review.Reason = string(r[:450])
	}
	return review, nil
}
