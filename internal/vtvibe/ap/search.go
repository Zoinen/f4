package ap

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// span is a half-open [a, b) byte range within a content string.
type span struct{ a, b int }

// occurrence is a half-open [start, end) match, as (start, end).
type occurrence [2]int

func normalizedNonBlankLines(s string) []string {
	var out []string
	for _, l := range splitLinesUniversal(strings.TrimSpace(s)) {
		t := strings.TrimSpace(l)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func derefOr(s *string, def string) string {
	if s == nil {
		return def
	}
	return *s
}

// splitLinesKeepEnds splits '\n'-only text into lines, keeping the trailing
// '\n' attached to each line except a possible final line without one; it
// mirrors Python's str.splitlines(keepends=True) for text that only ever
// uses '\n' as a separator (which is the only line ending ap.go's engine
// ever works with internally).
func splitLinesKeepEnds(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i+1])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

// smartFind is the "Hybrid Search" locator algorithm (§3.6): the first line
// of the (normalized, blank-line-stripped) snippet is matched as a *suffix*
// of a target line, subsequent lines must match exactly. Returns byte
// [start, end) ranges within content, relative to content itself.
func smartFind(content, snippet string) []occurrence {
	originalLines := splitLinesKeepEnds(content)
	snippetLines := normalizedNonBlankLinesKeepOrder(snippet)
	if len(snippetLines) == 0 {
		return nil
	}
	normalizedSnippetLines := make([]string, len(snippetLines))
	for i, l := range snippetLines {
		normalizedSnippetLines[i] = strings.TrimSpace(l)
	}

	var occurrences []occurrence
	for i := 0; i < len(originalLines); i++ {
		if strings.TrimSpace(originalLines[i]) == "" {
			continue
		}
		var contentLinesFound []string
		endLineIndex := i - 1
		tempJ := i
		for len(contentLinesFound) < len(snippetLines) && tempJ < len(originalLines) {
			line := originalLines[tempJ]
			if strings.TrimSpace(line) != "" {
				contentLinesFound = append(contentLinesFound, line)
			}
			endLineIndex = tempJ
			tempJ++
		}

		if len(contentLinesFound) != len(snippetLines) {
			continue
		}
		normalizedContentLines := make([]string, len(contentLinesFound))
		for k, l := range contentLinesFound {
			normalizedContentLines[k] = strings.TrimSpace(l)
		}
		firstLineMatch := strings.HasSuffix(normalizedContentLines[0], normalizedSnippetLines[0])
		if firstLineMatch && len(normalizedContentLines[0]) != len(normalizedSnippetLines[0]) {
			dropped := normalizedContentLines[0][:len(normalizedContentLines[0])-len(normalizedSnippetLines[0])]
			firstLineMatch = decorationPrefixRE.MatchString(dropped)
		}
		tailMatch := stringSlicesEqual(normalizedContentLines[1:], normalizedSnippetLines[1:])
		if firstLineMatch && tailMatch {
			startPos := len(strings.Join(originalLines[:i], ""))
			endPos := len(strings.Join(originalLines[:endLineIndex+1], ""))
			occurrences = append(occurrences, occurrence{startPos, endPos})
		}
	}
	return occurrences
}

// normalizedNonBlankLinesKeepOrder is like normalizedNonBlankLines but keeps
// the original (unstripped) line text, for smartFind's own trimming.
func normalizedNonBlankLinesKeepOrder(s string) []string {
	var out []string
	for _, l := range splitLinesUniversal(strings.TrimSpace(s)) {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

// --- fuzzy / partial-line diagnostics (§3.4 "clear, human-readable error
// messages") -----------------------------------------------------------

type fuzzyMatch struct {
	LineNumber int
	Score      float64
	Text       string
}

func getFuzzyMatches(content, snippet string) []fuzzyMatch {
	const cutoff = 0.7
	if strings.TrimSpace(snippet) == "" {
		return nil
	}
	// PERFORMANCE GUARD: fuzzy matching is O(N*M); skip it for large
	// snippets rather than hang on failure.
	if len(snippet) > 1000 || len(splitLinesUniversal(snippet)) > 20 {
		return nil
	}
	normalizedSnippetLines := normalizedNonBlankLines(snippet)
	if len(normalizedSnippetLines) == 0 {
		return nil
	}
	snippetAsBlock := strings.Join(normalizedSnippetLines, "\n")
	windowSize := len(normalizedSnippetLines)

	contentLines := splitLinesUniversal(content)
	type lineMeta struct {
		num  int
		text string
	}
	var sourceLines []lineMeta
	for i, l := range contentLines {
		t := strings.TrimSpace(l)
		if t != "" {
			sourceLines = append(sourceLines, lineMeta{i + 1, t})
		}
	}

	var matches []fuzzyMatch
	for i := 0; i+windowSize <= len(sourceLines); i++ {
		window := sourceLines[i : i+windowSize]
		var lineNums []int
		var windowLines []string
		for _, m := range window {
			lineNums = append(lineNums, m.num)
			windowLines = append(windowLines, m.text)
		}
		windowAsBlock := strings.Join(windowLines, "\n")
		ratio := sequenceRatio(snippetAsBlock, windowAsBlock)
		if ratio >= cutoff {
			startIdx := lineNums[0] - 1
			endIdx := lineNums[len(lineNums)-1]
			text := strings.Join(contentLines[startIdx:endIdx], "\n")
			matches = append(matches, fuzzyMatch{
				LineNumber: lineNums[0],
				Score:      math.Round(ratio*10000) / 10000,
				Text:       text,
			})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].Score > matches[j].Score })
	if len(matches) > 3 {
		matches = matches[:3]
	}
	return matches
}

// sequenceRatio approximates difflib.SequenceMatcher(None, a,
// b).ratio() = 2*M/T via an LCS length in place of the reference's
// recursive matching-blocks algorithm. The two methods usually agree
// closely on near-duplicate text; nothing in this package depends on an
// exact score, only on "close enough to be worth showing as a hint".
func sequenceRatio(a, b string) float64 {
	if a == "" && b == "" {
		return 1.0
	}
	ra, rb := []rune(a), []rune(b)
	lcs := lcsLength(ra, rb)
	return 2.0 * float64(lcs) / float64(len(ra)+len(rb))
}

func lcsLength(a, b []rune) int {
	n, m := len(a), len(b)
	if n == 0 || m == 0 {
		return 0
	}
	prev := make([]int, m+1)
	curr := make([]int, m+1)
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			if a[i-1] == b[j-1] {
				curr[j] = prev[j-1] + 1
			} else if prev[j] >= curr[j-1] {
				curr[j] = prev[j]
			} else {
				curr[j] = curr[j-1]
			}
		}
		prev, curr = curr, prev
	}
	return prev[m]
}

type partialLineMatch struct {
	LineNumber int
	Text       string
}

// findPartialLineMatches detects the single most common locator mistake: a
// snippet that is only a *fragment* of a line.
func findPartialLineMatches(content string, snippet *string, limit int) []partialLineMatch {
	lines := normalizedNonBlankLines(derefOr(snippet, ""))
	if len(lines) != 1 {
		return nil
	}
	needle := lines[0]
	if len(needle) < 4 {
		return nil
	}
	var hits []partialLineMatch
	for idx, line := range splitLinesUniversal(content) {
		stripped := strings.TrimSpace(line)
		if stripped != needle && strings.Contains(stripped, needle) {
			hits = append(hits, partialLineMatch{idx + 1, line})
			if len(hits) >= limit {
				break
			}
		}
	}
	return hits
}

// --- primary locator ------------------------------------------------------

// findTargetInContent is the Search and Location Algorithm of §3.2/§3.6: it
// locates snippet (optionally scoped by anchor) within content, applying
// uniqueness rules, the sequential cursor (lastMatchEnd), the self-written
// text exclusion (dirty) and the "repeated locator" allowance.
//
// It returns a byte [start, end) range within content, or an *AppError
// describing why none could be determined.
func findTargetInContent(content string, anchor, snippet *string, lastMatchEnd int, allowRepeat bool, dirty []span, dirtyStrict bool) (int, int, *AppError) {
	hasAnchor := anchor != nil && *anchor != ""

	if !hasAnchor {
		s := derefOr(snippet, "")
		if strings.TrimSpace(s) == "^" {
			return 0, 0, nil
		}
		if strings.TrimSpace(s) == "$" {
			return len(content), len(content), nil
		}
	}

	searchSpace := content
	offset := 0
	anchorFound := false

	if hasAnchor {
		anchorStr := *anchor
		anchorOccurrences := smartFind(content, anchorStr)
		if len(anchorOccurrences) == 0 {
			return 0, 0, &AppError{Code: ErrAnchorNotFound, Message: "Anchor not found.",
				Context: map[string]any{"anchor": anchorStr}}
		}

		// An anchor duplicated by this patch's own output is not a real
		// second anchor.
		if len(dirty) > 0 {
			var clean []occurrence
			for _, a := range anchorOccurrences {
				if !isDirtySpan(span{a[0], a[1]}, dirty) {
					clean = append(clean, a)
				}
			}
			if len(clean) > 0 {
				anchorOccurrences = clean
			}
		}

		// Cursor filtering: prefer anchors after the last change.
		if len(anchorOccurrences) > 1 && lastMatchEnd > 0 {
			var forward []occurrence
			for _, a := range anchorOccurrences {
				if a[0] >= lastMatchEnd {
					forward = append(forward, a)
				}
			}
			if len(forward) > 0 {
				anchorOccurrences = forward
			}
		}

		// Deep scope resolution: is the snippet unique inside exactly one
		// anchor scope?
		if len(anchorOccurrences) > 1 {
			snippetStr := derefOr(snippet, "")
			allSnippetOccurrences := smartFind(content, snippetStr)
			var validScopes []occurrence
			for _, a := range anchorOccurrences {
				aEnd := a[1]
				var snippetsAfter []occurrence
				for _, s := range allSnippetOccurrences {
					if s[0] >= aEnd {
						snippetsAfter = append(snippetsAfter, s)
					}
				}
				if len(snippetsAfter) > 0 {
					firstSnip := snippetsAfter[0]
					isShadowed := false
					for _, other := range anchorOccurrences {
						if other[0] > aEnd && other[0] < firstSnip[0] {
							isShadowed = true
							break
						}
					}
					if !isShadowed {
						validScopes = append(validScopes, a)
					}
				}
			}
			if len(validScopes) == 1 {
				anchorOccurrences = validScopes
			}
		}

		if len(anchorOccurrences) > 1 {
			return 0, 0, &AppError{
				Code:    ErrAmbiguousAnchor,
				Message: fmt.Sprintf("Anchor found %d times and ambiguity could not be resolved.", len(anchorOccurrences)),
				Context: map[string]any{"anchor": anchorStr, "count": len(anchorOccurrences)},
			}
		}

		anchorStart, anchorEnd := anchorOccurrences[0][0], anchorOccurrences[0][1]

		// Overlap & containment detection between anchor and snippet.
		sLines := normalizedNonBlankLines(derefOr(snippet, ""))
		aLines := normalizedNonBlankLines(anchorStr)
		isOverlap := false
		if len(sLines) > 0 && len(aLines) > 0 {
			for k := 1; k <= minInt(len(aLines), len(sLines)); k++ {
				if stringSlicesEqual(aLines[len(aLines)-k:], sLines[:k]) {
					isOverlap = true
					break
				}
			}
			if !isOverlap {
				for i := 0; i <= len(aLines)-len(sLines); i++ {
					if stringSlicesEqual(aLines[i:i+len(sLines)], sLines) {
						isOverlap = true
						break
					}
				}
			}
			if !isOverlap {
				for i := 0; i <= len(sLines)-len(aLines); i++ {
					if stringSlicesEqual(sLines[i:i+len(aLines)], aLines) {
						isOverlap = true
						break
					}
				}
			}
		}

		if isOverlap {
			searchSpace, offset, anchorFound = content[anchorStart:], anchorStart, true
		} else {
			searchSpace, offset, anchorFound = content[anchorEnd:], anchorEnd, true
		}
	}

	snippetStr := derefOr(snippet, "")
	occurrences := smartFind(searchSpace, snippetStr)

	// Self-written text filter.
	if len(dirty) > 0 && len(occurrences) > 0 {
		var clean []occurrence
		for _, o := range occurrences {
			if !isDirtySpan(span{o[0] + offset, o[1] + offset}, dirty) {
				clean = append(clean, o)
			}
		}
		if len(clean) > 0 {
			occurrences = clean
		} else if dirtyStrict {
			occurrences = nil
		}
		// else: all matches lie in self-written text; fall back to using
		// them anyway (occurrences left untouched).
	}

	// Sequential cursor filter.
	if len(occurrences) > 0 {
		var forward []occurrence
		for _, o := range occurrences {
			if o[0]+offset >= lastMatchEnd {
				forward = append(forward, o)
			}
		}
		if len(forward) > 0 {
			if len(forward) > 1 && !hasAnchor && !allowRepeat {
				var positions []int
				for _, o := range forward {
					positions = append(positions, lineNumberAt(content, o[0]+offset))
				}
				shown := positions
				if len(shown) > 10 {
					shown = shown[:10]
				}
				strPositions := make([]string, len(shown))
				for i, p := range shown {
					strPositions[i] = strconv.Itoa(p)
				}
				return 0, 0, &AppError{
					Code: ErrAmbiguousMatch,
					Message: fmt.Sprintf("Snippet matches %d places (lines %s). "+
						"Add an 'anchor' to disambiguate, or extend the snippet.",
						len(forward), strings.Join(strPositions, ", ")),
					Context: map[string]any{"snippet": snippetStr, "count": len(forward), "match_lines": positions},
				}
			}
			occurrences = forward[:1]
		} else {
			occurrences = nil
		}
	}

	if len(occurrences) == 0 {
		var previewLines []string
		for _, l := range splitLinesUniversal(searchSpace) {
			if strings.TrimSpace(l) != "" {
				previewLines = append(previewLines, l)
			}
		}
		shown := previewLines
		if len(shown) > 7 {
			shown = shown[:7]
		}
		ctx := map[string]any{
			"snippet":              snippetStr,
			"anchor":               derefOr(anchor, ""),
			"anchor_found":         anchorFound,
			"fuzzy_matches":        getFuzzyMatches(searchSpace, snippetStr),
			"search_space_preview": strings.Join(shown, "\n"),
		}
		partial := findPartialLineMatches(searchSpace, snippet, 3)
		if len(partial) > 0 {
			ctx["partial_line_matches"] = partial
			ctx["hint"] = "The snippet appears as a fragment of an existing line. " +
				"`ap` locators MUST cover whole lines: extend the snippet " +
				"to the full line, from its first non-blank character to its last."
		}
		return 0, 0, &AppError{Code: ErrSnippetNotFound, Message: "Snippet not found.", Context: ctx}
	}

	startPos, endPos := occurrences[0][0], occurrences[0][1]
	return startPos + offset, endPos + offset, nil
}

func lineNumberAt(content string, offset int) int {
	if offset > len(content) {
		offset = len(content)
	}
	return strings.Count(content[:offset], "\n") + 1
}

func isDirtySpan(sp span, dirty []span) bool {
	for _, d := range dirty {
		if d.a <= sp.a && sp.b <= d.b {
			return true
		}
	}
	return false
}

func shiftDirty(dirty []span, start, end, newLen int) []span {
	delta := newLen - (end - start)
	var updated []span
	for _, d := range dirty {
		if d.b <= start {
			updated = append(updated, d)
		} else if d.a >= end {
			updated = append(updated, span{d.a + delta, d.b + delta})
		}
	}
	if newLen > 0 {
		updated = append(updated, span{start, start + newLen})
	}
	sort.Slice(updated, func(i, j int) bool {
		if updated[i].a != updated[j].a {
			return updated[i].a < updated[j].a
		}
		return updated[i].b < updated[j].b
	})
	return updated
}
