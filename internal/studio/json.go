package studio

import (
	"encoding/json"
	"strings"
)

func parseJSONTranscript(b []byte) ([]Cue, bool) {
	var v struct {
		Segments []struct {
			Start float64 `json:"start"`
			End   float64 `json:"end"`
			Text  string  `json:"text"`
		} `json:"segments"`
	}
	if json.Unmarshal(b, &v) != nil || len(v.Segments) == 0 {
		return nil, false
	}
	var out []Cue
	for _, a := range v.Segments {
		out = append(out, Cue{a.Start, a.End, strings.TrimSpace(a.Text)})
	}
	out = normalizeCues(out)
	return out, len(out) > 0
}
