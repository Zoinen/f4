package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

func TestLanguageAlphabetsContamination(t *testing.T) {
	skipIfNoRelevantChanges(t, "lang_scripts",
		"lang/*.lng",
		"help/*.hlf",
		"lang_scripts_test.go",
	)
	// A map of scripts that are strictly bound to specific languages.
	// Latin and Common (punctuation, numbers) are allowed everywhere,
	// so they are intentionally omitted from this restriction list.
	restrictedScripts := map[string]*unicode.RangeTable{
		"Cyrillic":   unicode.Cyrillic,
		"Hangul":     unicode.Hangul,
		"Hiragana":   unicode.Hiragana,
		"Katakana":   unicode.Katakana,
		"Han":        unicode.Han,
		"Georgian":   unicode.Georgian,
		"Armenian":   unicode.Armenian,
		"Greek":      unicode.Greek,
		"Arabic":     unicode.Arabic,
		"Hebrew":     unicode.Hebrew,
		"Thai":       unicode.Thai,
		"Devanagari": unicode.Devanagari,
	}

	// Which language codes are allowed to contain the scripts above.
	allowedLangsForScript := map[string]map[string]bool{
		"Cyrillic":   {"ru": true, "uk": true, "be": true, "bg": true},
		"Hangul":     {"ko": true},
		"Hiragana":   {"ja": true},
		"Katakana":   {"ja": true},
		"Han":        {"zh": true, "ja": true, "ko": true},
		"Georgian":   {"ka": true},
		"Armenian":   {"hy": true},
		"Greek":      {"el": true},
		"Arabic":     {"ar": true, "fa": true, "ur": true},
		"Hebrew":     {"he": true},
		"Thai":       {"th": true},
		"Devanagari": {"hi": true},
	}

	dirs := []string{"lang"}

	for _, dir := range dirs {
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, file := range files {
			if file.IsDir() {
				continue
			}

			name := file.Name()
			if !strings.HasSuffix(name, ".lng") && !strings.HasSuffix(name, ".hlf") {
				continue
			}

			langCode := strings.Split(name, ".")[0]

			path := filepath.Join(dir, name)
			content, err := os.ReadFile(path)
			if err != nil {
				t.Errorf("Failed to read %s: %v", path, err)
				continue
			}

			lines := strings.Split(string(content), "\n")
			for i, line := range lines {
				for _, r := range line {
					for scriptName, scriptTable := range restrictedScripts {
						if unicode.Is(scriptTable, r) {
							if !allowedLangsForScript[scriptName][langCode] {
								t.Errorf("%s:%d: Found unexpected %s character %q in %s file", path, i+1, scriptName, r, langCode)
							}
						}
					}
				}
			}
		}
	}
}
