package studio

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
)

func makeTestVideo(t *testing.T, seconds int) string {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	video := filepath.Join(t.TempDir(), "episode.mp4")
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c=black:s=160x90:r=10:d="+strconv.Itoa(seconds), "-f", "lavfi", "-i", "sine=frequency=300:duration="+strconv.Itoa(seconds), "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", video)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("make video: %v %s", err, out)
	}
	return video
}

func TestPlanChunksFoldsTinyTail(t *testing.T) {
	chunks := planChunks(speechChunkSeconds + 0.5)
	if len(chunks) != 1 {
		t.Fatalf("tiny tail should fold into the first chunk, got %d chunks", len(chunks))
	}
	if chunks[0].end != speechChunkSeconds+0.5 {
		t.Fatalf("folded chunk should reach the end: %+v", chunks[0])
	}
	if got := len(planChunks(3 * speechChunkSeconds)); got != 3 {
		t.Fatalf("expected 3 chunks, got %d", got)
	}
}

func TestTranscribeSkipsSilentChunkAndRetries(t *testing.T) {
	video := makeTestVideo(t, 4)
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing bearer token")
		}
		switch n {
		case 1:
			w.Header().Set("Retry-After", "0")
			http.Error(w, "busy", http.StatusTooManyRequests)
		default:
			_ = json.NewEncoder(w).Encode(speechResponse{Text: "hello world", Words: []Word{{Start: 0.2, End: 0.6, Word: "hello"}, {Start: 0.7, End: 1.1, Word: "world"}}})
		}
	}))
	defer server.Close()
	cfg := CloudSpeech{Provider: "groq", APIKey: "test-key", Language: "en", Endpoint: server.URL, Client: server.Client(), CacheDir: t.TempDir()}
	var stages []string
	words, err := TranscribeVideoAudio(context.Background(), video, 4, cfg, func(done, total int, stage string) { stages = append(stages, stage) })
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if len(words) != 2 || words[0].Word != "hello" || words[1].Start != 0.7 {
		t.Fatalf("unexpected words %+v", words)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected one retry, got %d calls", calls)
	}
	if len(stages) == 0 {
		t.Fatal("no progress reported")
	}
	// Second import hits the cache and never calls the API.
	atomic.StoreInt32(&calls, 0)
	if _, err := TranscribeVideoAudio(context.Background(), video, 4, cfg, nil); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("expected cached result, got %d calls", calls)
	}
}

func TestTranscribeSilentEpisodeIsAnError(t *testing.T) {
	video := makeTestVideo(t, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(speechResponse{Text: ""})
	}))
	defer server.Close()
	cfg := CloudSpeech{Provider: "openai", APIKey: "k", Endpoint: server.URL, Client: server.Client()}
	if _, err := TranscribeVideoAudio(context.Background(), video, 3, cfg, nil); err == nil {
		t.Fatal("an episode with no speech must report an error, not an empty transcript")
	}
}

func TestTranscribeNonRetryableError(t *testing.T) {
	video := makeTestVideo(t, 2)
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		http.Error(w, `{"error":"invalid key"}`, http.StatusUnauthorized)
	}))
	defer server.Close()
	cfg := CloudSpeech{Provider: "groq", APIKey: "bad", Endpoint: server.URL, Client: server.Client()}
	_, err := TranscribeVideoAudio(context.Background(), video, 2, cfg, nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("a 401 must not be retried; got %d calls", calls)
	}
}

func TestValidateSpeech(t *testing.T) {
	if err := ValidateSpeech(CloudSpeech{Provider: "groq", APIKey: "x", Language: "en"}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []CloudSpeech{{Provider: "other", APIKey: "x"}, {Provider: "groq"}, {Provider: "groq", APIKey: "x", Language: "klingon"}} {
		if err := ValidateSpeech(bad); err == nil {
			t.Fatalf("expected rejection for %+v", bad)
		}
	}
}

func TestWordsToCues(t *testing.T) {
	words := []Word{{0, 0.3, "one"}, {0.35, 0.6, "two"}, {2, 2.3, "three"}}
	cues := WordsToCues(words)
	if len(cues) != 2 || cues[0].Text != "one two" || cues[1].Text != "three" {
		t.Fatalf("unexpected cues %+v", cues)
	}
}
