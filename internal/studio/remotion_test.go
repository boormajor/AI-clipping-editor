package studio

import (
	"context"
	"math"
	"strings"
	"testing"
)

func TestRemotionSourcePropsRealCueBoundaries(t *testing.T) {
	p := Project{Duration: 40, Cues: []Cue{{Start: 10, End: 12, Text: "Opening insight"}, {Start: 12.4, End: 15.7, Text: "A useful answer"}, {Start: 25, End: 29, Text: "Outside"}}, Config: Defaults()}
	c := Clip{ID: "clip1", Start: 10, End: 20, CaptionStyle: "kinetic", Captions: true, Layout: "two-up", Shots: []Shot{{Start: 0, End: 5, Mode: "both"}, {Start: 5, End: 10, Mode: "right"}}, Words: []Word{{Start: 0.1, End: 0.5, Word: "Opening"}}}
	props, err := makeRemotionProps(p, c)
	if err != nil {
		t.Fatal(err)
	}
	if props.Duration != 10 || len(props.Cues) != 2 || props.Cues[0].Start != 0 || math.Abs(props.Cues[1].Start-2.4) > 1e-6 {
		t.Fatalf("wrong clip timing: %#v", props)
	}
	if len(props.Words) != 1 || props.Words[0].Word != "Opening" {
		t.Fatal("must retain measured words")
	}
	if len(props.Shots) != 2 || props.Shots[0].Mode != "both" {
		t.Fatal("shot plan missing")
	}
	if props.CaptionColor != "#FFFFFF" {
		t.Fatal("default color failed")
	}
}
func TestRenderRemotionRequiresInstall(t *testing.T) {
	if RemotionInstalled() {
		t.Skip("Requires a real source video after Remotion dependencies installed")
	}
	p := Project{Duration: 10, Config: Defaults()}
	c := Clip{ID: "clip-01", Start: 0, End: 4, Layout: "focus"}
	_, _, err := RenderRemotion(context.Background(), p, c, "job", nil)
	if err == nil {
		t.Fatal("Remotion render must require Node dependencies")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "remotion") {
		t.Fatalf("unexpected error: %v", err)
	}
}
func TestUnsafeCaptionColorsAndShotBounds(t *testing.T) {
	p := Project{Duration: 60, Config: Defaults()}
	p.Config.CaptionColor = "red;evil" // Normalize replaces invalid colours with the default
	p.Config.AccentColor = "#f0aaff"
	c := Clip{ID: "clip-02", Start: 1, End: 16, Layout: "dynamic", Shots: []Shot{{Start: 0, End: 20, Mode: "both"}}}
	props, err := makeRemotionProps(p, c)
	if err == nil {
		t.Fatalf("should reject out of bounds shots: %#v", props)
	}
	c.Shots = nil
	props, err = makeRemotionProps(p, c)
	if err != nil {
		t.Fatal(err)
	}
	if props.CaptionColor != "#FFFFFF" || props.AccentColor != "#f0aaff" {
		t.Fatal("unsafe color validation")
	}
}
