package studio

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// ExportsFolderName is created beside the source video when no export folder
// is chosen.
const ExportsFolderName = "PodcastAI-Exports"

var clipIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// IsClipID reports whether s is a safe clip identifier (used in file names).
func IsClipID(s string) bool { return clipIDPattern.MatchString(s) }

// ClipID formats the canonical identifier for the n-th clip (1-based).
func ClipID(n int) string { return fmt.Sprintf("clip-%02d", n) }

// ExportDir returns the folder renders and project files are written to.
func (p Project) ExportDir() string {
	if p.OutputDir != "" {
		return p.OutputDir
	}
	return filepath.Join(filepath.Dir(p.VideoPath), ExportsFolderName)
}

// Slug turns free text into a short, file-name-safe token.
func Slug(s string, max int) string {
	var b strings.Builder
	lastDash := true
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if r < 128 {
				b.WriteRune(r)
				lastDash = false
			}
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
		if b.Len() >= max {
			break
		}
	}
	return strings.Trim(b.String(), "-")
}

// OutputFileName builds a unique, readable render file name.
func OutputFileName(c Clip, jobID string) string {
	name := c.ID
	if slug := Slug(c.Title, 40); slug != "" {
		name += "-" + slug
	}
	if len(jobID) >= 6 {
		name += "-" + jobID[:6]
	}
	return name + ".mp4"
}

// AbsRegularFile cleans a path and checks it points at a regular file.
func AbsRegularFile(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("path is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("cannot read %s", abs)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a file", abs)
	}
	return abs, nil
}

// AbsDir cleans a directory path, creating it when missing.
func AbsDir(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("folder is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", fmt.Errorf("cannot create folder %s: %w", abs, err)
	}
	return abs, nil
}

// WriteFileAtomic writes data to a temporary file and renames it into place
// so a crash never leaves a half-written file behind.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	cleanup := func() { _ = os.Remove(name) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Chmod(name, perm); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(name, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

// WriteJSONAtomic marshals v and writes it atomically.
func WriteJSONAtomic(path string, v any, indent bool) error {
	var b []byte
	var err error
	if indent {
		b, err = json.MarshalIndent(v, "", "  ")
	} else {
		b, err = json.Marshal(v)
	}
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, b, 0o600)
}

// ReadJSONLimited decodes a JSON file no larger than limit bytes.
func ReadJSONLimited(path string, limit int64, v any) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() > limit {
		return fmt.Errorf("%s is larger than %d MB", filepath.Base(path), limit/(1024*1024))
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
