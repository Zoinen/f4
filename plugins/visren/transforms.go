package visren

import (
	"fmt"
	"strings"
	"unicode"
)

var translitTable = []struct{ cyr, latin string }{
	{"а", "a"}, {"б", "b"}, {"в", "v"}, {"г", "g"}, {"д", "d"},
	{"е", "e"}, {"ё", "jo"}, {"ж", "zh"}, {"з", "z"}, {"и", "i"},
	{"й", "j"}, {"к", "k"}, {"л", "l"}, {"м", "m"}, {"н", "n"},
	{"о", "o"}, {"п", "p"}, {"р", "r"}, {"с", "s"}, {"т", "t"},
	{"у", "u"}, {"ф", "f"}, {"х", "kh"}, {"ц", "c"}, {"ч", "ch"},
	{"ш", "sh"}, {"щ", "shh"}, {"ъ", "`"}, {"ы", "y"}, {"ь", "'"},
	{"э", "eh"}, {"ю", "ju"}, {"я", "ja"},
}

func applyFlags(value string, f maskFlags, wordDiv string) string {
	if f.translit {
		value = transliterate(value)
	} else if f.detranslit {
		value = detransliterate(value)
	}
	switch {
	case f.lower:
		return strings.ToLower(value)
	case f.upper:
		return strings.ToUpper(value)
	case f.first:
		r := []rune(strings.ToLower(value))
		if len(r) > 0 {
			r[0] = unicode.ToUpper(r[0])
		}
		return string(r)
	case f.title:
		return titleCase(value, wordDiv)
	case f.mixed:
		return mixedCase(value, wordDiv)
	default:
		return value
	}
}

func titleCase(value, delimiters string) string {
	runes := []rune(strings.ToLower(value))
	start := true
	for i, r := range runes {
		if start {
			runes[i] = unicode.ToUpper(r)
		}
		start = strings.ContainsRune(delimiters, r)
	}
	return string(runes)
}

func mixedCase(value, delimiters string) string {
	runes := []rune(value)
	separator := -1
	for idx := 1; idx+2 < len(runes); idx++ {
		isSeparator := string(runes[idx:idx+3]) == " - " || string(runes[idx:idx+3]) == "_-_"
		if isSeparator && unicode.IsLetter(runes[idx-1]) {
			separator = idx
			break
		}
	}
	artistEnd := len(runes)
	if separator >= 0 {
		artistEnd = separator
	}
	for idx := 0; idx < artistEnd; idx++ {
		if idx == 0 || strings.ContainsRune(delimiters, runes[idx-1]) {
			runes[idx] = unicode.ToUpper(runes[idx])
		} else {
			runes[idx] = unicode.ToLower(runes[idx])
		}
		// The original has a Russian-specific exception for the isolated
		// conjunction "И" in an artist name.
		if idx > 0 && idx+1 < artistEnd && runes[idx] == 'И' && strings.ContainsRune(delimiters, runes[idx-1]) && strings.ContainsRune(delimiters, runes[idx+1]) {
			runes[idx] = 'и'
		}
	}
	if separator >= 0 && separator+3 < len(runes) {
		runes[separator+3] = unicode.ToUpper(runes[separator+3])
		for idx := separator + 4; idx < len(runes); idx++ {
			runes[idx] = unicode.ToLower(runes[idx])
		}
	}
	return string(runes)
}

func transliterate(value string) string {
	var out strings.Builder
	for _, r := range value {
		lower := strings.ToLower(string(r))
		found := false
		for _, pair := range translitTable {
			if lower == pair.cyr {
				repl := pair.latin
				if unicode.IsUpper(r) && repl != "" {
					repl = strings.ToUpper(repl)
				}
				out.WriteString(repl)
				found = true
				break
			}
		}
		if !found {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func detransliterate(value string) string {
	var out strings.Builder
	for pos := 0; pos < len(value); {
		bestLen := 0
		best := ""
		for _, pair := range translitTable {
			if len(pair.latin) <= bestLen || pos+len(pair.latin) > len(value) {
				continue
			}
			part := value[pos : pos+len(pair.latin)]
			if strings.EqualFold(part, pair.latin) {
				bestLen, best = len(pair.latin), pair.cyr
				if first, _ := utf8FirstRune(part); unicode.IsUpper(first) {
					best = strings.ToUpper(best)
				}
			}
		}
		if bestLen == 0 {
			r := rune(value[pos])
			size := 1
			if r >= 0x80 {
				r, size = utf8FirstRune(value[pos:])
			}
			out.WriteRune(r)
			pos += size
		} else {
			out.WriteString(best)
			pos += bestLen
		}
	}
	return out.String()
}

func utf8FirstRune(s string) (rune, int) {
	for _, r := range s {
		return r, len(string(r))
	}
	return 0, 0
}

func ValidateFilename(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("empty or reserved file name")
	}
	if strings.ContainsAny(name, "<>:\"*?/\\|") {
		return fmt.Errorf("file name contains a forbidden character")
	}
	for _, r := range name {
		if r == 0 || r < 32 {
			return fmt.Errorf("file name contains a control character")
		}
	}
	if strings.HasSuffix(name, " ") || strings.HasSuffix(name, ".") {
		return fmt.Errorf("file name ends with a space or dot")
	}
	base, _ := SplitName(name)
	upper := strings.ToUpper(strings.TrimSpace(base))
	reserved := map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true}
	if len(upper) == 4 && (strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")) && upper[3] >= '1' && upper[3] <= '9' {
		reserved[upper] = true
	}
	if reserved[upper] {
		return fmt.Errorf("reserved DOS device name %q", base)
	}
	return nil
}
