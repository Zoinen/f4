package vtui

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// fuzzyMatcher implements approximate substring search using Myers'
// bit-vector algorithm. For needles of up to 64 runes the edit distance is
// computed in O(len(text)) bit-parallel operations over a single uint64.
// Longer needles degrade to exact substring search.
//
// The matcher reports the best (lowest) edit distance between the needle and
// any substring of the text, plus the starting position of that match.
// Case-insensitive matching is done by indexing both cases of every needle
// character, so the text is looked up as-is.
type fuzzyMatcher struct {
	m int // needle length in runes
	k int // max edit distance accepted as a match

	needle        string
	caseSensitive bool

	ascii    bool
	peqAscii [256]uint64     // L1-resident fast path for ASCII needles and texts
	peqRune  map[rune]uint64 // built lazily on the first non-ASCII text

	exactOnly  bool   // needle longer than 64 runes
	needleFold string // needle (folded in case-insensitive mode) for exactOnly
	fold       bool   // fold the haystack too (case-insensitive exactOnly)

	rev *fuzzyMatcher // lazy reversed-needle matcher used to locate match starts
}

// newFuzzyMatcher builds a matcher for the given needle. It returns nil for
// an empty needle. The acceptance threshold is len(needle)/3 errors: exact
// substring matches always pass (score 0), short needles stay almost strict,
// longer ones tolerate more typos.
func newFuzzyMatcher(needle string, caseSensitive bool) *fuzzyMatcher {
	if needle == "" {
		return nil
	}
	fm := &fuzzyMatcher{m: utf8.RuneCountInString(needle), needle: needle, caseSensitive: caseSensitive}
	fm.k = fm.m / 3

	if fm.m > 64 {
		fm.exactOnly = true
		fm.fold = !caseSensitive
		if caseSensitive {
			fm.needleFold = needle
		} else {
			fm.needleFold = strings.ToLower(needle)
		}
		return fm
	}

	fm.ascii = isASCIIString(needle)
	if fm.ascii {
		for i := 0; i < len(needle); i++ {
			b := needle[i]
			fm.peqAscii[b] |= 1 << uint(i)
			if !caseSensitive {
				if 'a' <= b && b <= 'z' {
					fm.peqAscii[b-'a'+'A'] |= 1 << uint(i)
				} else if 'A' <= b && b <= 'Z' {
					fm.peqAscii[b-'A'+'a'] |= 1 << uint(i)
				}
			}
		}
	} else {
		fm.buildRuneTable()
	}
	return fm
}

// buildRuneTable populates the Unicode peq table. For ASCII needles it is
// built lazily on the first non-ASCII text, so the common all-ASCII case
// never pays for the map.
func (fm *fuzzyMatcher) buildRuneTable() {
	fm.peqRune = make(map[rune]uint64, fm.m)
	i := 0
	for _, r := range fm.needle {
		bit := uint64(1) << uint(i)
		fm.peqRune[r] |= bit
		if !fm.caseSensitive {
			fm.peqRune[unicode.ToLower(r)] |= bit
			fm.peqRune[unicode.ToUpper(r)] |= bit
		}
		i++
	}
}

// match searches the needle inside text. It returns the best edit distance,
// the starting position (in runes) of the best matching substring, and
// whether the distance is within the acceptance threshold.
// match searches the needle inside text. It returns the best edit distance,
// the span [start, end] (inclusive, in runes) of the best matching substring,
// and whether the distance is within the acceptance threshold.
func (fm *fuzzyMatcher) match(text string) (score, start, end int, ok bool) {
	if fm.exactOnly {
		hay := text
		if fm.fold {
			hay = strings.ToLower(text)
		}
		idx := strings.Index(hay, fm.needleFold)
		if idx < 0 {
			return 0, 0, 0, false
		}
		start = utf8.RuneCountInString(text[:idx])
		return 0, start, start + fm.m - 1, true
	}
	if fm.ascii && isASCIIString(text) {
		score, end = fm.matchASCII(text)
	} else {
		if fm.peqRune == nil {
			fm.buildRuneTable()
		}
		score, end = fm.matchRunes(text)
	}
	score = clampScore(score, fm.m)
	if score == 0 {
		// An exact match spans exactly m runes.
		start = end - fm.m + 1
	} else {
		start = fm.findStart(text, end)
	}
	if start < 0 {
		start = 0
	}
	return score, start, end, score <= fm.k
}

// findStart locates the beginning of the best fuzzy match ending at end
// (rune index). It runs a second bit-vector pass with the reversed needle
// over the reversed text prefix; edit distance is symmetric under reversal,
// so the reverse pass reproduces the same score and its end position maps
// back to the start of the forward match. Only used for inexact matches, so
// it runs at most once per matched row per filter rebuild — never in the
// render hot path.
func (fm *fuzzyMatcher) findStart(text string, end int) int {
	rev := fm.reverseMatcher()

	runes := []rune(text)
	if end+1 < len(runes) {
		runes = runes[:end+1]
	}
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	if rev.peqRune == nil {
		rev.buildRuneTable()
	}
	_, revEnd := rev.matchRunes(string(runes))
	return end - revEnd
}

// reverseMatcher lazily builds the matcher for the reversed needle.
func (fm *fuzzyMatcher) reverseMatcher() *fuzzyMatcher {
	if fm.rev == nil {
		r := []rune(fm.needle)
		for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
			r[i], r[j] = r[j], r[i]
		}
		fm.rev = newFuzzyMatcher(string(r), fm.caseSensitive)
	}
	return fm.rev
}

// matchASCII runs the bit-vector scan over bytes; text must be pure ASCII,
// so byte positions equal rune positions. Returns the best score and the
// end position of the best match.
func (fm *fuzzyMatcher) matchASCII(s string) (bestScore, bestEnd int) {
	m := fm.m
	top := uint64(1) << uint(m-1)
	pv := ^uint64(0)
	mv := uint64(0)
	score := m
	bestScore = m + 1
	bestEnd = 0
	for j := 0; j < len(s); j++ {
		eq := fm.peqAscii[s[j]]
		xv := eq | mv
		xh := (((eq & pv) + pv) ^ pv) | eq
		ph := mv | ^(xh | pv)
		mh := pv & xh
		if ph&top != 0 {
			score++
		} else if mh&top != 0 {
			score--
		}
		// Substring search: D[0][j] = 0, so the horizontal delta injected
		// into row 0 is 0 and both vectors shift in a zero bit.
		ph <<= 1
		mh <<= 1
		pv = mh | ^(xv | ph)
		mv = ph & xv
		if score < bestScore {
			bestScore = score
			bestEnd = j
			if bestScore == 0 {
				break // cannot do better; first exact match ends here
			}
		} else if score == bestScore {
			// Among equal-scoring matches prefer the latest end: it yields
			// the fullest span (e.g. "abxc" for needle "abc", not just "ab").
			bestEnd = j
		}
	}
	return bestScore, bestEnd
}

// matchRunes is the Unicode fallback of the same algorithm.
func (fm *fuzzyMatcher) matchRunes(s string) (bestScore, bestEnd int) {
	m := fm.m
	top := uint64(1) << uint(m-1)
	pv := ^uint64(0)
	mv := uint64(0)
	score := m
	bestScore = m + 1
	bestEnd = 0
	j := 0
	for _, r := range s {
		eq := fm.peqRune[r]
		xv := eq | mv
		xh := (((eq & pv) + pv) ^ pv) | eq
		ph := mv | ^(xh | pv)
		mh := pv & xh
		if ph&top != 0 {
			score++
		} else if mh&top != 0 {
			score--
		}
		ph <<= 1
		mh <<= 1
		pv = mh | ^(xv | ph)
		mv = ph & xv
		if score < bestScore {
			bestScore = score
			bestEnd = j
			if bestScore == 0 {
				break
			}
		} else if score == bestScore {
			bestEnd = j // see matchASCII: latest end gives the fullest span
		}
		j++
	}
	return bestScore, bestEnd
}

// clampScore converts the "no text scanned" sentinel into the real distance
// (needle length: deleting the whole needle matches the empty substring).
func clampScore(bestScore, m int) int {
	if bestScore > m {
		return m
	}
	return bestScore
}

func isASCIIString(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// FuzzyMatcher is the exported handle over the Myers bit-vector matcher for
// applications embedding vtui (Table.QuickSearch uses the unexported one).
// Construction precomputes the needle tables once; Match is then linear in
// the text length per candidate.
type FuzzyMatcher struct{ fm *fuzzyMatcher }

// NewFuzzyMatcher builds a matcher for needle, or nil for an empty needle.
// The acceptance threshold is len(needle)/3 errors: exact substring matches
// always pass, longer needles tolerate more typos.
func NewFuzzyMatcher(needle string, caseSensitive bool) *FuzzyMatcher {
	if fm := newFuzzyMatcher(needle, caseSensitive); fm != nil {
		return &FuzzyMatcher{fm: fm}
	}
	return nil
}

// Match reports the best match of the needle inside text: the edit distance
// score and the matched span as rune indices, end inclusive. ok is false
// when the best distance exceeds the acceptance threshold.
func (m *FuzzyMatcher) Match(text string) (score, start, end int, ok bool) {
	return m.fm.match(text)
}
