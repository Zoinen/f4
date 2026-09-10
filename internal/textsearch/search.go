// Package textsearch holds the one pattern search the viewer and the editor
// share. It lives apart from both because it belongs to neither: every
// argument is a byte slice or a flag, and a copy in each package is a pair of
// search semantics that drift.
package textsearch

import (
	"github.com/charlievieth/strcase"
	"github.com/coregx/coregex"
	"strings"
	"unsafe"
)

// FindMatch locates one occurrence of pattern in data: forward from
// startOff, or backward from just before it when reverse is set; next
// additionally skips a match starting exactly at startOff. The offset is
// -1 when nothing matches. The returned length is the matched byte length,
// which for regex or case-folded matches can differ from len(pattern).
func FindMatch(data []byte, pattern string, caseSensitive, reverse, regexp, wholeWord, next bool, startOff int) (int, int, error) {
	foundOffset := -1
	matchLen := len(pattern)
	totalSize := len(data)

	// Only whole-word matching needs the regex engine (for the \b
	// wrapping); literal search, case-sensitive or folded, is handled
	// below without it.
	if regexp || wholeWord {
		re, err := BuildSearchRegex(pattern, caseSensitive, regexp, wholeWord)
		if err != nil {
			return -1, 0, err
		}

		if !reverse {
			currOff := startOff
			if next {
				currOff++
			}
			if currOff < totalSize {
				loc := re.FindIndex(data[currOff:])
				if loc != nil {
					foundOffset = currOff + loc[0]
					matchLen = loc[1] - loc[0]
				}
			}
		} else {
			currOff := startOff
			if next {
				currOff--
			}
			if currOff > totalSize {
				currOff = totalSize
			}
			if currOff > 0 {
				// FindAllIndex scans left-to-right without overlap, so for
				// self-overlapping patterns (e.g. "яя" in "яяя") this can
				// land left of the true rightmost hit. Accepted: unlike the
				// old ToLower+LastIndex path it cannot corrupt byte offsets.
				locs := re.FindAllIndex(data[:currOff], -1)
				if len(locs) > 0 {
					last := locs[len(locs)-1]
					foundOffset = last[0]
					matchLen = last[1] - last[0]
				}
			}
		}
		return foundOffset, matchLen, nil
	}

	text := BytesToString(data)
	index, lastIndex := strings.Index, strings.LastIndex
	if !caseSensitive {
		// strcase folds while scanning the original text, so the offsets
		// it returns need no translation.
		index, lastIndex = strcase.Index, strcase.LastIndex
	}
	if !reverse {
		currOff := startOff
		if next {
			currOff++
		}
		if currOff < len(text) {
			idx := index(text[currOff:], pattern)
			if idx != -1 {
				foundOffset = currOff + idx
			}
		}
	} else {
		currOff := startOff
		if next {
			currOff--
		}
		if currOff > len(text) {
			currOff = len(text)
		}
		if currOff > 0 {
			idx := lastIndex(text[:currOff], pattern)
			if idx != -1 {
				foundOffset = idx
			}
		}
	}
	// A folded match can differ in byte length from the pattern
	// (K U+212A matches "k"), so measure what it consumed.
	if foundOffset != -1 && !caseSensitive {
		if after, ok := strcase.CutPrefix(text[foundOffset:], pattern); ok {
			matchLen = len(text) - foundOffset - len(after)
		}
	}
	return foundOffset, matchLen, nil
}

// buildSearchRegex compiles an editor search pattern with the rules shared
// by Find, Find All and Replace: non-regex input is quoted literally, case
// insensitivity becomes an (?i) prefix and whole-word wraps the pattern in
// word boundaries.
func BuildSearchRegex(pattern string, caseSensitive, useRegex, wholeWord bool) (*coregex.Regex, error) {
	finalPattern := pattern
	if !useRegex {
		finalPattern = coregex.QuoteMeta(pattern)
	}
	if !caseSensitive {
		finalPattern = "(?i)" + finalPattern
	}
	if wholeWord {
		finalPattern = `\b(?:` + finalPattern + `)\b`
	}
	return coregex.Compile(finalPattern)
}

// bytesToString views bytes as a string without copying them. Every caller
// here only reads, and the bytes are either a private snapshot or a window
// into a buffer nothing writes through.
func BytesToString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	// #nosec G103 -- the returned string is read-only and every caller retains b for the full lifetime of the synchronous search.
	return unsafe.String(unsafe.SliceData(b), len(b))
}
