package studio

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CloudSpeech configures hosted Whisper transcription. The key is used for the
// import request only and is never written to a project file.
type CloudSpeech struct {
	Provider string // groq or openai
	APIKey   string
	Language string // ISO-639-1 code or "auto"
	// CacheDir, when set, stores per-chunk results so re-importing an episode
	// does not pay for transcription again.
	CacheDir string
	// Concurrency bounds parallel chunk uploads (default 2).
	Concurrency int
	// Client overrides the HTTP client (tests).
	Client *http.Client
	// Endpoint overrides the API URL (tests).
	Endpoint string
}

// SpeechProviders lists the hosted speech services the app can call.
var SpeechProviders = []string{"groq", "openai"}

// SpeechLanguages lists the language codes offered in the UI; "auto" lets the
// model detect the language.
var SpeechLanguages = []string{"auto", "en", "ur", "hi", "es", "fr", "de", "it", "pt", "ar", "ja", "zh", "ko", "ru", "tr", "nl", "id"}

const (
	speechChunkSeconds = 470.0
	speechChunkOverlap = 1.0
	speechMinChunk     = 1.5
	speechMaxBytes     = 24 * 1024 * 1024
	speechMaxAttempts  = 4
)

// ImportProgress reports transcription progress: chunks done out of total and
// a short human-readable stage.
type ImportProgress func(done, total int, stage string)

func speechEndpoint(cfg CloudSpeech) (endpoint, model string, err error) {
	switch cfg.Provider {
	case "groq":
		return "https://api.groq.com/openai/v1/audio/transcriptions", "whisper-large-v3-turbo", nil
	case "openai":
		return "https://api.openai.com/v1/audio/transcriptions", "whisper-1", nil
	}
	return "", "", errors.New("choose Groq or OpenAI for cloud transcription")
}

// ValidateSpeech checks the configuration before any audio is extracted.
func ValidateSpeech(cfg CloudSpeech) error {
	if _, _, err := speechEndpoint(cfg); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return errors.New("enter a Groq or OpenAI speech API key; a ChatGPT or Claude subscription is not a speech API key")
	}
	if cfg.Language != "" && !contains(SpeechLanguages, cfg.Language) {
		return fmt.Errorf("unsupported language %q", cfg.Language)
	}
	return nil
}

type speechResponse struct {
	Words []Word `json:"words"`
	Text  string `json:"text"`
}

type retryableError struct {
	err   error
	after time.Duration
}

func (r *retryableError) Error() string { return r.err.Error() }
func (r *retryableError) Unwrap() error { return r.err }

// transcribePart uploads one audio chunk and returns measured words. A chunk
// with no speech returns an empty slice rather than an error.
func transcribePart(ctx context.Context, audioPath string, cfg CloudSpeech) ([]Word, error) {
	endpoint, model, err := speechEndpoint(cfg)
	if err != nil {
		return nil, err
	}
	if cfg.Endpoint != "" {
		endpoint = cfg.Endpoint
	}
	f, err := os.Open(audioPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if st, err := f.Stat(); err != nil {
		return nil, err
	} else if st.Size() > speechMaxBytes {
		return nil, errors.New("audio chunk exceeds the cloud API's 25 MB limit")
	}
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	_ = mw.WriteField("model", model)
	_ = mw.WriteField("response_format", "verbose_json")
	_ = mw.WriteField("timestamp_granularities[]", "word")
	_ = mw.WriteField("timestamp_granularities[]", "segment")
	if cfg.Language != "" && cfg.Language != "auto" {
		_ = mw.WriteField("language", cfg.Language)
	}
	part, err := mw.CreateFormFile("file", "speech.mp3")
	if err != nil {
		return nil, err
	}
	if _, err = io.Copy(part, f); err != nil {
		return nil, err
	}
	if err = mw.Close(); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: 4 * time.Minute}
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, &retryableError{err: err}
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 8*1024*1024))
	if err != nil {
		return nil, &retryableError{err: err}
	}
	if res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500 {
		wait := time.Duration(0)
		if ra := res.Header.Get("Retry-After"); ra != "" {
			if secs, e := strconv.Atoi(ra); e == nil {
				wait = time.Duration(secs) * time.Second
			}
		}
		return nil, &retryableError{err: fmt.Errorf("%s transcription HTTP %d: %s", cfg.Provider, res.StatusCode, Head(string(raw), 200)), after: wait}
	}
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("%s transcription failed (HTTP %d): %s", cfg.Provider, res.StatusCode, Head(string(raw), 280))
	}
	var result speechResponse
	if err = json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("cloud transcription response: %w", err)
	}
	if len(result.Words) == 0 {
		if strings.TrimSpace(result.Text) == "" {
			return []Word{}, nil // silence, music or noise: nothing to transcribe
		}
		return nil, fmt.Errorf("%s returned text without word timestamps; check the model and API tier support word-level timing", cfg.Provider)
	}
	return result.Words, nil
}

// transcribeWithRetry retries transient failures with exponential backoff.
func transcribeWithRetry(ctx context.Context, audioPath string, cfg CloudSpeech) ([]Word, error) {
	var last error
	for attempt := 0; attempt < speechMaxAttempts; attempt++ {
		words, err := transcribePart(ctx, audioPath, cfg)
		if err == nil {
			return words, nil
		}
		var re *retryableError
		if !errors.As(err, &re) || ctx.Err() != nil {
			return nil, err
		}
		last = err
		wait := re.after
		if wait <= 0 {
			wait = time.Duration(math.Pow(2, float64(attempt))) * 2 * time.Second
		}
		if wait > 90*time.Second {
			wait = 90 * time.Second
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	return nil, fmt.Errorf("gave up after %d attempts: %w", speechMaxAttempts, last)
}

type speechChunk struct {
	index int
	base  float64 // nominal start (seconds in the source)
	start float64 // extraction start including overlap
	end   float64 // extraction end including overlap
}

func planChunks(duration float64) []speechChunk {
	var chunks []speechChunk
	for base := 0.0; base < duration; base += speechChunkSeconds {
		start := math.Max(0, base-speechChunkOverlap)
		end := math.Min(duration, base+speechChunkSeconds+speechChunkOverlap)
		if end-base < speechMinChunk && len(chunks) > 0 {
			// Fold a tiny tail into the previous chunk instead of paying for an
			// almost-empty upload that the API may reject.
			chunks[len(chunks)-1].end = end
			break
		}
		chunks = append(chunks, speechChunk{index: len(chunks), base: base, start: start, end: end})
	}
	return chunks
}

func cacheKey(video string, info os.FileInfo, cfg CloudSpeech, chunk speechChunk) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%d|%d|%s|%s|%d|%.3f|%.3f", video, info.Size(), info.ModTime().UnixNano(), cfg.Provider, cfg.Language, chunk.index, chunk.start, chunk.end)
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// TranscribeVideoAudio extracts compressed mono audio chunks, sends them to the
// selected hosted Whisper API and returns measured word timings for the whole
// episode. It never interpolates or invents timings.
func TranscribeVideoAudio(ctx context.Context, video string, duration float64, cfg CloudSpeech, progress ImportProgress) ([]Word, error) {
	if duration <= 0 {
		return nil, errors.New("video duration is missing")
	}
	if err := ValidateSpeech(cfg); err != nil {
		return nil, err
	}
	if !HasTool("ffmpeg") {
		return nil, errors.New("FFmpeg is required to extract podcast audio")
	}
	info, err := os.Stat(video)
	if err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp("", "podcast-cloud-speech-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	if progress == nil {
		progress = func(int, int, string) {}
	}

	chunks := planChunks(duration)
	total := len(chunks)
	results := make([][]Word, total)
	workers := cfg.Concurrency
	if workers <= 0 {
		workers = 2
	}
	if workers > total {
		workers = total
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		mu    sync.Mutex
		done  int
		first error
		wg    sync.WaitGroup
		queue = make(chan speechChunk)
	)
	fail := func(e error) {
		mu.Lock()
		if first == nil {
			first = e
			cancel()
		}
		mu.Unlock()
	}
	progress(0, total, fmt.Sprintf("Extracting audio in %d part(s)", total))
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for chunk := range queue {
				words, e := transcribeChunk(ctx, video, info, tmp, chunk, cfg)
				if e != nil {
					fail(fmt.Errorf("cloud transcription at %s–%s: %w", clock(chunk.base), clock(chunk.end), e))
					continue
				}
				mu.Lock()
				results[chunk.index] = words
				done++
				progress(done, total, fmt.Sprintf("Transcribed %d of %d parts", done, total))
				mu.Unlock()
			}
		}()
	}
	for _, chunk := range chunks {
		select {
		case queue <- chunk:
		case <-ctx.Done():
		}
	}
	close(queue)
	wg.Wait()
	if first != nil {
		return nil, first
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	all := make([]Word, 0, 4096)
	for i, chunk := range chunks {
		upper := chunk.base + speechChunkSeconds
		if i == total-1 {
			upper = duration + 1
		}
		for _, w := range results[i] {
			middle := (w.Start + w.End) / 2
			if middle < chunk.base || middle >= upper || w.Start < 0 || w.End <= w.Start || w.Start > duration+1 {
				continue
			}
			w.Word = strings.TrimSpace(w.Word)
			if w.Word != "" {
				all = append(all, w)
			}
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Start < all[j].Start })
	if len(all) == 0 {
		return nil, errors.New("the speech service found no spoken words in this video")
	}
	progress(total, total, "Transcription complete")
	return all, nil
}

func transcribeChunk(ctx context.Context, video string, info os.FileInfo, tmp string, chunk speechChunk, cfg CloudSpeech) ([]Word, error) {
	var cachePath string
	if cfg.CacheDir != "" {
		cachePath = filepath.Join(cfg.CacheDir, cacheKey(video, info, cfg, chunk)+".json")
		var cached []Word
		if err := ReadJSONLimited(cachePath, 32*1024*1024, &cached); err == nil {
			return cached, nil
		}
	}
	audio := filepath.Join(tmp, fmt.Sprintf("part-%03d.mp3", chunk.index))
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y",
		"-ss", strconv.FormatFloat(chunk.start, 'f', 3, 64), "-i", video,
		"-t", strconv.FormatFloat(chunk.end-chunk.start, 'f', 3, 64),
		"-map", "0:a:0", "-vn", "-ac", "1", "-ar", "16000", "-c:a", "libmp3lame", "-b:a", "48k", audio}
	if _, err := DefaultTool.Run(ctx, "ffmpeg", args...); err != nil {
		return nil, fmt.Errorf("could not extract audio: %w", err)
	}
	defer os.Remove(audio)
	words, err := transcribeWithRetry(ctx, audio, cfg)
	if err != nil {
		return nil, err
	}
	// Shift from chunk-relative to absolute source time.
	for i := range words {
		words[i].Start += chunk.start
		words[i].End += chunk.start
	}
	if cachePath != "" {
		_ = WriteJSONAtomic(cachePath, words, false)
	}
	return words, nil
}

func clock(seconds float64) string {
	s := int(seconds + 0.5)
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, (s%3600)/60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// WordsToCues groups measured words into readable transcript cues for clip
// selection and the transcript editor.
func WordsToCues(words []Word) []Cue {
	cues := make([]Cue, 0, len(words)/8+1)
	var group []Word
	flush := func() {
		if len(group) == 0 {
			return
		}
		parts := make([]string, 0, len(group))
		for _, w := range group {
			parts = append(parts, w.Word)
		}
		cues = append(cues, Cue{Start: group[0].Start, End: group[len(group)-1].End, Text: strings.Join(parts, " ")})
		group = nil
	}
	for _, w := range words {
		if len(group) > 0 {
			last := group[len(group)-1]
			if w.Start-last.End > 0.55 || w.End-group[0].Start > 3.5 || len(group) >= 13 {
				flush()
			}
		}
		group = append(group, w)
	}
	flush()
	return cues
}
