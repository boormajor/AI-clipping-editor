package studio

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var cueTimeLine = regexp.MustCompile(`(?i)^\s*((?:\d{1,2}:)?\d{1,2}:\d{2}(?:[.,]\d{1,3})?)\s*-->\s*((?:\d{1,2}:)?\d{1,2}:\d{2}(?:[.,]\d{1,3})?)`)
var stampedLine = regexp.MustCompile(`^\s*\[?((?:\d{1,2}:)?\d{1,2}:\d{2}(?:[.,]\d{1,3})?)\]?\s*[:\-]?\s+(.+)$`)
var tags = regexp.MustCompile(`<[^>]+>`)

func parseTime(s string) (float64, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", ".")
	ps := strings.Split(s, ":")
	if len(ps) < 2 || len(ps) > 3 {
		return 0, fmt.Errorf("invalid timestamp %q", s)
	}
	v := 0.0
	for _, p := range ps {
		n, e := strconv.ParseFloat(p, 64)
		if e != nil {
			return 0, e
		}
		v = v*60 + n
	}
	return v, nil
}
func ParseTranscript(name string, data []byte) ([]Cue, string, error) {
	ext := strings.ToLower(filepath.Ext(name))
	var raw string
	switch ext {
	case ".srt", ".vtt", ".txt", ".md", ".json":
		raw = string(data)
	case ".docx":
		b, e := parseDocx(data)
		if e != nil {
			return nil, "", e
		}
		raw = b
	case ".pdf":
		return nil, "", errors.New("PDF transcripts need pdftotext. Use Load Transcript with a PDF path and install Poppler, or export SRT/VTT from Descript")
	default:
		return nil, "", fmt.Errorf("unsupported transcript type %q", ext)
	}
	// JSON from Whisper or word-caption exports: seek segments arrays.
	if ext == ".json" {
		if cues, ok := parseJSONTranscript(data); ok {
			return cues, "timed", nil
		}
	}
	raw = strings.TrimPrefix(raw, "\ufeff")
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	var cues []Cue
	lines := strings.Split(raw, "\n")
	for i := 0; i < len(lines); i++ {
		m := cueTimeLine.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		start, _ := parseTime(m[1])
		end, _ := parseTime(m[2])
		if end <= start {
			continue
		}
		var b []string
		for j := i + 1; j < len(lines); j++ {
			l := strings.TrimSpace(lines[j])
			if l == "" || cueTimeLine.MatchString(l) {
				break
			}
			// Strip speaker numbering lines only when followed by a time line, elsewhere keep.
			b = append(b, strings.TrimSpace(tags.ReplaceAllString(l, "")))
		}
		t := strings.TrimSpace(strings.Join(b, " "))
		if t != "" {
			cues = append(cues, Cue{start, end, t})
		}
	}
	if len(cues) > 0 {
		return normalizeCues(cues), "timed", nil
	}
	for _, l := range lines {
		m := stampedLine.FindStringSubmatch(l)
		if m != nil {
			t, _ := parseTime(m[1])
			cues = append(cues, Cue{Start: t, Text: strings.TrimSpace(m[2])})
		}
	}
	if len(cues) > 1 {
		sort.Slice(cues, func(i, j int) bool { return cues[i].Start < cues[j].Start })
		for i := range cues {
			if i+1 < len(cues) {
				cues[i].End = cues[i+1].Start
			} else {
				cues[i].End = cues[i].Start + 5
			}
		}
		return normalizeCues(cues), "timed", nil
	}
	// Untimed transcript is returned in `raw` but cannot be turned into guessed cuts.
	return nil, strings.TrimSpace(raw), nil
}
func normalizeCues(c []Cue) []Cue {
	sort.Slice(c, func(i, j int) bool { return c[i].Start < c[j].Start })
	out := make([]Cue, 0, len(c))
	for _, a := range c {
		// Normalize HTML-escaped transcripts (including repeated &gt;&gt; speaker markers).
		a.Text = strings.TrimSpace(html.UnescapeString(html.UnescapeString(a.Text)))
		a.Text = strings.TrimSpace(strings.TrimLeft(strings.TrimPrefix(a.Text, ">>"), " >"))
		if a.Start >= 0 && a.End > a.Start && a.Text != "" {
			out = append(out, a)
		}
	}
	return out
}

// MaxTranscriptBytes caps transcript files read from disk.
const MaxTranscriptBytes = 32 * 1024 * 1024

// LoadTranscript reads a transcript file. The second result is "timed" when
// cues carry timestamps, otherwise the plain text that was found.
func LoadTranscript(ctx context.Context, path string) ([]Cue, string, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".pdf" {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		out, e := DefaultTool.Output(ctx, "pdftotext", "-layout", path, "-")
		if e != nil {
			return nil, "", fmt.Errorf("PDF extraction requires pdftotext (Poppler): %w", e)
		}
		return ParseTranscript("extracted.txt", []byte(out))
	}
	info, e := os.Stat(path)
	if e != nil {
		return nil, "", e
	}
	if info.Size() > MaxTranscriptBytes {
		return nil, "", errors.New("transcript file is larger than 32 MB")
	}
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, "", e
	}
	return ParseTranscript(path, b)
}
func parseDocx(data []byte) (string, error) {
	r, e := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if e != nil {
		return "", e
	}
	for _, f := range r.File {
		if f.Name != "word/document.xml" {
			continue
		}
		reader, e := f.Open()
		if e != nil {
			return "", e
		}
		defer reader.Close()
		dec := xml.NewDecoder(reader)
		var b strings.Builder
		for {
			tok, e := dec.Token()
			if e != nil {
				break
			}
			switch t := tok.(type) {
			case xml.StartElement:
				if t.Name.Local == "p" {
					b.WriteString("\n")
				}
			case xml.CharData:
				b.WriteString(string(t))
			}
		}
		return b.String(), nil
	}
	return "", errors.New("DOCX is missing word/document.xml")
}
