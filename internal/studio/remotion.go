package studio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RenderTimeout bounds one render job end to end.
const RenderTimeout = 45 * time.Minute

// remotionRootOverride is set from the command line for development runs.
var remotionRootOverride string

// SetRemotionRoot points the renderer at a specific Remotion project folder.
func SetRemotionRoot(path string) { remotionRootOverride = path }

// lineWriter buffers process output and invokes fn for each complete line.
// Safe for the concurrent stdout/stderr writers exec may use.
type lineWriter struct {
	mu      sync.Mutex
	all     tailBuffer
	partial []byte
	fn      func(string)
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, _ = w.all.Write(p)
	w.partial = append(w.partial, p...)
	for {
		i := bytes.IndexAny(w.partial, "\r\n")
		if i < 0 {
			break
		}
		line := string(w.partial[:i])
		w.partial = w.partial[i+1:]
		if w.fn != nil && line != "" {
			w.fn(line)
		}
	}
	return len(p), nil
}

func (w *lineWriter) String() string { w.mu.Lock(); defer w.mu.Unlock(); return w.all.String() }

var (
	reRendered = regexp.MustCompile(`Rendered\s+(\d+)/(\d+)`)
	reEncoded  = regexp.MustCompile(`Encoded\s+(\d+)/(\d+)`)
)

type remotionAsset struct {
	Media    string  `json:"media"`
	Start    float64 `json:"start"`
	Duration float64 `json:"duration"`
	Mode     string  `json:"mode"`
}

// remotionProps mirrors remotion/src/schema.ts.
type remotionProps struct {
	Media           string          `json:"media"`
	Duration        float64         `json:"duration"`
	Width           int             `json:"width"`
	Height          int             `json:"height"`
	FPS             int             `json:"fps"`
	Layout          string          `json:"layout"`
	Pan             float64         `json:"pan"`
	Shots           []Shot          `json:"shots"`
	Cues            []Cue           `json:"cues"`
	Words           []Word          `json:"words"`
	Assets          []remotionAsset `json:"assets"`
	Captions        bool            `json:"captions"`
	CaptionPreset   string          `json:"captionPreset"`
	CaptionFont     string          `json:"captionFont"`
	CaptionSize     int             `json:"captionSize"`
	CaptionColor    string          `json:"captionColor"`
	AccentColor     string          `json:"accentColor"`
	CaptionOffset   float64         `json:"captionOffset"`
	CaptionWords    int             `json:"captionWords"`
	CaptionPosition float64         `json:"captionPosition"`
	Title           string          `json:"title"`
	Callout         string          `json:"callout"`
	Graphics        bool            `json:"graphics"`
	Zoom            bool            `json:"zoom"`
	Emphasis        bool            `json:"emphasis"`
	ProgressBar     bool            `json:"progressBar"`
}

// FindRemotion locates the bundled Remotion project. Only trusted locations
// are searched: an explicit override, the PODCAST_STUDIO_REMOTION variable and
// the folder next to the executable. The working directory is deliberately
// not consulted so a planted folder cannot hijack a render.
func FindRemotion() (string, error) {
	roots := []string{}
	if remotionRootOverride != "" {
		roots = append(roots, remotionRootOverride)
	}
	if env := os.Getenv("PODCAST_STUDIO_REMOTION"); env != "" {
		roots = append(roots, env)
	}
	if exe, e := os.Executable(); e == nil {
		if real, e := filepath.EvalSymlinks(exe); e == nil {
			exe = real
		}
		roots = append(roots, filepath.Join(filepath.Dir(exe), "remotion"))
	}
	for _, root := range roots {
		if _, e := os.Stat(filepath.Join(root, "package.json")); e == nil {
			abs, _ := filepath.Abs(root)
			return abs, nil
		}
	}
	return "", errors.New("the Remotion folder is missing; extract the entire Windows bundle or pass -remotion")
}

// RemotionCLI returns the path of the Remotion CLI script, which is run with
// node directly so no .cmd shim is involved in the launch.
func RemotionCLI(root string) (string, error) {
	cli := filepath.Join(root, "node_modules", "@remotion", "cli", "remotion-cli.js")
	if _, e := os.Stat(cli); e != nil {
		return "", errors.New("Remotion is not installed; run Install-and-Run.bat (or npm ci inside the remotion folder)")
	}
	return cli, nil
}

// RemotionInstalled reports whether the Remotion CLI and Node are available.
func RemotionInstalled() bool {
	root, e := FindRemotion()
	if e != nil {
		return false
	}
	_, e = RemotionCLI(root)
	return e == nil && HasTool("node")
}

// CleanStaleJobs removes leftover staging folders from interrupted renders.
func CleanStaleJobs() {
	root, e := FindRemotion()
	if e != nil {
		return
	}
	entries, e := os.ReadDir(filepath.Join(root, "public", "jobs"))
	if e != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "clip-") {
			_ = os.RemoveAll(filepath.Join(root, "public", "jobs", entry.Name()))
		}
	}
}

func makeRemotionProps(p Project, c Clip) (remotionProps, error) {
	cfg := p.Config
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return remotionProps{}, err
	}
	if err := c.Validate(p.Duration); err != nil {
		return remotionProps{}, err
	}
	dur := c.End - c.Start
	x := remotionProps{
		Duration: dur, Width: cfg.Width, Height: cfg.Height, FPS: cfg.FPS,
		Layout: c.Layout, Pan: c.Pan, Shots: c.Shots, Cues: []Cue{}, Words: c.Words, Assets: []remotionAsset{},
		Captions: cfg.Captions && c.Captions, CaptionPreset: c.CaptionStyle, CaptionFont: cfg.CaptionFont,
		CaptionSize: cfg.CaptionSize, CaptionColor: cfg.CaptionColor, AccentColor: cfg.AccentColor,
		CaptionOffset: cfg.CaptionOffset, CaptionWords: cfg.CaptionWords, CaptionPosition: cfg.CaptionPosition,
		Title: c.Title, Callout: c.Callout, Graphics: cfg.Graphics, Zoom: cfg.Zoom, Emphasis: cfg.CaptionEmphasis,
		ProgressBar: cfg.ProgressBar,
	}
	if x.Shots == nil {
		x.Shots = []Shot{}
	}
	if x.Words == nil {
		x.Words = []Word{}
	}
	if !IsCaptionPreset(x.CaptionPreset) {
		x.CaptionPreset = cfg.CaptionStyle
	}
	if !IsLayout(x.Layout) {
		x.Layout = "focus"
	}
	if !cfg.Reframe {
		x.Layout = "focus"
		x.Pan = 0.5
		x.Shots = []Shot{}
	}
	for _, cue := range p.Cues {
		start, end := cue.Start-c.Start, cue.End-c.Start
		if end <= 0 || start >= dur {
			continue
		}
		start = max(start, 0)
		end = min(end, dur)
		if end > start {
			x.Cues = append(x.Cues, Cue{Start: start, End: end, Text: cue.Text})
		}
	}
	return x, nil
}

// RenderRemotion renders one clip with the bundled Remotion project. It
// returns the output path and the tail of the renderer log.
func RenderRemotion(ctx context.Context, p Project, c Clip, jobID string, onProgress func(int)) (string, string, error) {
	root, e := FindRemotion()
	if e != nil {
		return "", "", e
	}
	cli, e := RemotionCLI(root)
	if e != nil {
		return "", "", e
	}
	if !HasTool("node") {
		return "", "", errors.New("Node.js is required for Remotion renders; run Install-and-Run.bat")
	}
	props, e := makeRemotionProps(p, c)
	if e != nil {
		return "", "", e
	}
	folder, e := AbsDir(p.ExportDir())
	if e != nil {
		return "", "", e
	}
	jobs := filepath.Join(root, "public", "jobs")
	if e = os.MkdirAll(jobs, 0o755); e != nil {
		return "", "", e
	}
	job, e := os.MkdirTemp(jobs, "clip-")
	if e != nil {
		return "", "", e
	}
	defer os.RemoveAll(job)
	id := filepath.Base(job)
	report := func(pct int) {
		if onProgress != nil {
			onProgress(pct)
		}
	}
	report(2)

	src := filepath.Join(job, "source.mp4")
	if out, err := DefaultTool.Run(ctx, "ffmpeg", "-hide_banner", "-nostdin", "-loglevel", "error", "-y",
		"-ss", strconv.FormatFloat(c.Start, 'f', 3, 64), "-i", p.VideoPath,
		"-t", strconv.FormatFloat(c.End-c.Start, 'f', 3, 64),
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "18", "-c:a", "aac", "-movflags", "+faststart", src); err != nil {
		return "", out, fmt.Errorf("preparing source clip failed: %w", err)
	}
	props.Media = "jobs/" + id + "/source.mp4"
	if p.Config.Broll {
		for i, asset := range c.Broll {
			path, err := AbsRegularFile(asset.Path)
			if err != nil {
				return "", "", fmt.Errorf("B-roll %d: %w", i+1, err)
			}
			ext := strings.ToLower(filepath.Ext(path))
			name := fmt.Sprintf("asset-%d.mp4", i)
			if ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".webp" {
				name = fmt.Sprintf("asset-%d%s", i, ext)
				data, err := os.ReadFile(path)
				if err != nil {
					return "", "", err
				}
				if err := os.WriteFile(filepath.Join(job, name), data, 0o600); err != nil {
					return "", "", err
				}
			} else {
				if out, err := DefaultTool.Run(ctx, "ffmpeg", "-hide_banner", "-nostdin", "-loglevel", "error", "-y",
					"-i", path, "-t", strconv.FormatFloat(asset.Duration, 'f', 3, 64), "-an",
					"-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-pix_fmt", "yuv420p", filepath.Join(job, name)); err != nil {
					return "", out, fmt.Errorf("B-roll preparation failed: %w", err)
				}
			}
			mode := asset.Mode
			if mode == "" {
				mode = "cover"
			}
			props.Assets = append(props.Assets, remotionAsset{Media: "jobs/" + id + "/" + name, Start: asset.Start, Duration: asset.Duration, Mode: mode})
		}
	}
	report(5)

	out := filepath.Join(folder, OutputFileName(c, jobID))
	var logs strings.Builder
	for attempt := 0; attempt < 2; attempt++ {
		log, err := runRemotion(ctx, root, cli, job, props, out, report)
		logs.WriteString(log)
		if err == nil {
			if _, err := os.Stat(out); err != nil {
				return "", logs.String(), errors.New("Remotion reported success but produced no file")
			}
			report(100)
			return out, logs.String(), nil
		}
		if ctx.Err() != nil {
			return "", logs.String(), fmt.Errorf("render cancelled: %w", ctx.Err())
		}
		// A caption font that cannot be downloaded (offline machine, blocked
		// CDN) stalls the composition. Retry once with a system font rather
		// than abandoning Remotion for this clip.
		if attempt == 0 && isFontFetchFailure(log) && props.CaptionFont != "Arial" {
			logs.WriteString("\n[studio] caption font " + props.CaptionFont + " could not be downloaded; retrying with Arial\n")
			props.CaptionFont = "Arial"
			continue
		}
		return "", logs.String(), fmt.Errorf("Remotion render failed: %w", err)
	}
	return "", logs.String(), errors.New("Remotion render failed")
}

func isFontFetchFailure(log string) bool {
	l := strings.ToLower(log)
	return strings.Contains(l, "font") && (strings.Contains(l, "err_name_not_resolved") || strings.Contains(l, "err_internet_disconnected") || strings.Contains(l, "delayrender") || strings.Contains(l, "fetching"))
}

// runRemotion writes the props file and executes one render attempt.
func runRemotion(ctx context.Context, root, cli, job string, props remotionProps, out string, report func(int)) (string, error) {
	b, e := json.Marshal(props)
	if e != nil {
		return "", e
	}
	propFile := filepath.Join(job, "props.json")
	if e = os.WriteFile(propFile, b, 0o600); e != nil {
		return "", e
	}
	args := []string{cli, "render", "src/index.ts", "PodcastClip", out, "--props", propFile, "--codec", "h264", "--log", "info", "--overwrite", "--timeout", "60000"}
	if browser := os.Getenv("PODCAST_STUDIO_BROWSER"); browser != "" {
		args = append(args, "--browser-executable", browser)
	}
	render := DefaultTool.Command(ctx, "node", args...)
	render.Dir = root
	render.Env = append(os.Environ(), "CI=1")
	lw := &lineWriter{all: tailBuffer{limit: maxOutput}, fn: func(line string) {
		// Render phase maps to 5-85%, encode phase to 85-100%.
		if m := reRendered.FindStringSubmatch(line); m != nil {
			done, _ := strconv.Atoi(m[1])
			total, _ := strconv.Atoi(m[2])
			if total > 0 {
				report(5 + done*80/total)
			}
		} else if m := reEncoded.FindStringSubmatch(line); m != nil {
			done, _ := strconv.Atoi(m[1])
			total, _ := strconv.Atoi(m[2])
			if total > 0 {
				report(85 + done*14/total)
			}
		}
	}}
	render.Stdout = lw
	render.Stderr = lw
	err := render.Run()
	return lw.String(), err
}
