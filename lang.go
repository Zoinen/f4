package main

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/unxed/vtui"
)

//go:embed lang/en.lng
var defaultLangData string

// Keep vtui's own small built-in table when InitLang replaces an earlier UI
// language. The replacement prevents untranslated keys from leaking in from
// whichever language happened to be active before the switch.
var vtuiBuiltInStrings = vtui.SnapshotStrings()

var languageState struct {
	sync.Mutex
	// core is the exact table produced by the most recent InitLang before
	// runtime/plugin overlays were reapplied.
	core map[string]string
}

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

func loadEmbeddedLanguageMap(code string) map[string]string {
	data, err := langPackFS.ReadFile("lang/" + code + ".lng")
	if err != nil {
		return nil
	}
	return loadLangMapFromINI(ParseIni(strings.NewReader(string(data))))
}

// InitLang transfers all f4 strings to vtui localization engine.
func InitLang() {
	languageState.Lock()
	defer languageState.Unlock()

	// vtui.AddStrings is a public runtime extension point used by in-process
	// plugins. Keep values that differ from the previous core language table so
	// replacing that core on a language switch does not erase plugin dialogs.
	runtimeOverlays := make(map[string]string)
	if languageState.core != nil {
		for key, value := range vtui.SnapshotStrings() {
			if previous, coreKey := languageState.core[key]; !coreKey || previous != value {
				runtimeOverlays[key] = value
			}
		}
	}

	primary := AppConfig.Language
	if primary == "" {
		primary = "en"
	}
	fallback := AppConfig.FallbackLanguage

	// 1. Always load embedded English as absolute fallback (Tier 1)
	embedIni := ParseIni(strings.NewReader(defaultLangData))
	baseMap := loadLangMapFromINI(embedIni)
	allBaseStrings := make(map[string]string, len(vtuiBuiltInStrings)+len(baseMap))
	for key, value := range vtuiBuiltInStrings {
		allBaseStrings[key] = value
	}
	for key, value := range baseMap {
		allBaseStrings[key] = value
	}
	vtui.ReplaceStrings(allBaseStrings)

	exeDir := filepath.Dir(os.Args[0])
	userDir := filepath.Join(GetF4ConfigDir(), "lang")

	loadLang := func(code string) {
		// Use the version embedded in this binary as the language baseline. A
		// separately installed or development-time .lng file may lag behind the
		// executable; loading it only as an overlay keeps new strings in the
		// selected UI language while preserving user overrides.
		if embedded := loadEmbeddedLanguageMap(code); len(embedded) > 0 {
			vtui.AddStrings(embedded)
		}
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

	languageState.core = vtui.SnapshotStrings()
	vtui.AddStrings(runtimeOverlays)
	resetCommandPaletteTranslations()
}
