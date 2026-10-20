package main

import (
	"context"
	"github.com/boormajor/podcast-ai-studio/internal/studio"
	"net/http"
	"time"
)

func (a *application) reviewVision(w http.ResponseWriter, r *http.Request) {
	var q struct {
		ClipID string `json:"clipId"`
		Model  string `json:"model"`
	}
	if err := decode(r, &q); err != nil {
		fail(w, 400, err.Error())
		return
	}
	a.mu.Lock()
	p := a.project
	a.mu.Unlock()
	var c *studio.Clip
	for i := range p.Clips {
		if p.Clips[i].ID == q.ClipID {
			c = &p.Clips[i]
			break
		}
	}
	if c == nil {
		fail(w, 404, "select a clip first")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	review, err := studio.InspectFrames(ctx, p.VideoPath, *c, q.Model)
	if err != nil {
		fail(w, 422, err.Error())
		return
	}
	reply(w, map[string]any{"review": review, "private": true, "framesUploadedToCloud": false})
}
