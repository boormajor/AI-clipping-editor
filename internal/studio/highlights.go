package studio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

var speakerMarker = regexp.MustCompile(`(?i)^\s*(?:>>+|speaker\s*\d+\s*:)\s*`)

func cleanTitle(s string) string {
	s = html.UnescapeString(html.UnescapeString(strings.TrimSpace(s)))
	s = speakerMarker.ReplaceAllString(s, "")
	s = strings.Trim(s, " \"\u201c\u201d.,!:-")
	if r := []rune(s); len(r) > 85 {
		s = strings.TrimSpace(string(r[:82])) + "…"
	}
	return s
}

type Candidate struct {
	ID    int     `json:"id"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
	Score float64 `json:"score"`
}

func Candidates(cues []Cue, minS, maxS float64) []Candidate {
	if minS < 10 {
		minS = 20
	}
	if maxS < minS {
		maxS = minS + 30
	}
	var all []Candidate
	for i := 0; i < len(cues); i++ {
		start := cues[i].Start
		var b strings.Builder
		end := start
		for j := i; j < len(cues) && j < i+100; j++ {
			if cues[j].Start-start > maxS {
				break
			}
			if j > i && cues[j].Start-end > 5 {
				break
			}
			b.WriteString(cues[j].Text)
			b.WriteString(" ")
			end = cues[j].End
			if end-start >= minS && end-start <= maxS {
				text := strings.TrimSpace(b.String())
				score := rank(text, end-start)
				all = append(all, Candidate{Start: start, End: end, Text: text, Score: score})
				break
			}
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Score > all[j].Score })
	kept := make([]Candidate, 0, 50)
	for _, c := range all {
		conflict := false
		for _, k := range kept {
			overlap := math.Min(c.End, k.End) - math.Max(c.Start, k.Start)
			if overlap > 8 {
				conflict = true
				break
			}
		}
		if !conflict {
			c.ID = len(kept) + 1
			kept = append(kept, c)
		}
		if len(kept) >= 50 {
			break
		}
	}
	return kept
}
func rank(t string, duration float64) float64 {
	l := strings.ToLower(t)
	score := 40. + math.Min(float64(len(t))/45, 25)
	for _, v := range []string{"how to", "you should", "biggest mistake", "here's why", "the reason", "the problem", "actually", "secret", "never", "always", "seo", "marketing", "ai", "example", "important", "surprising", "strategy", "what happens", "imagine", "results", "ranking"} {
		if strings.Contains(l, v) {
			score += 4
		}
	}
	if strings.Contains(t, "?") {
		score += 3
	}
	if duration >= 30 && duration <= 65 {
		score += 4
	}
	if len(t) < 130 {
		score -= 20
	}
	return math.Min(score, 100)
}
func SelectHighlights(ctx context.Context, cues []Cue, n int, minS, maxS float64, provider ProviderConfig, instructions string) ([]Clip, error) {
	if n < 1 || n > 30 {
		return nil, errors.New("clip count must be between 1 and 30")
	}
	pool := Candidates(cues, minS, maxS)
	if len(pool) == 0 {
		return nil, errors.New("no timestamped transcript passages are long enough; supply SRT/VTT or align a plain transcript first")
	}
	chosen := pool
	metas := map[int]struct{ Title, Reason string }{}
	if provider.Provider != "local" && provider.Provider != "heuristic" && provider.Provider != "" {
		shortlist := pool
		if len(shortlist) > 35 {
			shortlist = shortlist[:35]
		}
		request := struct {
			Candidates   []Candidate `json:"candidates"`
			Instructions string      `json:"instructions"`
			Max          int         `json:"max_clips"`
		}{shortlist, instructions, n}
		b, _ := json.Marshal(request)
		prompt := "You are the short-form EDITOR and quality reviewer for a marketing/SEO guest podcast. New viewers have no context. Select ONLY moments that stand alone: the hook introduces WHO or WHAT is being discussed, explains jargon if needed, and makes a specific promise. Reject excerpts starting mid-thought (like 'and that's why', 'this thing', 'he said') without identifying their referent. Return up to max_clips DIFFERENT standalone moments, ranked strongest first. Aim for max_clips whenever enough usable moments exist. Do not invent moments to meet the count. Write a meaningful accurate title (max 70 chars) and a short reason naming the context. Never invent quotations, details, timestamps, or speakers. Output ONLY valid JSON of this exact shape: {\"clips\":[{\"id\":2,\"title\":\"Why Exact Match Domains Still Matter\",\"reason\":\"Introduces the EMD question and explains its answer.\"}]}. Use only candidate IDs provided, maximum max_clips. Input:\n" + string(b)
		response, err := AskModel(ctx, provider, prompt)
		if err != nil {
			return nil, fmt.Errorf("AI request failed (local fallback available by selecting Heuristic): %w", err)
		}
		var selection struct {
			Clips []struct {
				ID     int    `json:"id"`
				Title  string `json:"title"`
				Reason string `json:"reason"`
			} `json:"clips"`
			IDs []int `json:"ids"`
		}
		clean := strings.TrimSpace(response)
		clean = strings.TrimPrefix(clean, "```json")
		clean = strings.TrimPrefix(clean, "```")
		clean = strings.TrimSuffix(clean, "```")
		if err = json.Unmarshal([]byte(strings.TrimSpace(clean)), &selection); err != nil {
			return nil, fmt.Errorf("AI returned invalid clip selection JSON: %w", err)
		}
		if len(selection.Clips) > 0 {
			for _, row := range selection.Clips {
				selection.IDs = append(selection.IDs, row.ID)
				metas[row.ID] = struct{ Title, Reason string }{cleanTitle(row.Title), strings.TrimSpace(row.Reason)}
			}
		}
		chosen = nil
		seen := map[int]bool{}
		for _, id := range selection.IDs {
			for _, c := range shortlist {
				if c.ID == id && !seen[id] {
					chosen = append(chosen, c)
					seen[id] = true
					break
				}
			}
		}
		// Keep the requested clip count when there are enough distinct passages.
		// Fallback entries are NOT claimed to be AI-approved; each needs review.
		for _, candidate := range pool {
			if len(chosen) >= n {
				break
			}
			if !seen[candidate.ID] {
				chosen = append(chosen, candidate)
				seen[candidate.ID] = true
			}
		}
		if len(chosen) == 0 {
			return nil, errors.New("no non-overlapping candidates found")
		}
	}
	if len(chosen) > n {
		chosen = chosen[:n]
	}
	var clips []Clip
	for i, c := range chosen {
		summary := cleanTitle(c.Text)
		reason := "Review the opening and ending: this clip has not passed a full context audit."
		if provider.Provider != "local" && provider.Provider != "heuristic" && provider.Provider != "" {
			if _, ok := metas[c.ID]; !ok {
				reason = "Additional ranked candidate, not selected by the AI model. Check its opening context and spoken words before publishing."
			}
		}
		if v, ok := metas[c.ID]; ok {
			if v.Title != "" {
				summary = v.Title
			}
			if v.Reason != "" {
				reason = v.Reason
			}
		}
		clips = append(clips, Clip{ID: fmt.Sprintf("clip-%02d", i+1), Title: summary, Start: c.Start, End: c.End, Score: math.Round(c.Score*10) / 10, Reason: reason, Selected: true, Pan: .5, Layout: "focus", Captions: true, CaptionStyle: "bold"})
	}
	return clips, nil
}
func AskModel(ctx context.Context, p ProviderConfig, prompt string) (string, error) {
	provider := strings.ToLower(p.Provider)
	model := strings.TrimSpace(p.Model)
	var url string
	var payload any
	switch provider {
	case "claude-code":
		return askClaudeCode(ctx, prompt)
	case "chatgpt":
		if p.APIKey == "" {
			return "", errors.New("connect your ChatGPT Plus account first")
		}
		if model == "" {
			return "", errors.New("select an available ChatGPT model in AI Connections")
		}
		return askChatGPTPlan(ctx, p.APIKey, model, prompt)
	case "ollama":
		if model == "" {
			model = "llama3.2"
		}
		url = "http://127.0.0.1:11434/api/chat"
		payload = map[string]any{"model": model, "stream": false, "messages": []map[string]string{{"role": "user", "content": prompt}}, "format": "json"}
	case "openai":
		if model == "" {
			model = "gpt-4.1-mini"
		}
		if p.APIKey == "" {
			return "", errors.New("OpenAI requires an API key; a ChatGPT Plus login is not an API key")
		}
		url = "https://api.openai.com/v1/responses"
		payload = map[string]any{"model": model, "input": prompt}
	case "anthropic":
		if model == "" {
			model = "claude-sonnet-4-5"
		}
		if p.APIKey == "" {
			return "", errors.New("Anthropic requires an API key")
		}
		url = "https://api.anthropic.com/v1/messages"
		payload = map[string]any{"model": model, "max_tokens": 1024, "messages": []map[string]string{{"role": "user", "content": prompt}}}
	default:
		return "", fmt.Errorf("unknown model provider %q", provider)
	}
	b, _ := json.Marshal(payload)
	req, e := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(b))
	if e != nil {
		return "", e
	}
	req.Header.Set("Content-Type", "application/json")
	if provider == "openai" {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
	}
	if provider == "anthropic" {
		req.Header.Set("x-api-key", p.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	}
	client := &http.Client{Timeout: 100 * time.Second}
	resp, e := client.Do(req)
	if e != nil {
		return "", e
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 3*1024*1024))
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s: %s", resp.Status, truncate(string(rb), 450))
	}
	var response map[string]json.RawMessage
	if e = json.Unmarshal(rb, &response); e != nil {
		return "", e
	}
	if provider == "ollama" {
		var m struct {
			Content string `json:"content"`
		}
		if e = json.Unmarshal(response["message"], &m); e != nil {
			return "", e
		}
		return m.Content, nil
	}
	if provider == "anthropic" {
		var blocks []struct {
			Text string `json:"text"`
		}
		if e = json.Unmarshal(response["content"], &blocks); e != nil {
			return "", e
		}
		var out strings.Builder
		for _, v := range blocks {
			out.WriteString(v.Text)
		}
		return out.String(), nil
	}
	var items []struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if e = json.Unmarshal(response["output"], &items); e != nil {
		return "", e
	}
	var out strings.Builder
	for _, v := range items {
		for _, c := range v.Content {
			out.WriteString(c.Text)
		}
	}
	return out.String(), nil
}
func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if r := []rune(s); len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}
