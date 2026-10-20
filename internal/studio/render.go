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
func WriteASS(path string, cues []Cue, c Clip, config Config) error {
	fontSize := 76
	color := "&H00FFFFFF"
	outline := 4
	alignment := 2
	switch c.CaptionStyle {
	case "minimal":
		fontSize = 58
		outline = 2
	case "yellow":
		color = "&H0000EFFF"
	case "podcast":
		fontSize = 68
	case "highlight":
		color = "&H0000FFFF"
	case "karaoke":
		color = "&H0000FFFF"
	}
	// Uses ASS subtitle styling with per-cue event timings from the imported transcript.
	var b strings.Builder
	b.WriteString(fmt.Sprintf("[Script Info]\nScriptType: v4.00+\nPlayResX: %d\nPlayResY: %d\nWrapStyle: 2\n[V4+ Styles]\nFormat: Name,Fontname,Fontsize,PrimaryColour,SecondaryColour,OutlineColour,BackColour,Bold,Italic,Underline,StrikeOut,ScaleX,ScaleY,Spacing,Angle,BorderStyle,Outline,Shadow,Alignment,MarginL,MarginR,MarginV,Encoding\nStyle: Default,Arial,%d,%s,&H00FFFFFF,&H00000000,&HAA000000,-1,0,0,0,100,100,0,0,1,%d,1,%d,68,68,190,1\n[Events]\nFormat: Layer,Start,End,Style,Name,MarginL,MarginR,MarginV,Effect,Text\n", config.Width, config.Height, fontSize, color, outline, alignment))
	for _, cue := range cues {
		if cue.End <= c.Start || cue.Start >= c.End {
			continue
		}
		start := cue.Start - c.Start
		if start < 0 {
			start = 0
		}
		end := cue.End - c.Start
		if end > c.End-c.Start {
			end = c.End - c.Start
		}
		if end <= start {
			continue
		}
		words := strings.Fields(cue.Text)
		// Split long segments into short readable caption chunks, using proportional timing if word timings aren't available.
		for i := 0; i < len(words); i += 7 {
			j := i + 7
			if j > len(words) {
				j = len(words)
			}
			a := start + (end-start)*float64(i)/float64(len(words))
			z := start + (end-start)*float64(j)/float64(len(words))
			b.WriteString(fmt.Sprintf("Dialogue: 0,%s,%s,Default,,0,0,0,,%s\n", AssTime(a), AssTime(z), EscapeAss(strings.Join(words[i:j], " "))))
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0600)
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
	var f []string
	// Pan is horizontal focal position, range 0..1. Keep center of interest in the vertical crop.
	pan := c.Pan
	if pan < 0 {
		pan = 0
	}
	if pan > 1 {
		pan = 1
	}
	f = append(f, "fps="+strconv.Itoa(cfg.FPS))
	if cfg.Reframe {
		scale := fmt.Sprintf("scale=w=%d:h=%d:force_original_aspect_ratio=increase", cfg.Width, cfg.Height)
		crop := fmt.Sprintf("crop=%d:%d:(iw-%d)*%.4f:(ih-%d)/2", cfg.Width, cfg.Height, cfg.Width, pan, cfg.Height)
		f = append(f, scale, crop, "setsar=1")
	} else {
		f = append(f, fmt.Sprintf("scale=w=%d:h=%d:force_original_aspect_ratio=decrease", cfg.Width, cfg.Height), fmt.Sprintf("pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black", cfg.Width, cfg.Height), "setsar=1")
	}
	if cfg.Zoom {
		f = append(f, fmt.Sprintf("crop=w='iw/(1+0.06*between(t,1.5,3.5))':h='ih/(1+0.06*between(t,1.5,3.5))':x='(iw-ow)/2':y='(ih-oh)/2',scale=%d:%d", cfg.Width, cfg.Height))
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
	vf := strings.Join(f, ",")
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
