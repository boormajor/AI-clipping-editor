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

// cleanTitle trims transcript noise from a model- or cue-derived title.
func cleanTitle(s string) string {
	s = html.UnescapeString(html.UnescapeString(strings.TrimSpace(s)))
	s = speakerMarker.ReplaceAllString(s, "")
	s = strings.Trim(s, " \"“”.,!:-")
	if r := []rune(s); len(r) > 85 {
		s = strings.TrimSpace(string(r[:82])) + "…"
	}
	return s
}

// Candidate is a scored transcript window considered for a clip.
type Candidate struct {
	ID    int     `json:"id"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
	Score float64 `json:"score"`
}

// Selection limits accepted by SelectHighlights.
const (
	MinClipSeconds = 10
	MaxClipSeconds = 240
	MaxClipCount   = 30
	maxCandidates  = 50
	shortlistSize  = 35
)

// ValidateSelection checks the user's clip count and duration range.
func ValidateSelection(n int, minS, maxS float64) error {
	if n < 1 || n > MaxClipCount {
		return fmt.Errorf("clip count must be between 1 and %d", MaxClipCount)
	}
	if minS < MinClipSeconds || maxS > MaxClipSeconds || maxS < minS {
		return fmt.Errorf("clip length must be between %d and %d seconds, with the maximum at least the minimum", MinClipSeconds, MaxClipSeconds)
	}
	return nil
}

var sentenceEnd = regexp.MustCompile(`[.!?]["'”)]?\s*$`)

// Candidates builds non-overlapping transcript windows between minS and maxS
// seconds, favouring windows that end on a sentence boundary.
func Candidates(cues []Cue, minS, maxS float64) []Candidate {
	if minS < MinClipSeconds {
		minS = MinClipSeconds
	}
	if maxS < minS {
		maxS = minS + 30
	}
	var all []Candidate
	for i := 0; i < len(cues); i++ {
		start := cues[i].Start
		var b strings.Builder
		end := start
		best := Candidate{Score: -1}
		for j := i; j < len(cues) && j < i+200; j++ {
			if cues[j].Start-start > maxS {
				break
			}
			if j > i && cues[j].Start-end > 5 {
				break
			}
			b.WriteString(cues[j].Text)
			b.WriteString(" ")
			end = cues[j].End
			length := end - start
			if length < minS {
				continue
			}
			if length > maxS {
				break
			}
			text := strings.TrimSpace(b.String())
			score := rank(text, length)
			if sentenceEnd.MatchString(cues[j].Text) {
				score += 6
			}
			if score > best.Score {
				best = Candidate{Start: start, End: end, Text: text, Score: math.Min(score, 100)}
			}
		}
		if best.Score >= 0 {
			all = append(all, best)
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Score > all[j].Score })
	kept := make([]Candidate, 0, maxCandidates)
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
		if len(kept) >= maxCandidates {
			break
		}
	}
	return kept
}

// HookPhrases are weighted when ranking candidate windows. They can be tuned
// per project through Config.Instructions for the AI pass; the heuristic
// uses this list directly.
var HookPhrases = []string{"how to", "you should", "biggest mistake", "here's why", "the reason", "the problem", "actually", "secret", "never", "always", "seo", "marketing", "ai", "example", "important", "surprising", "strategy", "what happens", "imagine", "results", "ranking", "nobody", "everyone", "truth", "money"}

var hookPatterns = func() []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(HookPhrases))
	for _, p := range HookPhrases {
		out = append(out, regexp.MustCompile(`\b`+regexp.QuoteMeta(p)+`\b`))
	}
	return out
}()

func rank(t string, duration float64) float64 {
	l := strings.ToLower(t)
	score := 40. + math.Min(float64(len(t))/45, 25)
	for _, re := range hookPatterns {
		if re.MatchString(l) {
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
	return score
}

// SelectHighlights ranks candidate windows and, for online providers, asks the
// editorial model to pick, title and justify the strongest standalone moments.
func SelectHighlights(ctx context.Context, cues []Cue, n int, minS, maxS float64, provider ProviderConfig, instructions string) ([]Clip, error) {
	if err := ValidateSelection(n, minS, maxS); err != nil {
		return nil, err
	}
	pool := Candidates(cues, minS, maxS)
	if len(pool) == 0 {
		return nil, errors.New("no transcript passage is long enough for the requested clip length; lower the minimum duration")
	}
	chosen := pool
	metas := map[int]struct{ Title, Reason string }{}
	if provider.IsOnline() {
		shortlist := pool
		if len(shortlist) > shortlistSize {
			shortlist = shortlist[:shortlistSize]
		}
		request := struct {
			Candidates   []Candidate `json:"candidates"`
			Instructions string      `json:"instructions"`
			Max          int         `json:"max_clips"`
		}{shortlist, instructions, n}
		b, _ := json.Marshal(request)
		prompt := "You are the short-form EDITOR and quality reviewer for an interview podcast. New viewers have no context. Select ONLY moments that stand alone: the hook introduces WHO or WHAT is being discussed, explains jargon if needed, and makes a specific promise. Reject excerpts starting mid-thought (like 'and that's why', 'this thing', 'he said') without identifying their referent. Return up to max_clips DIFFERENT standalone moments, ranked strongest first. Aim for max_clips whenever enough usable moments exist. Do not invent moments to meet the count. Write a meaningful accurate title (max 70 chars) and a short reason naming the context. Never invent quotations, details, timestamps, or speakers. Output ONLY valid JSON of this exact shape: {\"clips\":[{\"id\":2,\"title\":\"Why Exact Match Domains Still Matter\",\"reason\":\"Introduces the EMD question and explains its answer.\"}]}. Use only candidate IDs provided, maximum max_clips. Input:\n" + string(b)
		response, err := AskModel(ctx, provider, prompt)
		if err != nil {
			return nil, fmt.Errorf("AI request failed (switch to the free heuristic to continue offline): %w", err)
		}
		var selection struct {
			Clips []struct {
				ID     int    `json:"id"`
				Title  string `json:"title"`
				Reason string `json:"reason"`
			} `json:"clips"`
		}
		if err = json.Unmarshal([]byte(ExtractJSON(response)), &selection); err != nil {
			return nil, fmt.Errorf("AI returned invalid clip selection JSON: %w", err)
		}
		chosen = nil
		seen := map[int]bool{}
		for _, row := range selection.Clips {
			if seen[row.ID] {
				continue
			}
			for _, c := range shortlist {
				if c.ID == row.ID {
					chosen = append(chosen, c)
					seen[row.ID] = true
					metas[row.ID] = struct{ Title, Reason string }{cleanTitle(row.Title), strings.TrimSpace(row.Reason)}
					break
				}
			}
		}
		// Top up with ranked candidates so the requested count is met when
		// enough distinct passages exist. These are not claimed as AI-approved.
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
	clips := make([]Clip, 0, len(chosen))
	for i, c := range chosen {
		summary := cleanTitle(c.Text)
		reason := "Ranked by the local heuristic. Review the opening and ending before publishing."
		if provider.IsOnline() {
			if _, ok := metas[c.ID]; !ok {
				reason = "Additional ranked candidate, not selected by the AI model. Check its opening context before publishing."
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
		clips = append(clips, Clip{ID: ClipID(i + 1), Title: summary, Start: c.Start, End: c.End, Score: math.Round(c.Score*10) / 10, Reason: reason, Selected: true, Pan: .5, Layout: "focus", Captions: true, CaptionStyle: "kinetic", Broll: []Broll{}})
	}
	return clips, nil
}

// ExtractJSON returns the outermost JSON object in a model response, tolerating
// code fences and surrounding prose.
func ExtractJSON(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}

// ModelClient is the HTTP client used for hosted model requests (tests may
// replace it).
var ModelClient = &http.Client{Timeout: 100 * time.Second}

// Model API endpoints (tests may replace them).
var (
	OpenAIResponsesURL   = "https://api.openai.com/v1/responses"
	AnthropicMessagesURL = "https://api.anthropic.com/v1/messages"
)

// Default models used when the caller leaves the model blank.
const (
	DefaultOpenAIModel    = "gpt-4.1-mini"
	DefaultAnthropicModel = "claude-sonnet-4-5"
)

// AskModel sends a prompt to the configured provider and returns its text.
func AskModel(ctx context.Context, p ProviderConfig, prompt string) (string, error) {
	provider := strings.ToLower(strings.TrimSpace(p.Provider))
	model := strings.TrimSpace(p.Model)
	var url string
	var payload any
	switch provider {
	case "claude-code":
		return askClaudeCode(ctx, prompt)
	case "chatgpt":
		if p.APIKey == "" {
			return "", errors.New("connect your ChatGPT account first")
		}
		if model == "" {
			return "", errors.New("select an available ChatGPT model in AI Connections")
		}
		return askChatGPTPlan(ctx, p.APIKey, model, prompt)
	case "openai":
		if model == "" {
			model = DefaultOpenAIModel
		}
		if p.APIKey == "" {
			return "", errors.New("OpenAI requires an API key; a ChatGPT subscription is not an API key")
		}
		url = OpenAIResponsesURL
		payload = map[string]any{"model": model, "input": prompt}
	case "anthropic":
		if model == "" {
			model = DefaultAnthropicModel
		}
		if p.APIKey == "" {
			return "", errors.New("Anthropic requires an API key")
		}
		url = AnthropicMessagesURL
		payload = map[string]any{"model": model, "max_tokens": 4096, "messages": []map[string]string{{"role": "user", "content": prompt}}}
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
	resp, e := ModelClient.Do(req)
	if e != nil {
		return "", e
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 3*1024*1024))
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s: %s", resp.Status, Head(string(rb), 450))
	}
	var response map[string]json.RawMessage
	if e = json.Unmarshal(rb, &response); e != nil {
		return "", e
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
		if stop := string(response["stop_reason"]); strings.Contains(stop, "max_tokens") {
			return out.String(), errors.New("the model response was cut off; try a smaller clip count")
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
