package mdmath

import (
	"strings"
	"unicode"

	"github.com/unxed/f4/internal/mermaid"
)

// Prepare rewrites the formulas of a Markdown document: $...$ inside a line
// and $$...$$ blocks become Unicode text (ToUnicode); a formula that cannot be
// converted, and any converted text that Markdown would take for markup, is
// left readable as a code span or block holding its source. Fenced code and
// inline code spans are never touched, and a lone dollar sign, as in a price,
// stays a dollar sign.
func Prepare(markdown string) string {
	out, _ := PrepareMapped(markdown)
	return out
}

// PrepareMapped is Prepare that also says where each line of the result came
// from: origin[i] is the 0-based line of markdown that line i of the result
// was made from (the first one, for a block that was rewritten as a whole).
// A host that shows the result beside the original uses it to map places.
func PrepareMapped(markdown string) (prepared string, origin []int) {
	lines := strings.Split(markdown, "\n")
	if !strings.Contains(markdown, "$") && !strings.Contains(strings.ToLower(markdown), "mermaid") {
		origin = make([]int, len(lines))
		for i := range origin {
			origin[i] = i
		}
		return markdown, origin
	}
	out := make([]string, 0, len(lines))
	add := func(at int, l ...string) {
		out = append(out, l...)
		for range l {
			origin = append(origin, at)
		}
	}
	var fence string // the opening fence marker while inside fenced code
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if fence != "" {
			if closesFence(trimmed, fence) {
				fence = ""
			}
			add(i, line)
			continue
		}
		if m := opensFence(trimmed); m != "" {
			if isMermaid(trimmed, m) {
				if block, next, ok := mermaidBlock(lines, i, m); ok {
					add(i, block...)
					i = next
					continue
				}
			}
			fence = m
			add(i, line)
			continue
		}
		if strings.HasPrefix(trimmed, "$$") {
			if block, next, ok := displayBlock(lines, i); ok {
				add(i, block...)
				i = next
				continue
			}
		}
		add(i, inlineMath(line))
	}
	return strings.Join(out, "\n"), origin
}

// isMermaid says whether an opening fence line names the mermaid language.
func isMermaid(trimmed, fence string) bool {
	info := strings.TrimSpace(strings.TrimPrefix(trimmed, fence))
	return strings.EqualFold(strings.Fields(info + " x")[0], "mermaid")
}

// mermaidBlock converts a fenced mermaid block starting at lines[at] into a
// plain fenced block holding the diagram as text. When the diagram is not one
// the converter fully understands (or the block never closes) it reports
// false and the source stays as it is.
func mermaidBlock(lines []string, at int, fence string) (block []string, last int, ok bool) {
	for j := at + 1; j < len(lines); j++ {
		if !closesFence(strings.TrimSpace(lines[j]), fence) {
			continue
		}
		text, converted := mermaid.Convert(strings.Join(lines[at+1:j], "\n"))
		if !converted {
			return nil, at, false
		}
		f := fenceFor(text)
		return []string{f, text, f}, j, true
	}
	return nil, at, false
}

// fenceFor returns a code fence that text cannot close.
func fenceFor(text string) string {
	fence := "```"
	for strings.Contains(text, fence) {
		fence += "`"
	}
	return fence
}

func opensFence(trimmed string) string {
	for _, ch := range []string{"`", "~"} {
		n := 0
		for n < len(trimmed) && trimmed[n] == ch[0] {
			n++
		}
		if n >= 3 {
			return strings.Repeat(ch, n)
		}
	}
	return ""
}

func closesFence(trimmed, fence string) bool {
	if !strings.HasPrefix(trimmed, fence) {
		return false
	}
	return strings.Trim(trimmed, fence[:1]) == ""
}

// displayBlock reads a $$ block starting at lines[at]: either $$ formula $$
// on one line, or $$ alone on a line, the formula, then $$ alone. It returns
// the replacement lines and the index of the last line consumed.
func displayBlock(lines []string, at int) (block []string, last int, ok bool) {
	first := strings.TrimSpace(lines[at])
	var body string
	last = at
	switch {
	case len(first) > 4 && strings.HasSuffix(first, "$$"):
		body = first[2 : len(first)-2]
	case first == "$$":
		var parts []string
		for j := at + 1; j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) == "$$" {
				body = strings.Join(parts, " ")
				last = j
				ok = true
				break
			}
			parts = append(parts, strings.TrimSpace(lines[j]))
		}
		if !ok {
			return nil, at, false
		}
	default:
		return nil, at, false
	}
	if strings.TrimSpace(body) == "" {
		return nil, at, false
	}
	if text, converted := ToUnicode(body); converted && text != "" {
		return []string{"", inertText(text), ""}, last, true
	}
	return []string{"", "```", strings.TrimSpace(body), "```", ""}, last, true
}

// inertText returns text unchanged when Markdown has nothing to make of it,
// and as a code span when it holds characters Markdown treats as markup.
func inertText(text string) string {
	if strings.ContainsAny(text, "*_`[]<>\\&") {
		return codeSpan(text)
	}
	return text
}

func codeSpan(s string) string {
	return "`" + strings.ReplaceAll(s, "`", "'") + "`"
}

// inlineMath converts the $...$ pairs of one line outside code spans.
func inlineMath(line string) string {
	if !strings.Contains(line, "$") {
		return line
	}
	rs := []rune(line)
	var b strings.Builder
	for i := 0; i < len(rs); {
		r := rs[i]
		switch {
		case r == '\\' && i+1 < len(rs):
			b.WriteRune(r)
			b.WriteRune(rs[i+1])
			i += 2
		case r == '`':
			n := 0
			for i+n < len(rs) && rs[i+n] == '`' {
				n++
			}
			end := findRun(rs, i+n, n)
			if end < 0 {
				b.WriteString(string(rs[i : i+n]))
				i += n
				break
			}
			b.WriteString(string(rs[i : end+n]))
			i = end + n
		case r == '$':
			end := closingDollar(rs, i)
			if end < 0 {
				b.WriteRune(r)
				i++
				break
			}
			src := string(rs[i+1 : end])
			if text, ok := ToUnicode(src); ok && text != "" {
				b.WriteString(inertText(text))
			} else {
				b.WriteString(codeSpan("$" + src + "$"))
			}
			i = end + 1
		default:
			b.WriteRune(r)
			i++
		}
	}
	return b.String()
}

// findRun finds the next run of exactly n backticks at or after from.
func findRun(rs []rune, from, n int) int {
	for i := from; i < len(rs); i++ {
		if rs[i] != '`' {
			continue
		}
		run := 0
		for i+run < len(rs) && rs[i+run] == '`' {
			run++
		}
		if run == n {
			return i
		}
		i += run - 1
	}
	return -1
}

// closingDollar returns the index of the $ that closes a formula opened at
// rs[open], or -1. The opener must be followed by a non-space and the closer
// preceded by one and not followed by a digit, so "costs $5 and $6" is not
// mistaken for a formula.
func closingDollar(rs []rune, open int) int {
	if open+1 >= len(rs) || unicode.IsSpace(rs[open+1]) || rs[open+1] == '$' {
		return -1
	}
	for j := open + 1; j < len(rs); j++ {
		if rs[j] == '\\' {
			j++
			continue
		}
		if rs[j] != '$' {
			continue
		}
		if unicode.IsSpace(rs[j-1]) {
			return -1
		}
		if j+1 < len(rs) && unicode.IsDigit(rs[j+1]) {
			return -1
		}
		return j
	}
	return -1
}
