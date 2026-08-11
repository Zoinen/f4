package main

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"

	"github.com/unxed/vtui"
)

//go:embed lang/en.lng
var defaultLangData string

// Msg is a proxy for vtui.Msg to keep f4 code clean.
func Msg(key string) string {
	return vtui.Msg(key)
}

func init() {
	// Initial load for tests. SetupUI will call this again after LoadConfig.
	InitLang()
}

func loadLangMapFromINI(ini *IniFile) map[string]string {
	m := make(map[string]string)
	if sec, ok := ini.data["Strings"]; ok {
		for k, v := range sec {
			// Unescape newlines
			m[k] = strings.ReplaceAll(v, "\\n", "\n")
		}
	}
	return m
}

// InitLang transfers all f4 strings to vtui localization engine.
func InitLang() {
	primary := AppConfig.Language
	if primary == "" {
		primary = "en"
	}
	fallback := AppConfig.FallbackLanguage

	// 1. Always load embedded English as absolute fallback (Tier 1)
	embedIni := ParseIni(strings.NewReader(defaultLangData))
	baseMap := loadLangMapFromINI(embedIni)
	vtui.AddStrings(baseMap)

	exeDir := filepath.Dir(os.Args[0])
	userDir := filepath.Join(GetF4ConfigDir(), "lang")

	loadLang := func(code string) {
		candidates := []string{
			filepath.Join(userDir, code+".lng"),
			filepath.Join(exeDir, "lang", code+".lng"),
			filepath.Join("lang", code+".lng"), // Fallback for "go run ." development
		}
		var langIni *IniFile
		for _, cand := range candidates {
			if _, err := os.Stat(cand); err == nil {
				langIni = LoadIni(cand)
				vtui.DebugLog("LANG: Loaded language file from disk: %s", cand)
				break
			}
		}
		if langIni != nil {
			overlayMap := loadLangMapFromINI(langIni)
			vtui.AddStrings(overlayMap)
		} else {
			vtui.DebugLog("LANG: Warning - language file for '%s' not found.", code)
		}
	}

	// 2. Load Fallback language if configured (Tier 2)
	if fallback != "" && fallback != "en" && fallback != primary {
		loadLang(fallback)
	}

	// 3. Load Primary language (Tier 3)
	if primary != "en" && primary != "eng" {
		loadLang(primary)
	} else {
		vtui.DebugLog("LANG: Primary is English, relying on base.")
	}
}
