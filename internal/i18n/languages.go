package i18n

import (
	"github.com/unxed/f4/internal/ini"
	"os"
	"path/filepath"
	"strings"
)

// Language is one selectable UI language: the code its .lng file is named
// after, and the display name that file gives itself.
type Language struct {
	Code string
	Name string
}

// SearchDirs are the directories a .lng file is looked for in, most preferred
// first: the user's profile, then beside the executable, then ./lang for
// `go run .`. userLangDir may be empty, and is skipped when it is.
//
// One list, because a fourth location added to one caller and not the others is
// a translation that appears in the menu and not in the command palette.
//
// LangPackFS's "lang" is deliberately not here. That path is relative to the
// .go file carrying the //go:embed line and only coincides with the runtime
// name; the directory in the repository may be renamed one day, and the
// directory in a user's profile may not.
func SearchDirs(userLangDir string) []string {
	dirs := make([]string, 0, 3)
	if userLangDir != "" {
		dirs = append(dirs, userLangDir)
	}
	return append(dirs, filepath.Join(filepath.Dir(os.Args[0]), "lang"), "lang")
}

// ListAvailable enumerates the UI languages this build can switch to: every
// .lng embedded in the binary, plus any found in SearchDirs.
func ListAvailable(userLangDir string) []Language {
	langs := []Language{{Code: "en", Name: "English"}}
	seen := map[string]bool{"en": true}

	// Every .lng shipped with f4 is embedded in the binary (LangPackFS), and
	// InitLang loads the configured language from there even when no lang/
	// directory exists on disk. The dialog has to enumerate the same set:
	// built from disk alone it offered English only, silently misrepresenting
	// a configured non-English language as "English" (selection fell back to
	// item 0) with no way to see or change the real setting.
	if entries, err := LangPackFS.ReadDir("lang"); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".lng") {
				continue
			}
			code := strings.TrimSuffix(e.Name(), ".lng")
			if seen[code] {
				continue
			}
			data, err := LangPackFS.ReadFile("lang/" + e.Name())
			if err != nil {
				continue
			}
			ini := ini.Parse(strings.NewReader(string(data)))
			langs = append(langs, Language{Code: code, Name: ini.GetString("Language", "Name", code)})
			seen[code] = true
		}
	}

	// Packs on disk extend the embedded set (user-supplied translations).
	for _, d := range SearchDirs(userLangDir) {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".lng") {
				code := strings.TrimSuffix(e.Name(), ".lng")
				if !seen[code] {
					ini := ini.Load(filepath.Join(d, e.Name()))
					name := ini.GetString("Language", "Name", code)
					langs = append(langs, Language{Code: code, Name: name})
					seen[code] = true
				}
			}
		}
	}
	return langs
}

// LanguageName is the display name of one language code, read from its .lng
// file. It falls back to the code in upper case, which is what the settings
// dialog shows for a pack that names itself nothing.
func LanguageName(code, userLangDir string) string {
	if code == "en" || code == "eng" {
		return "English"
	}
	if !SafeLanguageCode(code) {
		return strings.ToUpper(code)
	}
	for _, dir := range SearchDirs(userLangDir) {
		cand := filepath.Join(dir, code+".lng")
		// #nosec G703 -- SafeLanguageCode rejects separators and ".." before code is used as a path component.
		if _, err := os.Stat(cand); err == nil {
			ini := ini.Load(cand)
			if name := ini.GetString("Language", "Name", ""); name != "" {
				return name
			}
		}
	}
	return strings.ToUpper(code)
}
