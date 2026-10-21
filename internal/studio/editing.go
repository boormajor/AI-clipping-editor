package studio

import "strings"

// Conservative disfluency set. Only unambiguous filler sounds are included so
// meaningful words ("like", "so", "right") are never auto-removed.
var fillerWords = map[string]bool{
	"um": true, "umm": true, "ummm": true, "uh": true, "uhh": true, "uhhh": true,
	"erm": true, "uhm": true, "hmm": true, "hmmm": true, "mm": true, "mmm": true,
	"ah": true, "ahh": true, "er": true, "err": true, "eh": true, "mhm": true,
}

func normalizeWord(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.TrimFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	})
}

// IsFiller reports whether a recognized word is a disfluency safe to drop.
func IsFiller(word string) bool { return fillerWords[normalizeWord(word)] }

// CleanReport summarizes trimmable dead air and disfluencies in a clip's
// measured words. All times are seconds relative to the clip start.
type CleanReport struct {
	Words          int      `json:"words"`
	Fillers        int      `json:"fillers"`
	FillerWords    []string `json:"fillerWords"`
	Silences       int      `json:"silences"`
	LongestSilence float64  `json:"longestSilence"`
	DeadAir        float64  `json:"deadAir"` // leading+trailing seconds that can be trimmed
	TrimStart      float64  `json:"trimStart"`
	TrimEnd        float64  `json:"trimEnd"`
}

// AnalyzeClean inspects measured words (clip-relative, 0..dur) and suggests
// trimming leading/trailing dead air and reports internal long pauses and
// filler words. It never rewrites audio; it only advises on tighter bounds.
func AnalyzeClean(words []Word, dur, silenceGap float64) CleanReport {
	rep := CleanReport{Words: len(words)}
	if silenceGap <= 0 {
		silenceGap = 0.6
	}
	if len(words) == 0 {
		return rep
	}
	first, last := -1, -1
	for i, w := range words {
		if IsFiller(w.Word) {
			rep.Fillers++
			if len(rep.FillerWords) < 25 {
				rep.FillerWords = append(rep.FillerWords, strings.TrimSpace(w.Word))
			}
			continue
		}
		if first < 0 {
			first = i
		}
		last = i
	}
	if first < 0 { // the whole clip is filler/noise; don't suggest trimming it away
		return rep
	}
	// Leading dead air: trim up to the first meaningful word.
	if s := words[first].Start; s > 0.12 {
		rep.TrimStart = s
	}
	// Trailing dead air: trim from the last meaningful word to the clip end.
	if e := dur - words[last].End; e > 0.12 {
		rep.TrimEnd = e
	}
	rep.DeadAir = rep.TrimStart + rep.TrimEnd
	// Internal long pauses between meaningful words.
	for i := first + 1; i <= last; i++ {
		if IsFiller(words[i].Word) {
			continue
		}
		prev := i - 1
		for prev > first && IsFiller(words[prev].Word) {
			prev--
		}
		gap := words[i].Start - words[prev].End
		if gap > silenceGap {
			rep.Silences++
			if gap > rep.LongestSilence {
				rep.LongestSilence = gap
			}
		}
	}
	return rep
}
