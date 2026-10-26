package studio

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Probe returns the duration of a media file in seconds.
func Probe(ctx context.Context, video string) (float64, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	out, err := DefaultTool.Output(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", video)
	if err != nil {
		return 0, fmt.Errorf("ffprobe could not read the video: %w", err)
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(out), 64)
	if err != nil || n <= 0 {
		return 0, errors.New("ffprobe did not report a duration; the file may be corrupt or not a video")
	}
	return n, nil
}

// AssTime formats seconds as an ASS timestamp (h:mm:ss.cc).
func AssTime(s float64) string {
	if s < 0 {
		s = 0
	}
	ms := int(s*100 + 0.5)
	return fmt.Sprintf("%d:%02d:%02d.%02d", ms/360000, (ms/6000)%60, (ms/100)%60, ms%100)
}

// EscapeAss escapes text for an ASS dialogue line.
func EscapeAss(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "{", "\\{")
	s = strings.ReplaceAll(s, "}", "\\}")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// validCaptionFont limits unsafe or unsupported characters in ASS style
// definitions.
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

func assColor(hex string) string {
	// #RRGGBB -> &H00BBGGRR
	if !IsHexColor(hex) {
		return "&H00FFFFFF"
	}
	return "&H00" + strings.ToUpper(hex[5:7]+hex[3:5]+hex[1:3])
}

// WriteASS writes burned-in subtitles for the FFmpeg fallback. It keeps the
// original cue boundaries and never invents word timing.
func WriteASS(path string, cues []Cue, c Clip, config Config) error {
	fontSize := config.CaptionSize
	if fontSize < 24 || fontSize > 150 {
		fontSize = 76
	}
	color := assColor(config.CaptionColor)
	outline := 4
	switch c.CaptionStyle {
	case "minimal", "editorial":
		outline = 2
	case "yellow", "highlight", "karaoke":
		color = assColor(config.AccentColor)
	case "podcast", "boxed":
		outline = 3
	}
	marginV := int(float64(config.Height) * (1 - config.CaptionPosition))
	if marginV < 40 {
		marginV = 40
	}
	font := validCaptionFont(config.CaptionFont)
	var b strings.Builder
	fmt.Fprintf(&b, "[Script Info]\nScriptType: v4.00+\nPlayResX: %d\nPlayResY: %d\nWrapStyle: 0\n[V4+ Styles]\nFormat: Name,Fontname,Fontsize,PrimaryColour,SecondaryColour,OutlineColour,BackColour,Bold,Italic,Underline,StrikeOut,ScaleX,ScaleY,Spacing,Angle,BorderStyle,Outline,Shadow,Alignment,MarginL,MarginR,MarginV,Encoding\nStyle: Default,%s,%d,%s,&H00FFFFFF,&H00000000,&H99000000,-1,0,0,0,100,100,0,0,1,%d,1,2,64,64,%d,1\n[Events]\nFormat: Layer,Start,End,Style,Name,MarginL,MarginR,MarginV,Effect,Text\n", config.Width, config.Height, font, fontSize, color, outline, marginV)
	dur := c.End - c.Start
	for _, cue := range cues {
		start := cue.Start + config.CaptionOffset - c.Start
		end := cue.End + config.CaptionOffset - c.Start
		if end <= 0 || start >= dur {
			continue
		}
		start = max(start, 0)
		end = min(end, dur)
		if end <= start {
			continue
		}
		fmt.Fprintf(&b, "Dialogue: 0,%s,%s,Default,,0,0,0,,%s\n", AssTime(start), AssTime(end), EscapeAss(cue.Text))
	}
	return os.WriteFile(path, []byte(b.String()), 0o600)
}

// videoChain builds the reframing filter for a single source video. two-up
// assumes the guests occupy the left and right halves of the source frame.
func videoChain(cfg Config, c Clip) string {
	prefix := "fps=" + strconv.Itoa(cfg.FPS)
	portrait := cfg.Height > cfg.Width
	if cfg.Reframe && c.Layout == "two-up" {
		if portrait {
			h := cfg.Height / 2
			return fmt.Sprintf("%s,split=2[sideL][sideR];[sideL]crop=iw/2:ih:0:0,scale=w=%d:h=%d:force_original_aspect_ratio=increase,crop=%d:%d[top];[sideR]crop=iw/2:ih:iw/2:0,scale=w=%d:h=%d:force_original_aspect_ratio=increase,crop=%d:%d[bottom];[top][bottom]vstack=inputs=2,setsar=1", prefix, cfg.Width, h, cfg.Width, h, cfg.Width, h, cfg.Width, h)
		}
		w := cfg.Width / 2
		return fmt.Sprintf("%s,split=2[sideL][sideR];[sideL]crop=iw/2:ih:0:0,scale=w=%d:h=%d:force_original_aspect_ratio=increase,crop=%d:%d[left];[sideR]crop=iw/2:ih:iw/2:0,scale=w=%d:h=%d:force_original_aspect_ratio=increase,crop=%d:%d[right];[left][right]hstack=inputs=2,setsar=1", prefix, w, cfg.Height, w, cfg.Height, w, cfg.Height, w, cfg.Height)
	}
	if cfg.Reframe {
		pan := min(max(c.Pan, 0), 1)
		return fmt.Sprintf("%s,scale=w=%d:h=%d:force_original_aspect_ratio=increase,crop=%d:%d:(iw-%d)*%.4f:(ih-%d)/2,setsar=1", prefix, cfg.Width, cfg.Height, cfg.Width, cfg.Height, cfg.Width, pan, cfg.Height)
	}
	return fmt.Sprintf("%s,scale=w=%d:h=%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black,setsar=1", prefix, cfg.Width, cfg.Height, cfg.Width, cfg.Height)
}

// Render exports a clip with FFmpeg alone. It is the fallback when Remotion
// is unavailable: captions are burned from cue boundaries, B-roll is overlaid
// and the callout is drawn as a static banner when the local FFmpeg build can
// draw text (static builds often cannot; the banner is then skipped).
func Render(ctx context.Context, p Project, c Clip, jobID string) (string, string, error) {
	if p.VideoPath == "" {
		return "", "", errors.New("no podcast video selected")
	}
	cfg := p.Config
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return "", "", err
	}
	if err := c.Validate(p.Duration); err != nil {
		return "", "", err
	}
	target, err := AbsDir(p.ExportDir())
	if err != nil {
		return "", "", err
	}
	wantCallout := cfg.Graphics && strings.TrimSpace(c.Callout) != ""
	var logs strings.Builder
	for attempt := 0; attempt < 2; attempt++ {
		callout := wantCallout && attempt == 0
		out, log, err := renderFFmpeg(ctx, p, c, cfg, target, jobID, callout)
		logs.WriteString(log)
		if err == nil {
			return out, logs.String(), nil
		}
		if ctx.Err() != nil {
			return "", logs.String(), err
		}
		if callout {
			logs.WriteString("\n[studio] this FFmpeg build could not draw the callout banner; retrying without it\n")
			continue
		}
		return "", logs.String(), err
	}
	return "", logs.String(), errors.New("FFmpeg render failed")
}

func renderFFmpeg(ctx context.Context, p Project, c Clip, cfg Config, target, jobID string, callout bool) (string, string, error) {
	temp, err := os.MkdirTemp("", "podcast-studio-render-")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(temp)
	ass := filepath.Join(temp, "captions.ass")
	if err = WriteASS(ass, p.Cues, c, cfg); err != nil {
		return "", "", err
	}
	vf := videoChain(cfg, c)
	var finalFilters []string
	if cfg.Captions && c.Captions {
		finalFilters = append(finalFilters, "ass=captions.ass")
	}
	if callout {
		if err = os.WriteFile(filepath.Join(temp, "callout.txt"), []byte(c.Callout), 0o600); err != nil {
			return "", "", err
		}
		finalFilters = append(finalFilters, "drawbox=x=0:y=120:w=iw:h=145:color=black@0.60:t=fill:enable='between(t,0,3.5)',drawtext=font=Arial:textfile=callout.txt:expansion=none:fontsize=56:fontcolor=white:x=(w-text_w)/2:y=145:enable='between(t,0,3.5)'")
	}
	args := []string{"-hide_banner", "-nostdin", "-y", "-ss", fmt.Sprintf("%.3f", c.Start), "-t", fmt.Sprintf("%.3f", c.End-c.Start), "-i", p.VideoPath}
	var overlays []Broll
	if cfg.Broll {
		for _, a := range c.Broll {
			if path, e := AbsRegularFile(a.Path); e == nil && a.Duration > 0 {
				a.Path = path
				overlays = append(overlays, a)
			}
		}
	}
	sort.Slice(overlays, func(i, j int) bool { return overlays[i].Start < overlays[j].Start })
	for _, a := range overlays {
		args = append(args, "-stream_loop", "-1", "-i", a.Path)
	}
	if len(overlays) == 0 {
		if len(finalFilters) > 0 {
			vf += "," + strings.Join(finalFilters, ",")
		}
		args = append(args, "-vf", vf, "-map", "0:v:0")
	} else {
		graph := "[0:v]" + vf + "[v0];"
		previous := "v0"
		for i, a := range overlays {
			out := fmt.Sprintf("v%d", i+1)
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
	}
	args = append(args, "-map", "0:a:0?", "-c:a", "aac", "-b:a", "160k", "-pix_fmt", "yuv420p", "-movflags", "+faststart")
	filename := filepath.Join(target, OutputFileName(c, jobID))
	encoders := []string{"libx264"}
	if cfg.GPU {
		encoders = []string{"h264_nvenc", "libx264"}
	}
	var log string
	for _, encoder := range encoders {
		opts := []string{"-c:v", encoder}
		if encoder == "h264_nvenc" {
			opts = append(opts, "-preset", "p4", "-cq", "23")
		} else {
			opts = append(opts, "-preset", "medium", "-crf", "22")
		}
		cmd := DefaultTool.Command(ctx, "ffmpeg", append(append(append([]string{}, args...), opts...), filename)...)
		cmd.Dir = temp
		buf := &tailBuffer{limit: maxOutput}
		cmd.Stderr = buf
		err = cmd.Run()
		log = buf.String()
		if err == nil {
			return filename, log, nil
		}
		if ctx.Err() != nil {
			return "", log, fmt.Errorf("render cancelled: %w", ctx.Err())
		}
	}
	return "", log, fmt.Errorf("FFmpeg render failed: %w: %s", err, Tail(log, 600))
}
