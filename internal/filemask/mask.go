// Package filemask matches a file name against a far2l-style file mask.
//
// It is the syntax f4 already speaks wherever a user names a set of files --
// associations, selection, directory sync -- and it lives on its own so that
// code which is not a panel can ask the same question the panel asks.
package filemask

import (
	"path/filepath"
	"regexp"
	"strings"
)

// Match reports whether name satisfies the given far2l-style mask.
//
// Syntax mirrors far2l's CFileMask:
//   - "," and ";" separate include globs (OR).
//   - "|" splits include vs. exclude; matches on the exclude side veto.
//   - A section wrapped in "/…/" is a regular expression.
//
// Matching honours ignoreCase for both globs and regex (via (?i)).
// A mask that fails to parse (bad regex, empty include list) never
// matches — safer than silently degrading to "matches everything".
func Match(name, mask string, ignoreCase bool) bool {
	mask = strings.TrimSpace(mask)
	if mask == "" || name == "" {
		return false
	}
	includes, excludes := splitMaskIncludeExclude(mask)
	if len(includes) == 0 {
		return false
	}
	matchAny := func(list []string) bool {
		for _, m := range list {
			if m = strings.TrimSpace(m); m == "" {
				continue
			}
			if matchOneMask(name, m, ignoreCase) {
				return true
			}
		}
		return false
	}
	if !matchAny(includes) {
		return false
	}
	if matchAny(excludes) {
		return false
	}
	return true
}

// splitMaskIncludeExclude divides on the far2l "|" separator and then
// splits each side on "," / ";" into individual masks. A regex section
// (`/…/`) is preserved as a single mask so its commas stay literal.
func splitMaskIncludeExclude(mask string) (includes, excludes []string) {
	inc, exc, hasExc := splitOnPipeSkipRegex(mask)
	includes = splitMaskCommaSemi(inc)
	if hasExc {
		excludes = splitMaskCommaSemi(exc)
	}
	return includes, excludes
}

// splitOnPipeSkipRegex splits mask on the first '|' that is not inside
// a /regex/ block. If no '|' occurs, hasExc is false.
func splitOnPipeSkipRegex(mask string) (inc, exc string, hasExc bool) {
	depth := 0 // "inside /…/" tracker
	for i := 0; i < len(mask); i++ {
		c := mask[i]
		switch c {
		case '/':
			// Toggle regex mode on unescaped slashes. Rough but matches
			// far2l behaviour: a lone '/' outside a regex is treated as
			// part of the glob (unusual but permitted).
			if depth == 0 {
				depth = 1
			} else {
				depth = 0
			}
		case '|':
			if depth == 0 {
				return mask[:i], mask[i+1:], true
			}
		}
	}
	return mask, "", false
}

// splitMaskCommaSemi splits a mask side on ',' / ';' while keeping
// regex sections (/…/) intact.
func splitMaskCommaSemi(side string) []string {
	if side == "" {
		return nil
	}
	var out []string
	var cur strings.Builder
	depth := 0
	flush := func() {
		s := strings.TrimSpace(cur.String())
		cur.Reset()
		if s != "" {
			out = append(out, s)
		}
	}
	for i := 0; i < len(side); i++ {
		c := side[i]
		switch c {
		case '/':
			if depth == 0 {
				depth = 1
			} else {
				depth = 0
			}
			cur.WriteByte(c)
		case ',', ';':
			if depth == 0 {
				flush()
			} else {
				cur.WriteByte(c)
			}
		default:
			cur.WriteByte(c)
		}
	}
	flush()
	return out
}

// matchOneMask handles a single mask: '/regex/' or a glob. Glob "*.*"
// is normalised to "*" so it matches extension-less names too, matching
// far2l's long-standing quirk.
func matchOneMask(name, mask string, ignoreCase bool) bool {
	if len(mask) >= 2 && mask[0] == '/' && mask[len(mask)-1] == '/' {
		pattern := mask[1 : len(mask)-1]
		if ignoreCase {
			pattern = "(?i)" + pattern
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false
		}
		return re.MatchString(name)
	}
	// Star-dot-star means "match everything".
	if mask == "*.*" {
		mask = "*"
	}
	m, n := mask, name
	if ignoreCase {
		m = strings.ToLower(m)
		n = strings.ToLower(n)
	}
	ok, err := filepath.Match(m, n)
	if err != nil {
		return false
	}
	return ok
}
