package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/boormajor/podcast-ai-studio/internal/studio"
)

// Build information, stamped by -ldflags at release time.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// application is the single in-memory state of the local studio: one
// project, the job queue and the ChatGPT session. All fields behind mu are
// read through snapshots (Clone) so request handlers never hold the lock
// while talking to the network or encoding JSON.
type application struct {
	auth  *chatAuth
	log   *slog.Logger
	jobs  *jobManager
	tools *toolStatus

	mu       sync.Mutex
	project  studio.Project
	revision uint64 // bumped whenever the project is replaced
	stateDir string
	cacheDir string
	logPath  string
	token    string // per-launch session token for API access
}

func newApplication(stateDir string, auth *chatAuth, logger *slog.Logger) *application {
	a := &application{
		auth:     auth,
		log:      logger,
		stateDir: stateDir,
		project:  emptyProject(),
	}
	if stateDir != "" {
		a.cacheDir = filepath.Join(stateDir, "cache", "speech")
		_ = os.MkdirAll(a.cacheDir, 0o700)
		a.logPath = filepath.Join(stateDir, "studio.log")
	}
	a.jobs = newJobManager(a)
	a.tools = newToolStatus()
	return a
}

func emptyProject() studio.Project {
	return studio.Project{Name: "Untitled Podcast", Config: studio.Defaults(), Clips: []studio.Clip{}, Cues: []studio.Cue{}}
}

// snapshot returns a deep copy of the project and its revision.
func (a *application) snapshot() (studio.Project, uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.project.Clone(), a.revision
}

// replaceProject installs a new project and bumps the revision so in-flight
// AI results for the previous project are discarded.
func (a *application) replaceProject(p studio.Project) {
	a.mu.Lock()
	a.project = p
	a.project.LastUpdated = time.Now()
	a.revision++
	a.persistProjectLocked()
	a.mu.Unlock()
}

// update applies fn to the live project under the lock when the revision
// still matches. It returns false when the project was replaced meanwhile.
func (a *application) update(expected uint64, fn func(p *studio.Project)) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if expected != 0 && a.revision != expected {
		return false
	}
	fn(&a.project)
	a.project.LastUpdated = time.Now()
	a.persistProjectLocked()
	return true
}

func (a *application) projectFile() string { return filepath.Join(a.stateDir, "last-project.json") }
func (a *application) jobsFile() string    { return filepath.Join(a.stateDir, "jobs.json") }

// loadState restores the last project and job history at startup. It runs
// before the server starts so it takes no lock.
func (a *application) loadState() {
	if a.stateDir == "" {
		return
	}
	var p studio.Project
	if err := studio.ReadJSONLimited(a.projectFile(), studio.MaxProjectFileBytes, &p); err == nil {
		if err := p.Validate(); err == nil {
			a.project = p
			a.revision = 1
		} else {
			a.log.Warn("saved project ignored", "error", err)
		}
	}
	var list []*job
	if err := studio.ReadJSONLimited(a.jobsFile(), 16*1024*1024, &list); err == nil {
		a.jobs.restore(list)
	}
}

// persistProjectLocked writes the current project to disk. Call with mu held.
func (a *application) persistProjectLocked() {
	if a.stateDir == "" || a.project.VideoPath == "" {
		return
	}
	if err := studio.WriteJSONAtomic(a.projectFile(), a.project, false); err != nil {
		a.log.Error("could not save project state", "error", err)
	}
}

// persistJobs writes the job list to disk.
func (a *application) persistJobs(list []*job) {
	if a.stateDir == "" {
		return
	}
	if err := studio.WriteJSONAtomic(a.jobsFile(), list, false); err != nil {
		a.log.Error("could not save job state", "error", err)
	}
}

// shutdown cancels running work and flushes state.
func (a *application) shutdown(ctx context.Context) {
	a.jobs.cancelAll()
	a.jobs.wait(ctx)
}

// toolStatus caches PATH lookups so /api/status stays cheap.
type toolStatus struct {
	mu      sync.Mutex
	checked time.Time
	tools   map[string]bool
}

func newToolStatus() *toolStatus { return &toolStatus{tools: map[string]bool{}} }

func (t *toolStatus) get() map[string]bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if time.Since(t.checked) < 15*time.Second && len(t.tools) > 0 {
		return copyBools(t.tools)
	}
	t.tools = map[string]bool{
		"ffmpeg":   studio.HasTool("ffmpeg"),
		"ffprobe":  studio.HasTool("ffprobe"),
		"node":     studio.HasTool("node"),
		"remotion": studio.RemotionInstalled(),
		"claude":   studio.HasTool("claude"),
		"python":   studio.HasTool(qualityPython()),
	}
	t.checked = time.Now()
	return copyBools(t.tools)
}

func copyBools(m map[string]bool) map[string]bool {
	out := make(map[string]bool, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
