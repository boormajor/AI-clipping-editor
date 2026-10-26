package main

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/boormajor/podcast-ai-studio/internal/studio"
)

// Pexels stock footage. The API key travels with the request and is never
// written to a project file.

func (a *application) searchStock(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Query  string `json:"query"`
		APIKey string `json:"apiKey"`
	}
	if err := decode(r, &q); err != nil {
		fail(w, 400, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	results, err := studio.SearchStock(ctx, strings.TrimSpace(q.APIKey), strings.TrimSpace(q.Query))
	if err != nil {
		fail(w, 422, err.Error())
		return
	}
	reply(w, map[string]any{"videos": results})
}

func (a *application) downloadStock(w http.ResponseWriter, r *http.Request) {
	var q struct {
		URL string `json:"url"`
		ID  int    `json:"id"`
	}
	if err := decode(r, &q); err != nil {
		fail(w, 400, err.Error())
		return
	}
	p, _ := a.snapshot()
	if p.VideoPath == "" {
		fail(w, 400, "Import an episode first")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	path, err := studio.DownloadStock(ctx, q.URL, filepath.Join(p.ExportDir(), "stock-assets"), q.ID)
	if err != nil {
		fail(w, 422, err.Error())
		return
	}
	reply(w, map[string]any{"path": path})
}
