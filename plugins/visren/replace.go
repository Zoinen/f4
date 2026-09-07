package visren

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/coregx/coregex"
)

func (p *replacementProgram) matches(input string) []TextRange {
	if p == nil || p.plainSearch == "" {
		return nil
	}
	var byteRanges [][]int
	if p.useRegex {
		byteRanges = p.regex.re.FindAllStringIndex(input, -1)
	} else {
		haystack, needle := input, p.plainSearch
		if !p.caseSens {
			haystack, needle = strings.ToLower(haystack), strings.ToLower(needle)
		}
		for offset := 0; offset <= len(haystack); {
			found := strings.Index(haystack[offset:], needle)
			if found < 0 {
				break
			}
			start := offset + found
			end := start + len(needle)
			if end > len(input) {
				break
			}
			byteRanges = append(byteRanges, []int{start, end})
			offset = end
		}
	}

	ranges := make([]TextRange, 0, len(byteRanges))
	for _, match := range byteRanges {
		if len(match) < 2 || match[0] < 0 || match[1] <= match[0] || match[1] > len(input) {
			continue
		}
		ranges = append(ranges, TextRange{
			Start: utf8.RuneCountInString(input[:match[0]]),
			End:   utf8.RuneCountInString(input[:match[1]]),
		})
	}
	return ranges
}

func (p *replacementProgram) applyWithRanges(input string) (string, []TextRange) {
	if p == nil || p.plainSearch == "" {
		return input, nil
	}

	type replacement struct {
		start, end int
		text       string
	}
	var replacements []replacement
	if p.useRegex {
		template := farReplacement(p.plainRepl)
		for _, match := range p.regex.re.FindAllStringSubmatchIndex(input, -1) {
			if len(match) < 2 || match[0] < 0 || match[1] < match[0] {
				continue
			}
			expanded := p.regex.re.ExpandString(nil, template, input, match)
			replacements = append(replacements, replacement{start: match[0], end: match[1], text: string(expanded)})
		}
	} else {
		haystack, needle := input, p.plainSearch
		if !p.caseSens {
			haystack, needle = strings.ToLower(haystack), strings.ToLower(needle)
		}
		for offset := 0; offset <= len(haystack); {
			found := strings.Index(haystack[offset:], needle)
			if found < 0 {
				break
			}
			start := offset + found
			end := start + len(needle)
			if end > len(input) {
				break
			}
			replacements = append(replacements, replacement{start: start, end: end, text: p.plainRepl})
			offset = end
		}
	}

	if len(replacements) == 0 {
		return input, nil
	}
	var out strings.Builder
	ranges := make([]TextRange, 0, len(replacements))
	last := 0
	runePos := 0
	for _, replacement := range replacements {
		unchanged := input[last:replacement.start]
		out.WriteString(unchanged)
		runePos += utf8.RuneCountInString(unchanged)
		start := runePos
		out.WriteString(replacement.text)
		runePos += utf8.RuneCountInString(replacement.text)
		if runePos > start {
			ranges = append(ranges, TextRange{Start: start, End: runePos})
		}
		last = replacement.end
	}
	out.WriteString(input[last:])
	return out.String(), ranges
}

type regexReplacer struct {
	re *coregex.Regexp
}

func compileReplacement(opts Options) (*replacementProgram, error) {
	p := &replacementProgram{
		plainSearch: opts.Search,
		plainRepl:   opts.Replace,
		useRegex:    opts.Regex,
		caseSens:    opts.CaseSensitive,
	}
	if !opts.Regex || opts.Search == "" {
		return p, nil
	}
	pattern, flags, err := parseFarRegex(opts.Search)
	if err != nil {
		return nil, err
	}
	if !opts.CaseSensitive || strings.Contains(flags, "i") {
		pattern = "(?i)" + pattern
	}
	re, err := coregex.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regular expression: %w", err)
	}
	p.regex.re = re
	return p, nil
}

func parseFarRegex(expr string) (string, string, error) {
	if !strings.HasPrefix(expr, "/") {
		return expr, "", nil
	}
	escaped := false
	for i := 1; i < len(expr); i++ {
		if escaped {
			escaped = false
			continue
		}
		if expr[i] == '\\' {
			escaped = true
			continue
		}
		if expr[i] == '/' {
			flags := expr[i+1:]
			for _, flag := range flags {
				if flag != 'i' {
					return "", "", fmt.Errorf("unsupported regex flag %q", flag)
				}
			}
			return strings.ReplaceAll(expr[1:i], `\/`, `/`), flags, nil
		}
	}
	return "", "", fmt.Errorf("unterminated /regular expression/")
}

func farReplacement(src string) string {
	// coregex follows Go's $1 expansion. Far additionally accepts a backslash
	// before a literal replacement character; remove that quoting here.
	var out strings.Builder
	for i := 0; i < len(src); i++ {
		if src[i] == '\\' && i+1 < len(src) {
			i++
			out.WriteByte(src[i])
			continue
		}
		out.WriteByte(src[i])
	}
	return out.String()
}
