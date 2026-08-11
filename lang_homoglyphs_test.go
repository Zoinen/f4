package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"unicode"
)

// homoglyphScripts are the alphabetic scripts we are able to tell apart.
// Mixing two of them inside a single word is the fingerprint of machine
// translation damage: a letter silently replaced by a look-alike from another
// alphabet (Hebrew vav -> Arabic waw, Cyrillic er -> Latin p, and so on).
// Such a string still renders "almost right", so it survives a visual review,
// but it breaks search, sorting and any font that lacks the foreign glyph.
//
// TestLanguageAlphabetsContamination catches the subset where the foreign
// script is forbidden for that language outright. This test is the complement:
// it also catches damage inside a language that is allowed to use the script
// (Latin inside Cyrillic, Bengali inside Georgian), because the unit of
// suspicion here is the word, not the file.
var homoglyphScripts = []struct {
	name  string
	table *unicode.RangeTable
}{
	{"Latin", unicode.Latin},
	{"Cyrillic", unicode.Cyrillic},
	{"Greek", unicode.Greek},
	{"Hebrew", unicode.Hebrew},
	{"Arabic", unicode.Arabic},
	{"Armenian", unicode.Armenian},
	{"Georgian", unicode.Georgian},
	{"Devanagari", unicode.Devanagari},
	{"Bengali", unicode.Bengali},
	{"Thai", unicode.Thai},
	{"Han", unicode.Han},
	{"Hangul", unicode.Hangul},
	{"Hiragana", unicode.Hiragana},
	{"Katakana", unicode.Katakana},
}

// wordFriendlyScripts may share a word without raising suspicion: CJK and
// Korean text is written without spaces, so "Colorer设置", "VisRen은" and
// "ID3タグ" are perfectly normal. Everything else is separated by punctuation
// in a healthy file ("ID3-теги", "f4-ից", "Colorer-ის"), which splits the
// word before this check ever sees it.
var wordFriendlyScripts = map[string]bool{
	"Latin":    true,
	"Han":      true,
	"Hangul":   true,
	"Hiragana": true,
	"Katakana": true,
}

func scriptOfRune(r rune) string {
	for _, s := range homoglyphScripts {
		if unicode.Is(s.table, r) {
			return s.name
		}
	}
	return ""
}

// splitIntoScriptWords cuts a string on everything that is neither a letter
// nor a combining mark, so digits, punctuation and format characters act as
// word separators.
func splitIntoScriptWords(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsMark(r)
	})
}

func scriptsInWord(word string) []string {
	seen := make(map[string]bool)
	for _, r := range word {
		if name := scriptOfRune(r); name != "" {
			seen[name] = true
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func allWordFriendly(scripts []string) bool {
	for _, s := range scripts {
		if !wordFriendlyScripts[s] {
			return false
		}
	}
	return true
}

// homoglyphBaselinePath lists the findings that are known and not repaired
// yet, one "<path>:<line>" per line. It exists so this check can guard
// everything else from today instead of waiting until the whole backlog is
// worked off, the same trick tools/hardcoded_baseline.txt plays for hardcoded
// strings. The list may only shrink: anything not in it fails the test.
const homoglyphBaselinePath = "lang/homoglyph_baseline.txt"

func loadHomoglyphBaseline(t *testing.T) map[string]bool {
	baseline := make(map[string]bool)
	data, err := os.ReadFile(homoglyphBaselinePath)
	if err != nil {
		if os.IsNotExist(err) {
			return baseline
		}
		t.Fatalf("cannot read %s: %v", homoglyphBaselinePath, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		baseline[line] = true
	}
	return baseline
}

func TestTranslationsAreFreeOfHomoglyphs(t *testing.T) {
	skipIfNoRelevantChanges(t, "lang_homoglyphs",
		"lang/*.lng",
		"lang/*.txt",
		"help/*.hlf",
		"lang_homoglyphs_test.go",
	)

	paths, err := filepath.Glob("lang/*.lng")
	if err != nil {
		t.Fatalf("cannot list language files: %v", err)
	}
	helpPaths, err := filepath.Glob("help/*.hlf")
	if err != nil {
		t.Fatalf("cannot list help files: %v", err)
	}
	paths = append(paths, helpPaths...)

	if len(paths) == 0 {
		t.Fatal("no localization files found, is the test running from the repository root?")
	}

	baseline := loadHomoglyphBaseline(t)
	seen := make(map[string]bool)

	escapes := strings.NewReplacer(`\n`, " ", `\t`, " ", `\r`, " ")

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("cannot read %s: %v", path, err)
			continue
		}

		isLng := strings.HasSuffix(path, ".lng")
		for i, line := range strings.Split(string(data), "\n") {
			text := line
			if isLng {
				if idx := strings.Index(line, "="); idx > 0 {
					text = line[idx+1:]
				}
			}
			text = escapes.Replace(text)

			for _, word := range splitIntoScriptWords(text) {
				scripts := scriptsInWord(word)
				if len(scripts) < 2 || allWordFriendly(scripts) {
					continue
				}
				location := fmt.Sprintf("%s:%d", path, i+1)
				if baseline[location] {
					seen[location] = true
					t.Logf("Tech Debt -> %s: word %q mixes scripts %s (known, listed in %s)",
						location, word, strings.Join(scripts, "+"), homoglyphBaselinePath)
					continue
				}
				t.Errorf("%s: word %q mixes scripts %s, most likely a look-alike letter from another alphabet",
					location, word, strings.Join(scripts, "+"))
			}
		}
	}

	stale := make([]string, 0, len(baseline))
	for location := range baseline {
		if !seen[location] {
			stale = append(stale, location)
		}
	}
	sort.Strings(stale)
	for _, location := range stale {
		t.Logf("%s: %s is clean now, drop it from the baseline", homoglyphBaselinePath, location)
	}
}
