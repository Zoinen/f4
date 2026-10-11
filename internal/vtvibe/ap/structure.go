package ap

import (
	"fmt"
	"regexp"
	"strings"
)

// --- Structural awareness ---------------------------------------------
// The patcher cannot parse every language, but a bracket counter that skips
// strings and comments is enough to answer the two questions that cause
// most structurally broken output: "am I about to insert top-level code
// inside a block?" and "where does the block opened by this snippet end?".

// Words that open a declaration (the thing that lives at file scope) versus
// words that open a control-flow or closure block (the thing that
// legitimately lives inside another block).
var declarationKeywords = toStringSet([]string{
	"func", "function", "def", "class", "struct", "interface", "enum", "impl",
	"trait", "type", "fn", "namespace", "module", "package", "const", "let",
	"var", "export", "public", "private", "protected", "static", "abstract",
	"template", "record", "object", "proc", "sub",
})

var controlKeywords = toStringSet([]string{
	"if", "else", "elif", "for", "foreach", "while", "switch", "case", "do",
	"try", "catch", "except", "finally", "go", "defer", "select", "with",
	"match", "when", "return", "loop", "unless", "repeat",
})

func toStringSet(items []string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, s := range items {
		m[s] = true
	}
	return m
}

var tokenRE = regexp.MustCompile(`^[A-Za-z_@#]\w*`)

// lineStartDepths returns, for each line start offset in content, the
// bracket nesting depth at that offset. String literals, line comments and
// /* */ blocks are skipped so that brackets inside them do not distort the
// depth. With clamp, ')]}' past a balanced prefix does not push depth
// negative (used for scanning); without it, the raw balance is kept (used
// for the structural sanity check).
func lineStartDepths(content string, clamp bool) (starts []int, depths []int) {
	starts = []int{0}
	depths = []int{0}
	depth := 0
	var inString byte
	inBlockComment := false
	lineComment := false
	escaped := false
	n := len(content)
	for i := 0; i < n; {
		ch := content[i]
		if ch == '\n' {
			lineComment = false
			if inString == '\'' || inString == '"' {
				inString = 0
			}
			escaped = false
			starts = append(starts, i+1)
			depths = append(depths, depth)
			i++
			continue
		}
		if inBlockComment {
			if ch == '*' && strings.HasPrefix(content[i:], "*/") {
				inBlockComment = false
				i += 2
				continue
			}
			i++
			continue
		}
		if inString != 0 {
			switch {
			case escaped:
				escaped = false
			case ch == '\\':
				escaped = true
			case ch == inString:
				inString = 0
			}
			i++
			continue
		}
		if lineComment {
			i++
			continue
		}
		if strings.HasPrefix(content[i:], "/*") {
			inBlockComment = true
			i += 2
			continue
		}
		if ch == '#' || strings.HasPrefix(content[i:], "//") || strings.HasPrefix(content[i:], "--") {
			lineComment = true
			i++
			continue
		}
		switch ch {
		case '"', '\'', '`':
			inString = ch
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if clamp {
				if depth > 0 {
					depth--
				}
			} else {
				depth--
			}
		}
		i++
	}
	return starts, depths
}

// netBracketDepth is the bracket balance of a whole text, ignoring strings
// and comments.
func netBracketDepth(text string) int {
	_, depths := lineStartDepths(text+"\n", false)
	if len(depths) == 0 {
		return 0
	}
	return depths[len(depths)-1]
}

func lineIndexAt(starts []int, offset int) int {
	lo, hi := 0, len(starts)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if starts[mid] <= offset {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}

// resolveScopeEnd extends a located region to the end of the block its
// snippet opens (§2.5 "scope_end"). Brace languages are handled by nesting
// depth; indentation languages by the indent of the snippet's first line.
// The bool result is false when the snippet does not open a block at all.
func resolveScopeEnd(content string, startPos, endPos int) (int, bool) {
	starts, depths := lineStartDepths(content, true)
	lines := strings.Split(content, "\n")
	i0 := lineIndexAt(starts, startPos)
	i1 := lineIndexAt(starts, maxInt(endPos-1, startPos))
	baseDepth := depths[i0]
	depthAfter := baseDepth
	if i1+1 < len(depths) {
		depthAfter = depths[i1+1]
	}

	if depthAfter > baseDepth {
		for j := i1 + 1; j < len(starts); j++ {
			if depths[j] <= baseDepth {
				return starts[j], true
			}
		}
		return len(content), true
	}

	// Indentation languages: the block is everything indented deeper than
	// the snippet's first line.
	baseIndent := indentWidth(lines[i0])
	opener := strings.TrimRightFunc(lines[i1], isPySpace)
	if !strings.HasSuffix(opener, ":") {
		return 0, false
	}
	for j := i1 + 1; j < len(lines); j++ {
		if strings.TrimSpace(lines[j]) == "" {
			continue
		}
		if indentWidth(lines[j]) <= baseIndent {
			return starts[j], true
		}
	}
	return len(content), true
}

func isPySpace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	}
	return false
}

// topLevelInsertCorrection detects the single most damaging insertion
// mistake: a self-contained top-level declaration inserted between a
// declaration line and the body it opens. Returns the corrected offset, or
// ok=false if the insertion point is fine.
func topLevelInsertCorrection(content string, pos int, newContent string, after bool) (int, bool) {
	var body []string
	for _, l := range strings.Split(newContent, "\n") {
		if strings.TrimSpace(l) != "" {
			body = append(body, l)
		}
	}
	if len(body) == 0 || indentWidth(body[0]) > 0 {
		return 0, false
	}
	starts, depths := lineStartDepths(content, true)
	idx := lineIndexAt(starts, pos)
	if depths[idx] <= 0 {
		return 0, false
	}
	// The inserted block must be self-contained.
	_, inner := lineStartDepths(newContent+"\n", true)
	if len(inner) > 0 && inner[len(inner)-1] != 0 {
		return 0, false
	}
	maxInner := 0
	for _, v := range inner {
		if v > maxInner {
			maxInner = v
		}
	}
	if len(inner) == 0 || maxInner == 0 {
		return 0, false
	}
	head := strings.TrimSpace(body[0])
	tok := tokenRE.FindString(head)
	if tok == "" {
		return 0, false
	}
	if controlKeywords[tok] {
		return 0, false
	}
	trimmedHead := strings.TrimRightFunc(head, isPySpace)
	if !declarationKeywords[tok] && !strings.HasSuffix(trimmedHead, "{") && !strings.HasSuffix(trimmedHead, ":") {
		return 0, false
	}
	if after {
		for j := idx; j < len(starts); j++ {
			if depths[j] == 0 {
				return starts[j], true
			}
		}
		return len(content), true
	}
	for j := idx; j >= 0; j-- {
		if depths[j] == 0 {
			return starts[j], true
		}
	}
	return 0, true
}

// reindentContent restores the indentation a model dropped from a `content`
// block, when the whole block sits at column 0 but the target region is
// indented (§3.6).
func reindentContent(content string, startPos, endPos int, newContent, action string, strict bool, warn func(string)) string {
	if strict {
		return newContent
	}
	var regionLines []string
	for _, l := range strings.Split(content[startPos:endPos], "\n") {
		if strings.TrimSpace(l) != "" {
			regionLines = append(regionLines, l)
		}
	}
	if len(regionLines) == 0 {
		return newContent
	}
	var reference string
	if action == "INSERT_AFTER" {
		reference = regionLines[len(regionLines)-1]
	} else {
		reference = regionLines[0]
	}
	baseIndent := leadingWhitespace(reference)
	if baseIndent == "" {
		return newContent
	}
	newLines := strings.Split(newContent, "\n")
	var nonBlank []string
	for _, l := range newLines {
		if strings.TrimSpace(l) != "" {
			nonBlank = append(nonBlank, l)
		}
	}
	if len(nonBlank) == 0 {
		return newContent
	}
	minIndent := -1
	for _, l := range nonBlank {
		w := indentWidth(l)
		if minIndent == -1 || w < minIndent {
			minIndent = w
		}
	}
	if minIndent > 0 {
		return newContent
	}
	if warn != nil {
		warn(fmt.Sprintf("'content' starts at column 0 while the target is indented by %d. "+
			"Re-indenting the inserted block to match.", len(baseIndent)))
	}
	outLines := make([]string, len(newLines))
	for i, l := range newLines {
		if strings.TrimSpace(l) != "" {
			outLines[i] = baseIndent + l
		} else {
			outLines[i] = l
		}
	}
	return strings.Join(outLines, "\n")
}
