package visren

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

type maskFlags struct {
	lower, upper, first, title, mixed bool
	translit, detranslit              bool
}

type replacementProgram struct {
	plainSearch string
	plainRepl   string
	regex       regexReplacer
	useRegex    bool
	caseSens    bool
}

func renderItem(item *Item, index int, opts Options, repl *replacementProgram) (string, []TextRange, error) {
	base, ext := SplitName(item.Source)
	name, nf, err := expandMask(opts.NameMask, base, ext, item, index)
	if err != nil {
		return "", nil, fmt.Errorf("name mask: %w", err)
	}
	newExt, ef, err := expandMask(opts.ExtMask, base, ext, item, index)
	if err != nil {
		return "", nil, fmt.Errorf("extension mask: %w", err)
	}
	full := joinName(name, newExt)
	full, replacementMatches := repl.applyWithRanges(full)
	name, newExt = SplitName(full)
	nameMatches, extMatches, separatorMatch := splitTextRanges(full, replacementMatches)
	name, nameMatches = applyFlagsToRanges(name, nameMatches, nf, opts.WordDiv)
	newExt, extMatches = applyFlagsToRanges(newExt, extMatches, ef, opts.WordDiv)
	destination := joinName(name, newExt)
	if newExt != "" {
		offset := len([]rune(name)) + 1
		if separatorMatch {
			nameMatches = append(nameMatches, TextRange{Start: offset - 1, End: offset})
		}
		for _, match := range extMatches {
			nameMatches = append(nameMatches, TextRange{Start: match.Start + offset, End: match.End + offset})
		}
	}
	return destination, nameMatches, nil
}

func splitTextRanges(value string, ranges []TextRange) ([]TextRange, []TextRange, bool) {
	dot := strings.LastIndex(value, ".")
	if dot < 0 || dot == len(value)-1 {
		return ranges, nil, false
	}
	split := utf8.RuneCountInString(value[:dot])
	var nameRanges, extRanges []TextRange
	separatorMatch := false
	for _, match := range ranges {
		if start, end := match.Start, minInt(match.End, split); start < end {
			nameRanges = append(nameRanges, TextRange{Start: start, End: end})
		}
		separatorMatch = separatorMatch || match.Start <= split && match.End > split
		if start, end := maxInt(match.Start, split+1), match.End; start < end {
			extRanges = append(extRanges, TextRange{Start: start - split - 1, End: end - split - 1})
		}
	}
	return nameRanges, extRanges, separatorMatch
}

func applyFlagsToRanges(value string, ranges []TextRange, flags maskFlags, wordDiv string) (string, []TextRange) {
	result := applyFlags(value, flags, wordDiv)
	if len(ranges) == 0 {
		return result, nil
	}
	runes := []rune(value)
	mapped := make([]TextRange, 0, len(ranges))
	for _, match := range ranges {
		start := maxInt(0, minInt(len(runes), match.Start))
		end := maxInt(start, minInt(len(runes), match.End))
		mappedStart := len([]rune(applyFlags(string(runes[:start]), flags, wordDiv)))
		mappedEnd := len([]rune(applyFlags(string(runes[:end]), flags, wordDiv)))
		if mappedStart < mappedEnd {
			mapped = append(mapped, TextRange{Start: mappedStart, End: mappedEnd})
		}
	}
	return result, mapped
}

func SplitName(name string) (string, string) {
	idx := strings.LastIndex(name, ".")
	if idx < 0 || idx == len(name)-1 {
		return name, ""
	}
	return name[:idx], name[idx+1:]
}

func joinName(base, ext string) string {
	if ext == "" {
		return base
	}
	return base + "." + ext
}

func expandMask(mask, base, ext string, item *Item, index int) (string, maskFlags, error) {
	var out strings.Builder
	var flags maskFlags
	for pos := 0; pos < len(mask); {
		if strings.HasPrefix(mask[pos:], "[]]") {
			out.WriteByte(']')
			pos += 3
			continue
		}
		if mask[pos] != '[' {
			r, size := utf8.DecodeRuneInString(mask[pos:])
			if r == ']' {
				return "", flags, fmt.Errorf("unmatched ']' at column %d", utf8.RuneCountInString(mask[:pos])+1)
			}
			out.WriteRune(r)
			pos += size
			continue
		}
		endRel := strings.IndexByte(mask[pos+1:], ']')
		if endRel < 0 {
			return "", flags, fmt.Errorf("unclosed '[' at column %d", utf8.RuneCountInString(mask[:pos])+1)
		}
		end := pos + 1 + endRel
		token := mask[pos+1 : end]
		value, err := expandToken(token, base, ext, item, index, &flags)
		if err != nil {
			return "", flags, err
		}
		out.WriteString(value)
		pos = end + 1
	}
	return out.String(), flags, nil
}

func expandToken(token, base, ext string, item *Item, index int, flags *maskFlags) (string, error) {
	switch token {
	case "[":
		return "[", nil
	case "]":
		return "]", nil
	case "L":
		flags.lower = true
		return "", nil
	case "U":
		flags.upper = true
		return "", nil
	case "F":
		flags.first = true
		return "", nil
	case "T":
		flags.title = true
		return "", nil
	case "M":
		flags.mixed = true
		return "", nil
	case "TL":
		flags.translit = true
		return "", nil
	case "TR":
		flags.detranslit = true
		return "", nil
	case "DM":
		return item.MTime.Local().Format("2006.01.02"), nil
	case "TM":
		return item.MTime.Local().Format("15-04-05"), nil
	case "R":
		return strconv.Itoa(item.Random), nil
	}

	if strings.HasPrefix(token, "N") {
		return applyRange([]rune(base), token[1:])
	}
	if strings.HasPrefix(token, "E") {
		return applyRange([]rune(ext), token[1:])
	}
	if strings.HasPrefix(token, "C") {
		return expandCounter(token[1:], index)
	}

	meta, _ := item.Metadata()
	switch token {
	case "#":
		return meta.Track, nil
	case "t":
		return safeMetadata(meta.Title), nil
	case "a":
		return safeMetadata(meta.Artist), nil
	case "l":
		return safeMetadata(meta.Album), nil
	case "y":
		return meta.Year, nil
	case "g":
		return safeMetadata(meta.Genre), nil
	case "c":
		return safeMetadata(meta.CameraMake), nil
	case "m":
		return safeMetadata(meta.CameraModel), nil
	case "d":
		return meta.ImageDate, nil
	case "r":
		if meta.Width > 0 && meta.Height > 0 {
			return fmt.Sprintf("%dx%d", meta.Width, meta.Height), nil
		}
		return "", nil
	case "V":
		return meta.Version, nil
	default:
		return "", fmt.Errorf("unknown token [%s]", token)
	}
}

func expandCounter(spec string, index int) (string, error) {
	plus := strings.IndexByte(spec, '+')
	initialText := spec
	stepText := "1"
	if plus >= 0 {
		initialText, stepText = spec[:plus], spec[plus+1:]
	}
	if initialText == "" || len(initialText) > 9 || stepText == "" {
		return "", fmt.Errorf("invalid counter [C%s]", spec)
	}
	for _, r := range initialText {
		if r < '0' || r > '9' {
			return "", fmt.Errorf("invalid counter [C%s]", spec)
		}
	}
	initial, err1 := strconv.ParseInt(initialText, 10, 64)
	step, err2 := strconv.ParseInt(stepText, 10, 64)
	if err1 != nil || err2 != nil {
		return "", fmt.Errorf("invalid counter [C%s]", spec)
	}
	value := initial + int64(index)*step
	if value < 0 {
		return "-" + fmt.Sprintf("%0*d", len(initialText), -value), nil
	}
	return fmt.Sprintf("%0*d", len(initialText), value), nil
}

func applyRange(src []rune, spec string) (string, error) {
	if spec == "" {
		return string(src), nil
	}
	start, end, err := parseRange(spec, len(src))
	if err != nil {
		return "", err
	}
	if start >= len(src) || end < 0 || start > end || len(src) == 0 {
		return "", nil
	}
	if start < 0 {
		start = 0
	}
	if end >= len(src) {
		end = len(src) - 1
	}
	if start > end {
		return "", nil
	}
	return string(src[start : end+1]), nil
}

func parseRange(spec string, length int) (int, int, error) {
	resolve := func(n int) int {
		if n > 0 {
			return n - 1
		}
		return length + n
	}
	parsePos := func(s string) (int, error) {
		n, err := strconv.Atoi(s)
		if err != nil || n == 0 {
			return 0, fmt.Errorf("invalid range [%s]", spec)
		}
		return n, nil
	}

	if comma := strings.IndexByte(spec, ','); comma >= 0 {
		if strings.Count(spec, ",") != 1 {
			return 0, 0, fmt.Errorf("invalid range [%s]", spec)
		}
		s, err := parsePos(spec[:comma])
		count, err2 := strconv.Atoi(spec[comma+1:])
		if err != nil || err2 != nil || count <= 0 {
			return 0, 0, fmt.Errorf("invalid range [%s]", spec)
		}
		start := resolve(s)
		return start, start + count - 1, nil
	}

	if strings.HasSuffix(spec, "-") && len(spec) > 1 {
		s, err := parsePos(spec[:len(spec)-1])
		if err != nil {
			return 0, 0, err
		}
		return resolve(s), length - 1, nil
	}

	sep := -1
	for i := 1; i < len(spec); i++ {
		if spec[i] == '-' {
			sep = i
			break
		}
	}
	if sep >= 0 {
		s, err1 := parsePos(spec[:sep])
		e, err2 := parsePos(spec[sep+1:])
		if err1 != nil || err2 != nil {
			return 0, 0, fmt.Errorf("invalid range [%s]", spec)
		}
		start, end := resolve(s), resolve(e)
		if start < length && end >= 0 && start > end {
			return 0, 0, fmt.Errorf("reversed range [%s]", spec)
		}
		return start, end, nil
	}

	p, err := parsePos(spec)
	if err != nil {
		return 0, 0, err
	}
	idx := resolve(p)
	return idx, idx, nil
}

func safeMetadata(value string) string {
	if strings.ContainsAny(value, "<>:\"*?/\\|") {
		return ""
	}
	return value
}
