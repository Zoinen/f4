package vtui

import (
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"github.com/rivo/uniseg"
)

// EmojiPresentationWide tells the layout engine how wide a character that
// carries an emoji presentation selector (U+FE0F) is on screen. Terminals
// disagree: wcwidth based ones keep the width of the base character, while
// most modern emulators advance two columns. Two columns is the common case,
// so it is the default; set this to false for a strictly wcwidth terminal.
var EmojiPresentationWide = true

// Composite grapheme clusters (a base character plus combining marks, an
// emoji ZWJ sequence, a flag) do not fit into a rune, so CharInfo.Char keeps
// an index into a process wide registry instead of a code point. Indices are
// marked with CompCharFlag; anything below it is a plain rune. This mirrors
// far2l's COMP_CHAR, which is why CharInfo.Char is 64 bit wide.
const (
	CompCharFlag uint64 = 1 << 63
	// MaxCompChar is the largest index the registry may hand out. It stays
	// far below WideCharFiller (all bits set) so the two can never collide.
	MaxCompChar uint64 = CompCharFlag | 0x00FFFFFF
)

// The code points the width rules below care about by name.
const (
	runeZWJ            rune = 0x200D
	runeVS15           rune = 0xFE0E
	runeVS16           rune = 0xFE0F
	runeKeycap         rune = 0x20E3
	runeModifierFirst  rune = 0x1F3FB
	runeModifierLast   rune = 0x1F3FF
	runeRegionalFirst  rune = 0x1F1E6
	runeRegionalLast   rune = 0x1F1FF
	runeReplacement    rune = 0xFFFD
	runeControlVisible rune = '·'
)

type clusterRegistry struct {
	mu    sync.RWMutex
	byID  []string
	byStr map[string]uint64
}

var clusters = clusterRegistry{byStr: make(map[string]uint64)}

// IsCompChar reports whether a CharInfo.Char value is a registry index rather
// than a plain rune. WideCharFiller shares the high bit and is not one.
func IsCompChar(ch uint64) bool {
	return ch != WideCharFiller && ch&CompCharFlag != 0
}

// RegisterCluster turns a grapheme cluster into a CharInfo.Char value. Single
// rune clusters are stored as the rune itself, so the common path allocates
// nothing and old code comparing a cell against a rune keeps working. Longer
// ones go into the registry. If the registry is ever exhausted the base rune
// is returned, which loses the marks but never corrupts the screen.
func RegisterCluster(cluster string) uint64 {
	if cluster == "" {
		return 0
	}
	r, size := utf8.DecodeRuneInString(cluster)
	if size == len(cluster) {
		return uint64(r)
	}

	clusters.mu.RLock()
	id, ok := clusters.byStr[cluster]
	clusters.mu.RUnlock()
	if ok {
		return id
	}

	clusters.mu.Lock()
	defer clusters.mu.Unlock()
	if id, ok := clusters.byStr[cluster]; ok {
		return id
	}
	next := CompCharFlag | uint64(len(clusters.byID)+1)
	if next > MaxCompChar {
		return uint64(r)
	}
	clusters.byID = append(clusters.byID, cluster)
	clusters.byStr[cluster] = next
	return next
}

// CellString returns the text a cell carries. Fillers and empty cells render
// as nothing and a space respectively, which is what every backend wants.
func CellString(ch uint64) string {
	switch {
	case ch == WideCharFiller:
		return ""
	case ch == 0:
		return " "
	case IsCompChar(ch):
		idx := int(ch&^CompCharFlag) - 1
		clusters.mu.RLock()
		defer clusters.mu.RUnlock()
		if idx < 0 || idx >= len(clusters.byID) {
			return string(runeReplacement)
		}
		return clusters.byID[idx]
	default:
		return string(rune(ch))
	}
}

// CellRunes returns the runes a cell carries, base character first.
func CellRunes(ch uint64) []rune {
	if ch == WideCharFiller || ch == 0 {
		return nil
	}
	if !IsCompChar(ch) {
		return []rune{rune(ch)}
	}
	return []rune(CellString(ch))
}

// CellBaseRune returns the base character of a cell, ignoring any combining
// marks. Backends that can only draw one glyph per cell use this.
func CellBaseRune(ch uint64) rune {
	if ch == WideCharFiller {
		return 0
	}
	if !IsCompChar(ch) {
		return rune(ch)
	}
	r, _ := utf8.DecodeRuneInString(CellString(ch))
	return r
}

// runeCellWidth is the per rune column count a wcwidth driven terminal uses.
// go-runewidth does not treat every combining mark as zero wide (the
// Devanagari virama, for one), so the Unicode categories decide first: non
// spacing and enclosing marks and format characters take no room, spacing
// marks do.
func runeCellWidth(r rune) int {
	if unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf) {
		return 0
	}
	if w := runewidth.RuneWidth(r); w > 0 {
		return w
	}
	return 0
}

// ClusterWidth returns how many terminal columns a grapheme cluster occupies.
// The base is the sum of the per rune widths, which is what a wcwidth driven
// terminal does, with the emoji sequences that every terminal special cases
// pinned to two columns.
func ClusterWidth(cluster string) int {
	if cluster == "" {
		return 0
	}

	sum := 0
	regional := 0
	hasZWJ := false
	hasVS16 := false
	hasVS15 := false
	hasKeycap := false
	hasModifier := false

	for _, r := range cluster {
		switch {
		case r == runeZWJ:
			hasZWJ = true
		case r == runeVS16:
			hasVS16 = true
		case r == runeVS15:
			hasVS15 = true
		case r == runeKeycap:
			hasKeycap = true
		case r >= runeModifierFirst && r <= runeModifierLast:
			hasModifier = true
		case r >= runeRegionalFirst && r <= runeRegionalLast:
			regional++
		}
		sum += runeCellWidth(r)
	}

	switch {
	case hasZWJ, hasKeycap, hasModifier, regional >= 2:
		return 2
	case hasVS15:
		return 1
	case hasVS16 && EmojiPresentationWide:
		return 2
	}

	if sum <= 0 {
		// A cluster of nothing but combining marks: no base to hang them on,
		// so give it a column of its own rather than dropping it.
		return 1
	}
	return sum
}

// NextCluster splits off the first grapheme cluster of s. It returns the
// cluster, its width in columns and its size in bytes; size is zero only for
// an empty string.
func NextCluster(s string) (cluster string, width int, size int) {
	if s == "" {
		return "", 0, 0
	}
	g := uniseg.NewGraphemes(s)
	if !g.Next() {
		return "", 0, 0
	}
	from, to := g.Positions()
	cluster = s[from:to]
	return cluster, ClusterWidth(cluster), to - from
}

// SanitizeCluster makes a cluster safe to put on screen. Line breaks are
// dropped, other control characters become a visible dot, and a replacement
// character becomes a question mark, as before. The returned width is zero
// when the cluster must not be emitted at all.
func SanitizeCluster(cluster string) (string, int) {
	if cluster == "" {
		return "", 0
	}
	r, size := utf8.DecodeRuneInString(cluster)
	if size == len(cluster) {
		switch {
		case r == '\n' || r == '\r':
			return "", 0
		case r == runeReplacement:
			return "?", 1
		case r < 0x20 || r == 0x7F:
			return string(runeControlVisible), 1
		}
	}
	return cluster, ClusterWidth(cluster)
}

// ForEachCluster walks s cluster by cluster, handing the callback the
// sanitized text, its width and the byte offset the cluster started at in s.
// Clusters that must not be drawn are skipped.
func ForEachCluster(s string, fn func(cluster string, width int, offset int)) {
	ForEachClusterAt(s, func(cluster string, width, offset, _ int) {
		fn(cluster, width, offset)
	})
}

// ForEachClusterAt is ForEachCluster with the index of the cluster's first
// rune in s as well. Positions coming from code that counts runes, such as
// the hotkey position of an ampersand string, need it.
func ForEachClusterAt(s string, fn func(cluster string, width, offset, runeIndex int)) {
	runeIndex := 0
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		from, to := g.Positions()
		raw := s[from:to]
		text, w := SanitizeCluster(raw)
		if w > 0 {
			fn(text, w, from, runeIndex)
		}
		runeIndex += utf8.RuneCountInString(raw)
	}
}

// AppendCluster puts a cluster into a cell slice, following it with as many
// fillers as the extra columns it claims.
func AppendCluster(target []CharInfo, cluster string, width int, attr uint64) []CharInfo {
	if width <= 0 {
		return target
	}
	target = append(target, CharInfo{Char: RegisterCluster(cluster), Attributes: attr})
	for i := 1; i < width; i++ {
		target = append(target, CharInfo{Char: WideCharFiller, Attributes: attr})
	}
	return target
}

// StringWidth returns the width of a string in terminal columns, counting
// grapheme clusters rather than runes.
func StringWidth(s string) int {
	total := 0
	ForEachCluster(s, func(_ string, w int, _ int) {
		total += w
	})
	return total
}

// TruncateString shortens s so that it plus tail fits into w columns. It never
// cuts a grapheme cluster in half and never leaves a wide character with only
// one of its two columns on screen.
func TruncateString(s string, w int, tail string) string {
	if w <= 0 {
		return ""
	}
	if StringWidth(s) <= w {
		return s
	}
	tailW := StringWidth(tail)
	budget := w - tailW
	if budget < 0 {
		return tail
	}

	var sb strings.Builder
	used := 0
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		from, to := g.Positions()
		text, cw := SanitizeCluster(s[from:to])
		if cw == 0 {
			continue
		}
		if used+cw > budget {
			break
		}
		sb.WriteString(text)
		used += cw
	}
	return sb.String() + tail
}
