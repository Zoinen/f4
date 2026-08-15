package vtui

import (
	"github.com/rivo/uniseg"
	"golang.org/x/text/unicode/bidi"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// WideCharFiller is a special marker indicating that this cell in ScreenBuf
// is occupied by the right half of a full-width character (like CJK or Emoji).
const WideCharFiller = ^uint64(0)

// ExtractHotkey quickly finds the hotkey rune in a string without allocating memory.
func ExtractHotkey(s string) rune {
	idx := 0
	for {
		i := strings.IndexByte(s[idx:], '&')
		if i == -1 {
			return 0
		}
		idx += i
		if idx+1 < len(s) {
			if s[idx+1] == '&' {
				idx += 2
				continue
			}
			r, _ := utf8.DecodeRuneInString(s[idx+1:])
			return unicode.ToLower(r)
		}
		return 0
	}
}

// ParseAmpersandString parses a string with ampersands, removes utility &,
// processes && as &, and returns the clean string, the hotkey, and its position (in runes).
func ParseAmpersandString(s string) (clean string, hotkey rune, hotkeyPos int) {
	if s == "" || strings.IndexByte(s, '&') == -1 {
		return s, 0, -1
	}

	var sb strings.Builder
	hotkeyPos = -1
	runeCount := 0
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		if runes[i] == '&' && i+1 < len(runes) {
			if runes[i+1] == '&' {
				sb.WriteRune('&')
				runeCount++
				i++
			} else {
				if hotkeyPos == -1 {
					hotkey = unicode.ToLower(runes[i+1])
					hotkeyPos = runeCount
				}
				sb.WriteRune(runes[i+1])
				runeCount++
				i++
			}
		} else {
			sb.WriteRune(runes[i])
			runeCount++
		}
	}
	return sb.String(), hotkey, hotkeyPos
}

// StringToCharInfoHighlighted works like StringToCharInfo but highlights the letter after &.
func StringToCharInfoHighlighted(s string, normalAttr, highAttr uint64) ([]CharInfo, rune) {
	clean, hk, hkPos := ParseAmpersandString(s)
	if DefaultBidiMode == BidiOff || !HasRTL(clean) {
		res := make([]CharInfo, 0, len(clean))
		ForEachClusterAt(clean, func(cluster string, w, _, runeIdx int) {
			attr := normalAttr
			if hkPos >= runeIdx && hkPos < runeIdx+utf8.RuneCountInString(cluster) {
				attr = highAttr
			}
			res = AppendCluster(res, cluster, w, attr)
		})
		return res, hk
	}

	type logicalCluster struct {
		text    string
		runeIdx int
		attr    uint64
	}

	var logicalClusters []logicalCluster
	runeIdx := 0
	g := uniseg.NewGraphemes(clean)
	for g.Next() {
		from, to := g.Positions()
		clText := clean[from:to]
		attr := normalAttr
		if hkPos >= runeIdx && hkPos < runeIdx+utf8.RuneCountInString(clText) {
			attr = highAttr
		}
		logicalClusters = append(logicalClusters, logicalCluster{
			text:    clText,
			runeIdx: runeIdx,
			attr:    attr,
		})
		runeIdx += utf8.RuneCountInString(clText)
	}

	p := bidi.Paragraph{}
	_, err := p.SetString(clean)
	if err != nil {
		res := make([]CharInfo, 0, len(clean))
		for _, c := range logicalClusters {
			res = AppendCluster(res, c.text, ClusterWidth(c.text), c.attr)
		}
		return res, hk
	}
	order, err := p.Order()
	if err != nil {
		res := make([]CharInfo, 0, len(clean))
		for _, c := range logicalClusters {
			res = AppendCluster(res, c.text, ClusterWidth(c.text), c.attr)
		}
		return res, hk
	}

	var res []CharInfo
	numRuns := order.NumRuns()
	for i := 0; i < numRuns; i++ {
		run := order.Run(i)
		start, end := run.Pos()

		var runClusters []logicalCluster
		for _, c := range logicalClusters {
			if c.runeIdx >= start && c.runeIdx <= end {
				runClusters = append(runClusters, c)
			}
		}

		isRTL := run.Direction() == bidi.RightToLeft
		if isRTL {
			for i, j := 0, len(runClusters)-1; i < j; i, j = i+1, j-1 {
				runClusters[i], runClusters[j] = runClusters[j], runClusters[i]
			}
			for i := range runClusters {
				if utf8.RuneCountInString(runClusters[i].text) == 1 {
					runClusters[i].text = bidi.ReverseString(runClusters[i].text)
				}
			}
		}

		for _, c := range runClusters {
			res = AppendCluster(res, c.text, ClusterWidth(c.text), c.attr)
		}
	}

	return res, hk
}

// SanitizeRune ensures the rune is printable and handles its visual width.
// It looks at one rune in isolation, so a combining mark reaching it has
// already been separated from the character it belongs to and can only be
// shown as a placeholder. Code that has the surrounding text should call
// SanitizeCluster instead.
func SanitizeRune(r rune) (rune, int) {
	if r == '\n' || r == '\r' {
		return 0, 0
	}
	if r == '\uFFFD' {
		return '?', 1
	}
	if r < 0x20 || r == 0x7F {
		return '·', 1
	}
	w := runewidth.RuneWidth(r)
	if w <= 0 {
		return '·', 1 // Visible placeholder for zero-width/invalid
	}
	return r, w
}

func StringToCharInfo(s string, attr uint64) []CharInfo {
	s = VisualString(s)
	res := make([]CharInfo, 0, len(s))
	ForEachCluster(s, func(cluster string, w, _ int) {
		res = AppendCluster(res, cluster, w, attr)
	})
	return res
}

func FillCharInfo(target []CharInfo, data []byte, attr uint64) []CharInfo {
	target = target[:0]
	ForEachCluster(string(data), func(cluster string, w, _ int) {
		target = AppendCluster(target, cluster, w, attr)
	})
	return target
}

// FillCharInfoWithSelection combines FillCharInfo and selection highlighting in a single pass.
// Selection bounds are byte offsets into the whole line; a cluster is selected
// when the byte its first rune starts at falls inside them.
func FillCharInfoWithSelection(target []CharInfo, data []byte, defaultAttr, selAttr uint64, fragStartOffset, selMin, selMax int) []CharInfo {
	target = target[:0]
	ForEachCluster(string(data), func(cluster string, w, offset int) {
		attr := defaultAttr
		absPos := fragStartOffset + offset
		if absPos >= selMin && absPos < selMax {
			attr = selAttr
		}
		target = AppendCluster(target, cluster, w, attr)
	})
	return target
}

func RunesToCharInfo(runes []rune, attr uint64) []CharInfo {
	return StringToCharInfo(string(runes), attr)
}
