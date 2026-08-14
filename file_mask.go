package main

import "strings"

// destMask returns the wildcard mask typed as the last component of a copy or
// move destination, or "" when there is none.
//
// far2l treats such a component as a rename mask — "*.bak" copies every file
// under its own name with a new extension — rather than as a literal name.
//
// Only the last component is inspected. A wildcard earlier in the path names
// a directory that either exists or does not, and either way the operation
// fails on its own terms rather than producing a surprise.
func destMask(destInput string) string {
	trimmed := strings.TrimRight(destInput, "/\\")
	if trimmed == "" {
		return ""
	}
	last := trimmed
	if i := strings.LastIndexAny(trimmed, "/\\"); i >= 0 {
		last = trimmed[i+1:]
	}
	if strings.ContainsAny(last, "*?") {
		return last
	}
	return ""
}

// destWithoutMask is the destination directory left once the mask component
// is taken off. A bare mask means the destination panel's own directory.
func destWithoutMask(destInput string) string {
	trimmed := strings.TrimRight(destInput, "/\\")
	if i := strings.LastIndexAny(trimmed, "/\\"); i >= 0 {
		if dir := trimmed[:i+1]; dir != "" {
			return dir
		}
	}
	return "."
}

// applyFileMask generates a name from a source name and a wildcard mask, port
// of far2l's ConvertWildcards (src/mix/processname.cpp).
//
// The rules are not the ones a glob matcher uses, and they are worth stating:
// '?' takes one character of the source, '*' takes source characters until the
// next mask character matches (with a mask dot stopping at the source's *last*
// dot, so "*.bak" splits extension the way one expects), a literal character
// is written out and consumes a source character unless the source is sitting
// on a dot, a mask dot skips the source past its next dot when a wildcard
// follows, and a trailing dot in the result is dropped.
func applyFileMask(srcName, mask string) string {
	if !strings.ContainsAny(mask, "*?") {
		return mask
	}
	src := []rune(srcName)
	w := []rune(mask)
	out := make([]rune, 0, len(src)+len(w))

	srcDot := -1
	for i, r := range src {
		if r == '.' {
			srcDot = i
		}
	}
	// A mask char past the end reads as NUL in the original and matches
	// nothing, which is what makes a trailing '*' copy the rest.
	at := func(i int) rune {
		if i < len(w) {
			return w[i]
		}
		return 0
	}

	si, wi := 0, 0
	for wi < len(w) {
		switch w[wi] {
		case '?':
			wi++
			if si < len(src) {
				out = append(out, src[si])
				si++
			}
		case '*':
			wi++
			for si < len(src) {
				if at(wi) == '.' && srcDot >= 0 && !strings.ContainsRune(string(w[wi+1:]), '.') {
					if si == srcDot {
						break
					}
				} else if src[si] == at(wi) {
					break
				}
				out = append(out, src[si])
				si++
			}
		case '.':
			wi++
			out = append(out, '.')
			if strings.ContainsAny(string(w[wi:]), "*?") {
				for si < len(src) {
					c := src[si]
					si++
					if c == '.' {
						break
					}
				}
			}
		default:
			out = append(out, w[wi])
			wi++
			if si < len(src) && src[si] != '.' {
				si++
			}
		}
	}

	if n := len(out); n > 0 && out[n-1] == '.' {
		out = out[:n-1]
	}
	return string(out)
}
