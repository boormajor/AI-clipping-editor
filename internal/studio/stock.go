package studio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The Pexels key stays in the request and is never part of saved projects.
type StockVideo struct {
	ID      int    `json:"id"`
	Preview string `json:"preview"`
	URL     string `json:"url"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Credit  string `json:"credit"`
}

func SearchStock(ctx context.Context, key, query string) ([]StockVideo, error) {
	if strings.TrimSpace(key) == "" {
		return nil, errors.New("Pexels requires a free API key from pexels.com/api")
	}
	if len(query) < 2 || len(query) > 100 {
		return nil, errors.New("Search must contain 2–100 characters")
	}
	endpoint := "https://api.pexels.com/v1/videos/search?query=" + url.QueryEscape(query) + "&orientation=portrait&per_page=12"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", key)
	client := &http.Client{Timeout: 25 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 400))
		return nil, fmt.Errorf("Pexels (%d): %s", resp.StatusCode, string(body))
	}
	var raw struct {
		Videos []struct {
			ID    int    `json:"id"`
			Image string `json:"image"`
			User  struct {
				Name string `json:"name"`
			} `json:"user"`
			VideoFiles []struct {
				Link    string `json:"link"`
				Quality string `json:"quality"`
				Width   int    `json:"width"`
				Height  int    `json:"height"`
			} `json:"video_files"`
		} `json:"videos"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 3<<20)).Decode(&raw); err != nil {
		return nil, err
	}
	result := []StockVideo{}
	for _, v := range raw.Videos {
		best := -1
		for i, f := range v.VideoFiles {
			if !allowedVideoURL(f.Link) {
				continue
			}
			if best < 0 || ((f.Height >= 720 && f.Height <= 2160) && f.Width > v.VideoFiles[best].Width) {
				best = i
			}
		}
		if best < 0 {
			continue
		}
		f := v.VideoFiles[best]
		result = append(result, StockVideo{ID: v.ID, Preview: v.Image, URL: f.Link, Width: f.Width, Height: f.Height, Credit: v.User.Name})
	}
	return result, nil
}
func allowedVideoURL(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && u.Scheme == "https" && u.Hostname() == "videos.pexels.com"
}
func DownloadStock(ctx context.Context, src, dir string, id int) (string, error) {
	if !allowedVideoURL(src) {
		return "", errors.New("only verified Pexels video download URLs are permitted")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	target := filepath.Join(dir, "pexels-"+strconv.Itoa(id)+".mp4")
	if info, e := os.Stat(target); e == nil && info.Size() > 0 {
		return target, nil
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if e != nil {
		return "", e
	}
	client := &http.Client{Timeout: 150 * time.Second}
	resp, e := client.Do(req)
	if e != nil {
		return "", e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("stock download returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > 250<<20 {
		return "", errors.New("stock video exceeds 250 MB limit")
	}
	tmp, e := os.CreateTemp(dir, "stock-*.partial")
	if e != nil {
		return "", e
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	n, e := io.Copy(tmp, io.LimitReader(resp.Body, (250<<20)+1))
	if e != nil {
		return "", e
	}
	if n > 250<<20 {
		return "", errors.New("stock video exceeds 250 MB limit")
	}
	if e = tmp.Close(); e != nil {
		return "", e
	}
	return target, os.Rename(tmp.Name(), target)
}
