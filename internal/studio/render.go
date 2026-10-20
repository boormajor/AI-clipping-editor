package studio

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

func Probe(video string) (float64, error) {
	out, e := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", video).Output()
	if e != nil {
		return 0, fmt.Errorf("ffprobe failed: %w", e)
	}
	n, e := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	return n, e
}
func AssTime(s float64) string {
	if s < 0 {
		s = 0
	}
	ms := int(s*100 + 0.5)
	return fmt.Sprintf("%d:%02d:%02d.%02d", ms/360000, (ms/6000)%60, (ms/100)%60, ms%100)
}
func EscapeAss(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "{", "\\{")
	s = strings.ReplaceAll(s, "}", "\\}")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// validCaptionFont limits unsafe or unsupported characters in ASS style definitions.
func validCaptionFont(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "Arial"
	}
	if len(s) > 70 {
		s = s[:70]
	}
	for _, r := range s {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || r == '-') {
			return "Arial"
		}
	}
	return s
}

// WriteASS keeps original cue boundaries. It never invents word timing from SRT.
// Word-level animations require a separate real word-alignment pass.
func WriteASS(path string, cues []Cue, c Clip, config Config) error {
	fontSize := config.CaptionSize
	if fontSize < 24 || fontSize > 130 {
		fontSize = 76
	}
	color := "&H00FFFFFF"
	outline := 4
	switch c.CaptionStyle {
	case "minimal":
		outline = 2
	case "yellow", "highlight", "karaoke":
		color = "&H0000EFFF"
	case "podcast":
		outline = 3
	}
	font := validCaptionFont(config.CaptionFont)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("[Script Info]\nScriptType: v4.00+\nPlayResX: %d\nPlayResY: %d\nWrapStyle: 0\n[V4+ Styles]\nFormat: Name,Fontname,Fontsize,PrimaryColour,SecondaryColour,OutlineColour,BackColour,Bold,Italic,Underline,StrikeOut,ScaleX,ScaleY,Spacing,Angle,BorderStyle,Outline,Shadow,Alignment,MarginL,MarginR,MarginV,Encoding\nStyle: Default,%s,%d,%s,&H00FFFFFF,&H00000000,&H99000000,-1,0,0,0,100,100,0,0,1,%d,1,2,64,64,240,1\n[Events]\nFormat: Layer,Start,End,Style,Name,MarginL,MarginR,MarginV,Effect,Text\n", config.Width, config.Height, font, fontSize, color, outline))
	for _, cue := range cues {
		// Positive offsets delay captions; negative offsets move them earlier.
		start := cue.Start + config.CaptionOffset - c.Start
		end := cue.End + config.CaptionOffset - c.Start
		if end <= 0 || start >= c.End-c.Start {
			continue
		}
		if start < 0 {
			start = 0
		}
		if end > c.End-c.Start {
			end = c.End - c.Start
		}
		if end <= start {
			continue
		}
		b.WriteString(fmt.Sprintf("Dialogue: 0,%s,%s,Default,,0,0,0,,%s\n", AssTime(start), AssTime(end), EscapeAss(cue.Text)))
	}
	return os.WriteFile(path, []byte(b.String()), 0600)
}

// videoChain builds a filter on a *single finished video*.
// two-up is explicit because it assumes the guests occupy the left and right halves.
func videoChain(cfg Config, c Clip) string {
	prefix := "fps=" + strconv.Itoa(cfg.FPS)
	if cfg.Reframe && c.Layout == "two-up" {
		h := cfg.Height / 2
		return fmt.Sprintf("%s,split=2[sideL][sideR];[sideL]crop=iw/2:ih:0:0,scale=w=%d:h=%d:force_original_aspect_ratio=increase,crop=%d:%d[top];[sideR]crop=iw/2:ih:iw/2:0,scale=w=%d:h=%d:force_original_aspect_ratio=increase,crop=%d:%d[bottom];[top][bottom]vstack=inputs=2,setsar=1", prefix, cfg.Width, h, cfg.Width, h, cfg.Width, h, cfg.Width, h)
	}
	if cfg.Reframe {
		pan := c.Pan
		if pan < 0 {
			pan = 0
		}
		if pan > 1 {
			pan = 1
		}
		return fmt.Sprintf("%s,scale=w=%d:h=%d:force_original_aspect_ratio=increase,crop=%d:%d:(iw-%d)*%.4f:(ih-%d)/2,setsar=1", prefix, cfg.Width, cfg.Height, cfg.Width, cfg.Height, cfg.Width, pan, cfg.Height)
	}
	return fmt.Sprintf("%s,scale=w=%d:h=%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black,setsar=1", prefix, cfg.Width, cfg.Height, cfg.Width, cfg.Height)
}
func Render(ctx context.Context, p Project, c Clip) (string, string, error) {
	if c.End <= c.Start || c.Start < 0 || c.End > p.Duration+.5 {
		return "", "", fmt.Errorf("clip has invalid time bounds")
	}
	if p.VideoPath == "" {
		return "", "", fmt.Errorf("no podcast video selected")
	}
	cfg := p.Config
	if cfg.Width < 240 || cfg.Height < 240 || cfg.Width > 3840 || cfg.Height > 3840 {
		return "", "", fmt.Errorf("invalid export dimensions")
	}
	if cfg.FPS != 24 && cfg.FPS != 25 && cfg.FPS != 30 && cfg.FPS != 60 {
		return "", "", fmt.Errorf("FPS must be 24, 25, 30, or 60")
	}
	target := p.OutputDir
	if target == "" {
		target = filepath.Join(filepath.Dir(p.VideoPath), "PodcastAI-Exports")
	}
	if e := os.MkdirAll(target, 0755); e != nil {
		return "", "", e
	}
	safe := strings.Trim(strings.Map(func(r rune) rune {
		if r == '-' || r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, c.ID), "-")
	if safe == "" {
		safe = "clip"
	}
	temp, e := os.MkdirTemp("", "podcast-studio-render-")
	if e != nil {
		return "", "", e
	}
	defer os.RemoveAll(temp)
	ass := filepath.Join(temp, "captions.ass")
	if e = WriteASS(ass, p.Cues, c, cfg); e != nil {
		return "", "", e
	}
	vf := videoChain(cfg, c)
	if cfg.Zoom && c.Layout != "two-up" {
		vf += fmt.Sprintf(",crop=w='iw/(1+0.06*between(t,1.5,3.5))':h='ih/(1+0.06*between(t,1.5,3.5))':x='(iw-ow)/2':y='(ih-oh)/2',scale=%d:%d", cfg.Width, cfg.Height)
	}
	var finalFilters []string
	if cfg.Captions && c.Captions {
		finalFilters = append(finalFilters, "ass=captions.ass")
	}
	// Text callouts are escaped using a textfile in a temporary working directory.
	if cfg.Graphics && strings.TrimSpace(c.Callout) != "" {
		if e = os.WriteFile(filepath.Join(temp, "callout.txt"), []byte(c.Callout), 0600); e != nil {
			return "", "", e
		}
		finalFilters = append(finalFilters, "drawbox=x=0:y=120:w=iw:h=145:color=black@0.60:t=fill:enable='between(t,0,3.5)',drawtext=font=Arial:textfile=callout.txt:fontsize=56:fontcolor=white:x=(w-text_w)/2:y=145:enable='between(t,0,3.5)'")
	}
	// Keep B-roll on its own independent video layer; don't replace source dialogue audio.
	args := []string{"-hide_banner", "-y", "-ss", fmt.Sprintf("%.3f", c.Start), "-t", fmt.Sprintf("%.3f", c.End-c.Start), "-i", p.VideoPath}
	var overlays []Broll
	if cfg.Broll {
		for _, a := range c.Broll {
			if a.Path != "" && a.Duration > 0 {
				if _, e := os.Stat(a.Path); e == nil {
					overlays = append(overlays, a)
				}
			}
		}
	}
	sort.Slice(overlays, func(i, j int) bool { return overlays[i].Start < overlays[j].Start })
	for _, a := range overlays {
		args = append(args, "-stream_loop", "-1", "-i", a.Path)
	}
	mapVideo := "0:v:0"
	if len(overlays) == 0 {
		if len(finalFilters) > 0 {
			vf += "," + strings.Join(finalFilters, ",")
		}
		args = append(args, "-vf", vf)
	} else {
		graph := "[0:v]" + vf + "[v0];"
		previous := "v0"
		for i, a := range overlays {
			out := fmt.Sprintf("v%d", i+1)
			if a.Start < 0 {
				a.Start = 0
			}
			graph += fmt.Sprintf("[%d:v]fps=%d,scale=w=%d:h=%d:force_original_aspect_ratio=increase,crop=%d:%d,setsar=1[b%d];", i+1, cfg.FPS, cfg.Width, cfg.Height, cfg.Width, cfg.Height, i)
			graph += fmt.Sprintf("[%s][b%d]overlay=0:0:enable='between(t,%.3f,%.3f)':eof_action=pass[%s];", previous, i, a.Start, a.Start+a.Duration, out)
			previous = out
		}
		if len(finalFilters) > 0 {
			graph += "[" + previous + "]" + strings.Join(finalFilters, ",") + "[final];"
			previous = "final"
		}
		graph = strings.TrimSuffix(graph, ";")
		args = append(args, "-filter_complex", graph, "-map", "["+previous+"]")
		mapVideo = ""
	}
	if mapVideo != "" {
		args = append(args, "-map", mapVideo)
	}
	args = append(args, "-map", "0:a:0?", "-c:a", "aac", "-b:a", "160k", "-pix_fmt", "yuv420p", "-movflags", "+faststart")
	filename := filepath.Join(target, safe+".mp4")
	// Try GPU encoding; fallback to software on unsupported drivers/encoders.
	encoders := []string{"libx264"}
	if cfg.GPU {
		encoders = []string{"h264_nvenc", "libx264"}
	}
	var stderr string
	for _, encoder := range encoders {
		opts := []string{"-c:v", encoder}
		if encoder == "h264_nvenc" {
			opts = append(opts, "-preset", "p4", "-cq", "23")
		} else {
			opts = append(opts, "-preset", "medium", "-crf", "22")
		}
		command := exec.CommandContext(ctx, "ffmpeg", append(append(append([]string{}, args...), opts...), filename)...)
		command.Dir = temp
		var errbuf bytes.Buffer
		command.Stderr = &errbuf
		e = command.Run()
		stderr = errbuf.String()
		if e == nil {
			return filename, stderr, nil
		}
	}
	return "", stderr, fmt.Errorf("FFmpeg render failed: %w: %s", e, truncate(stderr, 1200))
}
func RenderTimeout() time.Duration { return 30 * time.Minute }
