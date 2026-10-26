package studio

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// Tool runs external commands (ffmpeg, ffprobe, node, python) with the
// safeguards every call needs: a context deadline, a bounded wait after
// cancellation, and termination of the whole process tree so a cancelled
// render does not leave Chrome or ffmpeg running in the background.
//
// The zero value is ready to use. Tests may set Lookup and Run to fake the
// commands they would otherwise launch.
type Tool struct {
	// WaitDelay bounds how long Wait blocks after the context is cancelled.
	WaitDelay time.Duration
}

// DefaultTool is used by package-level helpers.
var DefaultTool = &Tool{WaitDelay: 5 * time.Second}

// Command builds an exec.Cmd bound to ctx with process-group handling so the
// entire tree is killed on cancellation.
func (t *Tool) Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	delay := t.WaitDelay
	if delay <= 0 {
		delay = 5 * time.Second
	}
	cmd.WaitDelay = delay
	configureProcessGroup(cmd)
	return cmd
}

// Run executes a command and returns its combined output. Output is capped at
// maxOutput bytes, keeping the tail where tools put their actual error.
func (t *Tool) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := t.Command(ctx, name, args...)
	var buf tailBuffer
	buf.limit = maxOutput
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	out := buf.String()
	if err != nil {
		if ctx.Err() != nil {
			return out, fmt.Errorf("%s was cancelled: %w", name, ctx.Err())
		}
		return out, fmt.Errorf("%s failed: %w: %s", name, err, Tail(out, 400))
	}
	return out, nil
}

// Output runs a command and returns only stdout, with stderr folded into the
// error message.
func (t *Tool) Output(ctx context.Context, name string, args ...string) (string, error) {
	cmd := t.Command(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = io.Writer(&tailBuffer{limit: 4096, buf: &stderr})
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("%s was cancelled: %w", name, ctx.Err())
		}
		return "", fmt.Errorf("%s failed: %w: %s", name, err, Tail(stderr.String(), 400))
	}
	return stdout.String(), nil
}

const maxOutput = 256 * 1024

// tailBuffer keeps the last `limit` bytes written to it.
type tailBuffer struct {
	limit int
	buf   *bytes.Buffer
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	if b.buf == nil {
		b.buf = &bytes.Buffer{}
	}
	b.buf.Write(p)
	if b.limit > 0 && b.buf.Len() > b.limit {
		excess := b.buf.Len() - b.limit
		b.buf.Next(excess)
	}
	return len(p), nil
}

func (b *tailBuffer) String() string {
	if b.buf == nil {
		return ""
	}
	return b.buf.String()
}

// Tail returns the last n runes of s, prefixed with an ellipsis when cut.
func Tail(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return "…" + string(r[len(r)-n:])
}

// Head returns the first n runes of s, suffixed with an ellipsis when cut.
func Head(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// HasTool reports whether an executable is on PATH.
func HasTool(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
