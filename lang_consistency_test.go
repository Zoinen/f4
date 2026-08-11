package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/abadojack/whatlanggo"
)

func hasHotkey(s string) bool {
	s = strings.ReplaceAll(s, "&&", "")
	idx := strings.Index(s, "&")
	if idx == -1 || idx == len(s)-1 {
		return false
	}
	nextChar := []rune(s[idx+1:])[0]
	return unicode.IsLetter(nextChar) || unicode.IsDigit(nextChar)
}

func TestLangConsistency(t *testing.T) {
	skipIfNoRelevantChanges(t, "lang_consistency",
		"lang/*.lng",
		"lang/*.txt",
		"lang_consistency_test.go",
	)
	enData, err := os.ReadFile(filepath.Join("lang", "en.lng"))
	if err != nil {
		t.Fatalf("Failed to read en.lng: %v", err)
	}

	enIni := ParseIni(bytes.NewReader(enData))
	enStrings := loadLangMapFromINI(enIni)

	var enKeys []string
	enRawStrings := make(map[string]string)
	lines := strings.Split(string(enData), "\n")
	inStringsSection := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[Strings]") {
			inStringsSection = true
			continue
		} else if strings.HasPrefix(line, "[") {
			inStringsSection = false
			continue
		}
		if !inStringsSection || line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx > 0 {
			key := strings.TrimSpace(line[:idx])
			enRawStrings[key] = line[idx+1:]
			if _, ok := enStrings[key]; ok {
				found := false
				for _, k := range enKeys {
					if k == key {
						found = true
						break
					}
				}
				if !found {
					enKeys = append(enKeys, key)
				}
			}
		}
	}

	files, err := filepath.Glob(filepath.Join("lang", "*.lng"))
	if err != nil {
		t.Fatalf("Failed to glob lang/*.lng: %v", err)
	}

	placeholderRe := regexp.MustCompile(`%[sdvq]`)

	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Errorf("Failed to read %s: %v", file, err)
			continue
		}

		ini := ParseIni(bytes.NewReader(data))
		stringsMap := loadLangMapFromINI(ini)

		code := ini.GetString("Language", "Code", "")
		expectedCode := strings.TrimSuffix(filepath.Base(file), ".lng")
		if code != expectedCode {
			t.Errorf("%s: [Language] Code is '%s', expected '%s'", file, code, expectedCode)
		}
		if ini.GetString("Language", "Name", "") == "" {
			t.Errorf("%s: [Language] Name is missing", file)
		}

		if filepath.Base(file) == "en.lng" {
			continue
		}

		seenKeys := make(map[string]bool)
		targetRawStrings := make(map[string]string)
		flines := strings.Split(string(data), "\n")
		targetInStringsSection := false
		for _, line := range flines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "[Strings]") {
				targetInStringsSection = true
				continue
			} else if strings.HasPrefix(line, "[") {
				targetInStringsSection = false
				continue
			}
			if !targetInStringsSection || line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
				continue
			}
			idx := strings.Index(line, "=")
			if idx > 0 {
				key := strings.TrimSpace(line[:idx])
				targetRawStrings[key] = line[idx+1:]
				if seenKeys[key] {
					t.Errorf("%s: Duplicate key found: %s", file, key)
				}
				seenKeys[key] = true

				if _, ok := enStrings[key]; !ok {
					t.Errorf("%s: Key '%s' does not exist in en.lng", file, key)
				}
			} else {
				t.Errorf("%s: Invalid line without '=' in [Strings]: %s", file, line)
			}
		}

		mergedLineRe := regexp.MustCompile(`[a-zA-Z0-9_.-]+=`)
		allowedLangs := map[string][]whatlanggo.Lang{
			"ar": {whatlanggo.Arb, whatlanggo.Eng},
			"bn": {whatlanggo.Ben, whatlanggo.Eng},
			"be": {whatlanggo.Bel, whatlanggo.Rus, whatlanggo.Ukr, whatlanggo.Eng},
			"cs": {whatlanggo.Ces, whatlanggo.Pol, whatlanggo.Hrv, whatlanggo.Srp, whatlanggo.Slv, whatlanggo.Eng, whatlanggo.Deu, whatlanggo.Fra, whatlanggo.Ita, whatlanggo.Spa, whatlanggo.Por, whatlanggo.Hat, whatlanggo.Nld},
			"de": {whatlanggo.Deu, whatlanggo.Eng, whatlanggo.Nld, whatlanggo.Epo, whatlanggo.Fra, whatlanggo.Ita, whatlanggo.Spa},
			"pl": {whatlanggo.Pol, whatlanggo.Eng, whatlanggo.Ces, whatlanggo.Hrv, whatlanggo.Srp, whatlanggo.Slv, whatlanggo.Epo, whatlanggo.Deu, whatlanggo.Fra, whatlanggo.Ita, whatlanggo.Spa},
			"uk": {whatlanggo.Ukr, whatlanggo.Rus, whatlanggo.Bel, whatlanggo.Bul, whatlanggo.Srp, whatlanggo.Eng},
			"ru": {whatlanggo.Rus, whatlanggo.Ukr, whatlanggo.Bel, whatlanggo.Bul, whatlanggo.Srp, whatlanggo.Tuk, whatlanggo.Eng},
			"zh": {whatlanggo.Cmn, whatlanggo.Jpn, whatlanggo.Eng},
			"ja": {whatlanggo.Jpn, whatlanggo.Cmn, whatlanggo.Eng},
			"ko": {whatlanggo.Kor, whatlanggo.Eng},
			"ka": {whatlanggo.Kat, whatlanggo.Eng},
			"hu": {whatlanggo.Hun, whatlanggo.Eng},
			"fi": {whatlanggo.Fin, whatlanggo.Eng},
			"hy": {whatlanggo.Eng},
			"lt": {whatlanggo.Lit, whatlanggo.Eng},
			"lv": {whatlanggo.Lav, whatlanggo.Eng},
			"et": {whatlanggo.Est, whatlanggo.Eng},
			"es": {whatlanggo.Spa, whatlanggo.Eng},
			"he": {whatlanggo.Heb, whatlanggo.Eng},
			"hi": {whatlanggo.Hin, whatlanggo.Eng},
			"tr": {whatlanggo.Tur, whatlanggo.Eng},
		}

		for _, key := range enKeys {
			enVal := enStrings[key]
			val, ok := stringsMap[key]
			if !ok {
				// Missing keys are perfectly fine and explicitly allowed for contributors.
				// The runtime localization engine will elegantly fallback to the English base
				// (or the user's secondary language).
				t.Logf("Tech Debt -> %s: Missing key '%s' (will fallback at runtime)", file, key)
				continue
			}

			enNewlines := strings.Count(enVal, "\n")
			valNewlines := strings.Count(val, "\n")
			if enNewlines != valNewlines {
				t.Errorf("%s: Key '%s' has %d newlines, expected %d", file, key, valNewlines, enNewlines)
			}

			enPlaces := placeholderRe.FindAllString(enVal, -1)
			valPlaces := placeholderRe.FindAllString(val, -1)
			if len(enPlaces) != len(valPlaces) {
				t.Errorf("%s: Key '%s' has %v placeholders, expected %v", file, key, valPlaces, enPlaces)
			} else {
				for i := range enPlaces {
					if enPlaces[i] != valPlaces[i] {
						t.Errorf("%s: Key '%s' placeholder mismatch at %d: %s vs %s", file, key, i, valPlaces[i], enPlaces[i])
					}
				}
			}

			// 1. Anti-merge
			for _, match := range mergedLineRe.FindAllString(val, -1) {
				s := match[:len(match)-1]
				for i := 0; i < len(s); i++ {
					if _, ok := enStrings[s[i:]]; ok {
						t.Errorf("%s: Key '%s' contains a merged line pattern for key '%s'", file, key, s[i:])
						break
					}
				}
			}

			// 4. Whitespace drift
			enRawVal := enRawStrings[key]
			valRawVal := targetRawStrings[key]
			if strings.HasPrefix(enRawVal, " ") != strings.HasPrefix(valRawVal, " ") {
				t.Errorf("%s: Key '%s' leading space mismatch", file, key)
			}
			if strings.HasSuffix(enRawVal, " ") != strings.HasSuffix(valRawVal, " ") {
				t.Errorf("%s: Key '%s' trailing space mismatch", file, key)
			}

			// 5. Ampersand (Hotkey) sanity check
			enHasAmp := hasHotkey(enVal)
			valHasAmp := hasHotkey(val)

			if !enHasAmp && valHasAmp {
				t.Errorf("%s: Key '%s' has unexpected hotkey '&'", file, key)
			}

			if valHasAmp {
				valNoDbl := strings.ReplaceAll(val, "&&", "")
				ampCount := strings.Count(valNoDbl, "&")
				if ampCount > 1 {
					t.Errorf("%s: Key '%s' has multiple single '&'", file, key)
				}
				idx := strings.Index(valNoDbl, "&")
				if idx == len(valNoDbl)-1 {
					t.Errorf("%s: Key '%s' has '&' at the end of the string", file, key)
				} else {
					nextChar := []rune(valNoDbl[idx+1:])[0]
					if !unicode.IsLetter(nextChar) && !unicode.IsDigit(nextChar) {
						t.Errorf("%s: Key '%s' has invalid char after '&': %c", file, key, nextChar)
					}
					if code == "zh" || code == "ja" || code == "ko" {
						if nextChar < 'A' || (nextChar > 'Z' && nextChar < 'a') || nextChar > 'z' {
							t.Errorf("%s: Key '%s' in CJK must use Latin letter for hotkey, got: %c", file, key, nextChar)
						}
					}
				}
			}

			// 2. N-gram language detection
			cleanVal := placeholderRe.ReplaceAllString(val, "")
			cleanVal = strings.ReplaceAll(cleanVal, "&", "")
			if utf8.RuneCountInString(cleanVal) > 50 {
				info := whatlanggo.Detect(cleanVal)
				if info.IsReliable() && info.Confidence > 0.90 {
					allowed := false
					for _, l := range allowedLangs[code] {
						if info.Lang == l {
							allowed = true
							break
						}
					}
					if !allowed && info.Lang != whatlanggo.Eng {
						t.Errorf("%s: Key '%s' detected as %s with high confidence (%.2f)", file, key, info.Lang.String(), info.Confidence)
					}
				}
			}
		}
	}

	baselineData, err := os.ReadFile(filepath.Join("lang", "coverage_baseline.txt"))
	if err == nil {
		baselineLines := strings.Split(string(baselineData), "\n")
		for _, line := range baselineLines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.Split(line, "=")
			if len(parts) == 2 {
				code := parts[0]
				var expectedCount int
				fmt.Sscanf(parts[1], "%d", &expectedCount)

				file := filepath.Join("lang", code+".lng")
				data, err := os.ReadFile(file)
				if err != nil {
					t.Errorf("Coverage baseline requires %s but file is missing", code)
					continue
				}
				ini := ParseIni(bytes.NewReader(data))
				stringsMap := loadLangMapFromINI(ini)
				if len(stringsMap) < expectedCount {
					t.Errorf("%s has %d keys, baseline requires at least %d", code, len(stringsMap), expectedCount)
				}
			}
		}
	}
}
func TestAntiMergeLogic(t *testing.T) {
	enStrings := map[string]string{
		"LanguageSettings.Title": "Language Settings",
		"FileOp.Resume":          "Resume",
		"Some.OtherKey":          "Value",
	}

	mergedLineRe := regexp.MustCompile(`[a-zA-Z0-9_.-]+=`)

	checkMerged := func(val string) (string, bool) {
		for _, match := range mergedLineRe.FindAllString(val, -1) {
			s := match[:len(match)-1]
			for i := 0; i < len(s); i++ {
				if _, ok := enStrings[s[i:]]; ok {
					return s[i:], true
				}
			}
		}
		return "", false
	}

	tests := []struct {
		input     string
		wantKey   string
		wantFound bool
	}{
		{"Аднав&іцьLanguageSettings.Title= Налады мовы", "LanguageSettings.Title", true},
		{"ResumeLanguageSettings.Title= Налады мовы", "LanguageSettings.Title", true},
		{"This is a normal translation.", "", false},
		{"Some value = 42", "", false},
		{"NotAKey.Title= some text", "", false},
		{"FirstSome.OtherKey=SecondLanguageSettings.Title=foo", "Some.OtherKey", true},
	}

	for _, tt := range tests {
		gotKey, gotFound := checkMerged(tt.input)
		if gotFound != tt.wantFound {
			t.Errorf("checkMerged(%q) found = %v, want %v", tt.input, gotFound, tt.wantFound)
		}
		if gotFound && gotKey != tt.wantKey {
			t.Errorf("checkMerged(%q) found key = %q, want %q", tt.input, gotKey, tt.wantKey)
		}
	}
}
