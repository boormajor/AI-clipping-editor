package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/boormajor/podcast-ai-studio/internal/studio"
)

// Job kinds and statuses exposed through /api/jobs.
const (
	jobRender = "render"
	jobImport = "import"

	statusQueued    = "queued"
	statusRunning   = "running"
	statusComplete  = "complete"
	statusFailed    = "failed"
	statusCancelled = "cancelled"

	maxStoredJobs = 60
)

// job is one unit of background work and its progress, as shown in the UI.
type job struct {
	ID       string     `json:"id"`
	Kind     string     `json:"kind"`
	Status   string     `json:"status"`
	ClipID   string     `json:"clipId,omitempty"`
	Title    string     `json:"title,omitempty"`
	Output   string     `json:"output,omitempty"`
	Error    string     `json:"error,omitempty"`
	Log      string     `json:"log,omitempty"`
	Renderer string     `json:"renderer,omitempty"`
	Progress int        `json:"progress"`
	Stage    string     `json:"stage,omitempty"`
	Started  time.Time  `json:"started"`
	Finished *time.Time `json:"finished,omitempty"`
}

func (j *job) active() bool { return j.Status == statusQueued || j.Status == statusRunning }

// jobManager owns the job table, cancellation and the render semaphore.
type jobManager struct {
	app     *application
	mu      sync.Mutex
	jobs    map[string]*job
	cancels map[string]context.CancelFunc
	wg      sync.WaitGroup
	renders chan struct{} // semaphore bounding concurrent renders
}

func newJobManager(app *application) *jobManager {
	return &jobManager{app: app, jobs: map[string]*job{}, cancels: map[string]context.CancelFunc{}, renders: make(chan struct{}, 1)}
}

func newJobID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format("150405.000")))[:16]
	}
	return hex.EncodeToString(b[:])
}

// restore loads persisted jobs; anything that was running when the app
// stopped is marked failed.
func (m *jobManager) restore(list []*job) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range list {
		if j == nil || j.ID == "" {
			continue
		}
		if j.active() {
			j.Status = statusFailed
			j.Error = "Interrupted by app restart"
		}
		m.jobs[j.ID] = j
	}
}

func (m *jobManager) list() []*job {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := make([]*job, 0, len(m.jobs))
	for _, j := range m.jobs {
		cp := *j
		list = append(list, &cp)
	}
	sort.Slice(list, func(i, k int) bool { return list[i].Started.After(list[k].Started) })
	return list
}

func (m *jobManager) get(id string) (job, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if !ok {
		return job{}, false
	}
	return *j, true
}

// activeFor reports whether a job of the given kind for the clip is running.
func (m *jobManager) activeFor(kind, clipID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		if j.Kind == kind && j.ClipID == clipID && j.active() {
			return true
		}
	}
	return false
}

func (m *jobManager) cancel(id string) bool {
	m.mu.Lock()
	cancel, ok := m.cancels[id]
	m.mu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

func (m *jobManager) cancelAll() {
	m.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(m.cancels))
	for _, c := range m.cancels {
		cancels = append(cancels, c)
	}
	m.mu.Unlock()
	for _, c := range cancels {
		c()
	}
}

// wait blocks until running jobs finish or ctx expires.
func (m *jobManager) wait(ctx context.Context) {
	done := make(chan struct{})
	go func() { m.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// pruneLocked drops the oldest finished jobs beyond the retention limit.
func (m *jobManager) pruneLocked() {
	if len(m.jobs) <= maxStoredJobs {
		return
	}
	var finished []*job
	for _, j := range m.jobs {
		if !j.active() {
			finished = append(finished, j)
		}
	}
	sort.Slice(finished, func(i, k int) bool { return finished[i].Started.Before(finished[k].Started) })
	for i := 0; i < len(finished) && len(m.jobs) > maxStoredJobs; i++ {
		delete(m.jobs, finished[i].ID)
	}
}

func (m *jobManager) persistLocked() {
	list := make([]*job, 0, len(m.jobs))
	for _, j := range m.jobs {
		cp := *j
		cp.Log = "" // logs live in files
		list = append(list, &cp)
	}
	m.app.persistJobs(list)
}

// start registers a job and runs fn on a goroutine. fn receives the context,
// a progress reporter and must return the output path (if any), log text and
// error.
func (m *jobManager) start(kind string, j *job, timeout time.Duration, fn func(ctx context.Context, report func(pct int, stage string)) (output, log string, err error)) {
	j.ID = newJobID()
	j.Kind = kind
	j.Status = statusQueued
	j.Started = time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	m.mu.Lock()
	m.jobs[j.ID] = j
	m.cancels[j.ID] = cancel
	m.pruneLocked()
	m.persistLocked()
	m.mu.Unlock()
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer cancel()
		report := func(pct int, stage string) {
			m.mu.Lock()
			if pct > j.Progress {
				j.Progress = pct
			}
			if stage != "" {
				j.Stage = stage
			}
			m.mu.Unlock()
		}
		if kind == jobRender {
			// Bounded concurrency: renders are heavy, so they run one at a time.
			select {
			case m.renders <- struct{}{}:
				defer func() { <-m.renders }()
			case <-ctx.Done():
				m.finish(j, "", "", ctx.Err())
				return
			}
		}
		m.mu.Lock()
		j.Status = statusRunning
		m.mu.Unlock()
		out, log, err := fn(ctx, report)
		if err != nil && ctx.Err() != nil {
			err = ctx.Err()
		}
		m.finish(j, out, log, err)
	}()
}

func (m *jobManager) finish(j *job, out, log string, err error) {
	m.writeLog(j.ID, log)
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.cancels, j.ID)
	j.Log = studio.Tail(log, 1500)
	j.Finished = &now
	switch {
	case err != nil && errors.Is(err, context.Canceled):
		j.Status = statusCancelled
		j.Error = "Cancelled"
	case err != nil && errors.Is(err, context.DeadlineExceeded):
		j.Status = statusFailed
		j.Error = "Timed out"
	case err != nil:
		j.Status = statusFailed
		j.Error = err.Error()
	default:
		j.Status = statusComplete
		j.Output = out
		j.Progress = 100
		j.Stage = "Done"
	}
	m.persistLocked()
	m.app.log.Info("job finished", "id", j.ID, "kind", j.Kind, "status", j.Status, "error", j.Error)
}

func (m *jobManager) writeLog(id, text string) {
	if m.app.stateDir == "" || text == "" {
		return
	}
	dir := filepath.Join(m.app.stateDir, "logs")
	if err := os.MkdirAll(dir, 0o700); err == nil {
		_ = os.WriteFile(filepath.Join(dir, id+".log"), []byte(text), 0o600)
	}
}
