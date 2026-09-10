package i18n

import (
	_ "embed"
	"github.com/unxed/f4/internal/ini"
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	// generation counts completed InitLang runs. See Generation.
	generation uint64
}

// Generation counts how many times the string table has been replaced. A cache
// built from the table compares it and rebuilds when it moves.
//
// A pull rather than a push on purpose: the alternative is a callback this
// package holds and the root assigns, and a callback nobody assigned leaves the
// cache stale with no symptom but a menu entry that answers to the wrong word.
// A counter cannot be left unwired.
func Generation() uint64 {
	languageState.Lock()
	defer languageState.Unlock()
	return languageState.generation
}

// Msg is a proxy for vtui.Msg to keep f4 code clean.
func Msg(key string) string {
	return vtui.Msg(key)
}

func init() {
	// Initial load for tests. SetupUI calls this again once the configuration
	// has been read and knows which languages were asked for.
	InitLang("", "", "")
}

func LoadLangMapFromINI(ini *ini.File) map[string]string {
	m := make(map[string]string)
	if sec, ok := ini.Sections()["Strings"]; ok {
		for k, v := range sec {
			// Unescape newlines
			m[k] = strings.ReplaceAll(v, "\\n", "\n")
		}
	}
	return m
}

func LoadEmbeddedLanguageMap(code string) map[string]string {
	data, err := LangPackFS.ReadFile("lang/" + code + ".lng")
	if err != nil {
		return nil
	}
	return LoadLangMapFromINI(ini.Parse(strings.NewReader(string(data))))
}

func SafeLanguageCode(code string) bool {
	return code != "" && !strings.Contains(code, "..") && !strings.ContainsAny(code, `/\`)
}

// InitLang transfers all f4 strings to vtui's localization engine: primary
// first, then fallback, over the embedded English. userLangDir is where
// separately installed .lng files are looked for, and may be empty.
//
// All three are arguments rather than reads of the configuration, the same way
// history.NewF4HistoryProvider takes its directory. This package is a leaf, and
// a leaf that imports internal/config is a leaf the configuration's own tests
// cannot use — netfox's proxy test imports the plugin, the plugin imports this
// package, and the cycle closes.
func InitLang(primary, fallback, userLangDir string) {
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

	if primary == "" {
		primary = "en"
	}
	if !SafeLanguageCode(primary) {
		primary = "en"
	}
	if fallback != "" && !SafeLanguageCode(fallback) {
		fallback = ""
	}

	// 1. Always load embedded English as absolute fallback (Tier 1)
	embedIni := ini.Parse(strings.NewReader(defaultLangData))
	baseMap := LoadLangMapFromINI(embedIni)
	allBaseStrings := make(map[string]string, len(vtuiBuiltInStrings)+len(baseMap))
	for key, value := range vtuiBuiltInStrings {
		allBaseStrings[key] = value
	}
	for key, value := range baseMap {
		allBaseStrings[key] = value
	}
	vtui.ReplaceStrings(allBaseStrings)

	loadLang := func(code string) {
		if !SafeLanguageCode(code) {
			return
		}
		// Use the version embedded in this binary as the language baseline. A
		// separately installed or development-time .lng file may lag behind the
		// executable; loading it only as an overlay keeps new strings in the
		// selected UI language while preserving user overrides.
		if embedded := LoadEmbeddedLanguageMap(code); len(embedded) > 0 {
			vtui.AddStrings(embedded)
		}
		var langIni *ini.File
		for _, dir := range SearchDirs(userLangDir) {
			cand := filepath.Join(dir, code+".lng")
			// #nosec G703 -- SafeLanguageCode rejects separators and ".." before code is used as a path component.
			if _, err := os.Stat(cand); err == nil {
				langIni = ini.Load(cand)
				vtui.DebugLog("LANG: Loaded language file from disk: %s", cand)
				break
			}
		}
		if langIni != nil {
			overlayMap := LoadLangMapFromINI(langIni)
			vtui.AddStrings(overlayMap)
		} else {
			vtui.DebugLog("LANG: Warning - language file for '%s' not found.", code)
		}
	}

	// 2. Load Fallback language if configured (Tier 2). A fallback only
	// fills keys the primary lacks; with an English primary the embedded
	// base already covers everything, so loading the fallback would
	// override the primary instead of backing it up.
	primaryIsEnglish := primary == "en" || primary == "eng"
	if fallback != "" && fallback != "en" && fallback != primary && !primaryIsEnglish {
		loadLang(fallback)
	}

	// 3. Load Primary language (Tier 3)
	if !primaryIsEnglish {
		loadLang(primary)
	} else {
		vtui.DebugLog("LANG: Primary is English, relying on base.")
	}

	languageState.core = vtui.SnapshotStrings()
	languageState.generation++
	vtui.AddStrings(runtimeOverlays)
}
