package textlayout

import (
	"unicode"
	"unicode/utf8"

	"github.com/unxed/vtui"
)

// VisualCluster is a terminal-facing grapheme cluster with byte boundaries in
// the original string.
type VisualCluster struct {
	Text       string
	Width      int
	Start, End int
	RuneStart  int
	RuneEnd    int
}

// VisualClusters segments a string once and returns its visual clusters. It
// deliberately uses vtui's callback API rather than calling NextCluster for
// every suffix: the latter would rescan the remaining string for every ASCII
// character and turn long-line layout into quadratic work.
func VisualClusters(s string) []VisualCluster {
	clusters := make([]VisualCluster, 0, len(s))
	previousStart, previousWidth, previousRuneStart := 0, 0, 0
	havePrevious := false

	emit := func(start, end, width, runeStart, runeEnd int) {
		if start >= end {
			return
		}
		raw := s[start:end]
		if len(clusters) > 0 && vtui.JoinsConjunct(clusters[len(clusters)-1].Text, raw) {
			last := &clusters[len(clusters)-1]
			last.Text = s[last.Start:end]
			last.End = end
			last.Width = vtui.ClusterWidth(last.Text)
			last.RuneEnd = runeEnd
			return
		}
		clusters = append(clusters, VisualCluster{Text: raw, Width: width, Start: start, End: end, RuneStart: runeStart, RuneEnd: runeEnd})
	}

	vtui.ForEachClusterAt(s, func(_ string, width, offset, runeIndex int) {
		if havePrevious {
			emit(previousStart, offset, previousWidth, previousRuneStart, runeIndex)
		}
		previousStart, previousWidth, previousRuneStart = offset, width, runeIndex
		havePrevious = true
	})
	if havePrevious {
		emit(previousStart, len(s), previousWidth, previousRuneStart, utf8.RuneCountInString(s))
	}
	return clusters
}

// NextVisualCluster returns the next cluster as it is treated by a terminal
// text editor. UAX #29 (which vtui implements) handles combining marks, emoji,
// and bidi marks, while vtui.JoinsConjunct adds rule GB9c the way the
// terminal applies it, joining an Indic virama to the consonant that follows
// it. Keeping one rule for wrapping, painting, cursor movement and deletion
// makes them agree on the same byte boundaries.
func NextVisualCluster(s string) (cluster string, width int, size int) {
	clusters := VisualClusters(s)
	if len(clusters) == 0 {
		return "", 0, 0
	}
	first := clusters[0]
	return first.Text, first.Width, first.End
}

func containsIndicVirama(s string) bool {
	for _, r := range s {
		if isIndicVirama(r) {
			return true
		}
	}
	return false
}

func isIndicVirama(r rune) bool {
	switch r {
	case
		'\u094D',                     // Devanagari
		'\u09CD',                     // Bengali
		'\u0A4D',                     // Gurmukhi
		'\u0ACD',                     // Gujarati
		'\u0B4D',                     // Oriya
		'\u0BCD',                     // Tamil
		'\u0C4D',                     // Telugu
		'\u0CCD',                     // Kannada
		'\u0D3B', '\u0D3C', '\u0D4D', // Malayalam
		'\u0DCA', // Sinhala
		'\u1039', // Myanmar
		'\u1714', // Tagalog
		'\u17D2', // Khmer
		'\u1A60', // Tai Tham
		'\u1BAA', // Sundanese
		'\uA806', // Syloti Nagri
		'\uA8C4', // Saurashtra
		'\uA953', // Rejang
		'\uA9C0', // Javanese
		'\uAAF6', // Meetei Mayek
		'\uABED': // Meetei Mayek
		return true
	default:
		return false
	}
}

// TrailingModifierStart returns the byte offset of a terminal script modifier
// that the editor keeps as a separate backspace step. zoin-bot preserves the
// established terminal behaviour for final Indic viramas and Thaana marks,
// while ordinary Latin combining graphemes remain atomic.
func TrailingModifierStart(s string) int {
	r, size := utf8.DecodeLastRuneInString(s)
	if size == 0 {
		return -1
	}
	if containsIndicVirama(s) || isIndicScriptMark(r) || (r >= 0x0780 && r <= 0x07BF && unicode.Is(unicode.Mn, r)) {
		return len(s) - size
	}
	return -1
}

func isIndicScriptMark(r rune) bool {
	return (unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Mc, r)) && unicode.In(r,
		unicode.Devanagari,
		unicode.Bengali,
		unicode.Gurmukhi,
		unicode.Gujarati,
		unicode.Oriya,
		unicode.Tamil,
		unicode.Telugu,
		unicode.Kannada,
		unicode.Malayalam,
		unicode.Sinhala,
	)
}
