package app

import (
	"path/filepath"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
)

// initLang rebuilds the string table from the configured languages. Naming the
// two settings and the profile directory here is what keeps internal/i18n
// independent of internal/config.
func initLang() {
	i18n.InitLang(config.App.Language, config.App.FallbackLanguage, config.LocalLangDir())
}

// userLangDir is where the running profile keeps separately installed .lng
// files. internal/i18n takes it as an argument rather than reaching for the
// configuration package itself.
func userLangDir() string {
	return filepath.Join(config.GetF4ConfigDir(), "lang")
}
