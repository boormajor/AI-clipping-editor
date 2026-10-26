package main

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/boormajor/podcast-ai-studio/internal/ui"
)

const (
	sessionCookie = "podcast_studio_session"
	tokenHeader   = "X-Studio-Token"
	maxJSONBody   = 8 * 1024 * 1024
)

// apiError is the JSON error envelope every failed request returns.
type apiError struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

// routes builds the HTTP handler: embedded UI, session bootstrap and the JSON
// API, wrapped in the security middleware.
func (a *application) routes(allowedHosts []string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /session", a.session)
	mux.HandleFunc("GET /api/status", a.status)
	mux.HandleFunc("GET /api/options", a.options)
	mux.HandleFunc("GET /api/jobs", a.getJobs)
	mux.HandleFunc("POST /api/jobs/cancel", a.cancelJob)
	mux.HandleFunc("POST /api/import", a.importEpisode)
	mux.HandleFunc("POST /api/project/new", a.newProject)
	mux.HandleFunc("POST /api/project/save", a.save)
	mux.HandleFunc("POST /api/project/open", a.open)
	mux.HandleFunc("POST /api/config", a.configure)
	mux.HandleFunc("POST /api/analyze", a.analyze)
	mux.HandleFunc("POST /api/clip", a.updateClip)
	mux.HandleFunc("POST /api/clip/review", a.reviewClip)
	mux.HandleFunc("POST /api/clip/clean", a.cleanClip)
	mux.HandleFunc("POST /api/clip/metadata", a.generateMetadata)
	mux.HandleFunc("POST /api/transcript", a.editTranscript)
	mux.HandleFunc("POST /api/render", a.render)
	mux.HandleFunc("POST /api/pick", a.pick)
	mux.HandleFunc("POST /api/reveal", a.reveal)
	mux.HandleFunc("POST /api/stock/search", a.searchStock)
	mux.HandleFunc("POST /api/stock/download", a.downloadStock)
	mux.HandleFunc("GET /api/media", a.media)
	mux.HandleFunc("GET /api/auth/status", a.authStatus)
	mux.HandleFunc("POST /api/auth/chatgpt/start", a.authChatGPTStart)
	mux.HandleFunc("GET /api/auth/chatgpt/models", a.authChatGPTModels)
	mux.HandleFunc("POST /api/auth/chatgpt/disconnect", a.authChatGPTDisconnect)
	mux.HandleFunc("GET /api/auth/claude/status", a.authClaudeStatus)
	mux.HandleFunc("POST /api/auth/claude/login", a.authClaudeLogin)
	mux.HandleFunc("POST /api/quit", a.quit)
	mux.Handle("/", a.staticHandler())
	return a.middleware(mux, allowedHosts)
}

// staticHandler serves the embedded UI with no-cache headers so an upgraded
// build never runs against stale cached scripts.
func (a *application) staticHandler() http.Handler {
	fs := http.FileServer(http.FS(ui.Assets()))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			fail(w, 404, "Unknown API route")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			fail(w, 405, "Method not allowed")
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		fs.ServeHTTP(w, r)
	})
}

// middleware applies the local-app security model:
//   - Host must be the loopback address the server was started on (defeats DNS rebinding).
//   - Every /api request must carry the per-launch session token (cookie or header).
//   - POST requests must be same-origin JSON with a bounded body.
//   - Panics become 500 responses instead of killing the process.
func (a *application) middleware(next http.Handler, allowedHosts []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				a.log.Error("handler panic", "path", r.URL.Path, "panic", rec, "stack", string(debug.Stack()))
				fail(w, 500, "Internal error; see studio.log")
			}
		}()
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; img-src 'self' data: blob: https://images.pexels.com; media-src 'self' blob:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		if !hostAllowed(r.Host, allowedHosts) {
			fail(w, 421, "Podcast AI Studio only answers on its loopback address")
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			if !a.authorized(r) {
				failCode(w, 401, "unauthorized", "This browser tab is not linked to the running app. Open the link printed by Podcast AI Studio.")
				return
			}
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				if origin := r.Header.Get("Origin"); origin != "" && !originAllowed(origin, allowedHosts) {
					fail(w, 403, "Only the local app may change projects")
					return
				}
				if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
					fail(w, 403, "Cross-site requests are not allowed")
					return
				}
				if ct := strings.ToLower(r.Header.Get("Content-Type")); !strings.HasPrefix(ct, "application/json") {
					fail(w, 415, "Expected application/json")
					return
				}
				r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
			}
		}
		started := time.Now()
		next.ServeHTTP(w, r)
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/jobs" && r.URL.Path != "/api/status" {
			a.log.Debug("request", "method", r.Method, "path", r.URL.Path, "ms", time.Since(started).Milliseconds())
		}
	})
}

func hostAllowed(host string, allowed []string) bool {
	host = strings.ToLower(host)
	for _, h := range allowed {
		if host == h {
			return true
		}
	}
	return false
}

func originAllowed(origin string, allowed []string) bool {
	origin = strings.ToLower(origin)
	for _, h := range allowed {
		if origin == "http://"+h {
			return true
		}
	}
	return false
}

// authorized checks the per-launch session token.
func (a *application) authorized(r *http.Request) bool {
	if a.token == "" {
		return true
	}
	if c, err := r.Cookie(sessionCookie); err == nil && subtle.ConstantTimeCompare([]byte(c.Value), []byte(a.token)) == 1 {
		return true
	}
	if t := r.Header.Get(tokenHeader); t != "" && subtle.ConstantTimeCompare([]byte(t), []byte(a.token)) == 1 {
		return true
	}
	return false
}

// session exchanges the launch token in the URL for an HttpOnly cookie and
// redirects to the app, so the token never lingers in the address bar.
func (a *application) session(w http.ResponseWriter, r *http.Request) {
	t := r.URL.Query().Get("token")
	if a.token != "" && subtle.ConstantTimeCompare([]byte(t), []byte(a.token)) != 1 {
		http.Error(w, "Invalid session link. Open the address printed in the Podcast AI Studio console window.", http.StatusForbidden)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: a.token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// allowedHostsFor lists the Host header values accepted for a listener.
func allowedHostsFor(addr net.Addr) []string {
	_, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return nil
	}
	return []string{"127.0.0.1:" + port, "localhost:" + port, "[::1]:" + port}
}

// decode reads a JSON body, rejecting unknown fields.
func decode(r *http.Request, v any) error {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return errors.New("request body is too large")
		}
		if errors.Is(err, io.EOF) {
			return errors.New("request body is empty")
		}
		return fmt.Errorf("invalid request: %w", err)
	}
	return nil
}

func reply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Debug("reply encode failed", "error", err)
	}
}

func fail(w http.ResponseWriter, code int, msg string) { failCode(w, code, "", msg) }

func failCode(w http.ResponseWriter, code int, kind, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(apiError{Error: msg, Code: kind})
}
