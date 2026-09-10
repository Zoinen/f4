package app

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/ini"
	"github.com/unxed/vtui"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

var commandPaletteTranslationsCache struct {
	sync.Mutex
	loaded     bool
	generation uint64
	byKey      map[string][]string
}

// resetCommandPaletteTranslations invalidates installed-pack aliases. A
// language reload does that on its own through i18n.Generation; this is for a
// test that installs a pack behind the cache's back.
func ResetCommandPaletteTranslations() {
	commandPaletteTranslationsCache.Lock()
	commandPaletteTranslationsCache.loaded = false
	commandPaletteTranslationsCache.byKey = nil
	commandPaletteTranslationsCache.Unlock()
}

// commandPaletteTranslations returns invisible search aliases for localization
// keys. The palette still renders DisplayLabel/DisplayDescription through the
// active UI language; aliases only make the same command discoverable by a
// translation from any installed language pack.
func commandPaletteTranslations(keys ...string) []string {
	generation := i18n.Generation()
	commandPaletteTranslationsCache.Lock()
	// i18n.InitLang is also the point where newly installed or user-supplied packs
	// become visible, so a language reload has to rebuild this index too.
	if !commandPaletteTranslationsCache.loaded || commandPaletteTranslationsCache.generation != generation {
		packs := i18n.LoadAllLanguagePacks()
		packs = append(packs, loadInstalledCommandPaletteLanguagePacks()...)
		commandPaletteTranslationsCache.byKey = buildCommandPaletteTranslationIndex(packs)
		commandPaletteTranslationsCache.generation = generation
		commandPaletteTranslationsCache.loaded = true
	}
	byKey := commandPaletteTranslationsCache.byKey
	commandPaletteTranslationsCache.Unlock()

	seen := make(map[string]bool)
	var result []string
	for _, key := range keys {
		for _, value := range byKey[key] {
			normalized := normalizeCommandPaletteText(value)
			if normalized == "" || seen[normalized] {
				continue
			}
			seen[normalized] = true
			result = append(result, value)
		}
	}
	return result
}

func buildCommandPaletteTranslationIndex(packs []vtui.LanguagePack) map[string][]string {
	byKey := make(map[string][]string)
	seen := make(map[string]map[string]bool)
	for _, pack := range packs {
		for key, value := range pack.Strings {
			normalized := normalizeCommandPaletteText(value)
			if normalized == "" {
				continue
			}
			if seen[key] == nil {
				seen[key] = make(map[string]bool)
			}
			if seen[key][normalized] {
				continue
			}
			seen[key][normalized] = true
			byKey[key] = append(byKey[key], value)
		}
	}
	return byKey
}

// Embedded packs cover every language shipped with f4. Disk packs extend the
// index with installed updates and user-supplied languages, following the same
// locations as i18n.InitLang and the language selector.
func loadInstalledCommandPaletteLanguagePacks() []vtui.LanguagePack {
	directories := i18n.SearchDirs(config.LocalLangDir())

	seenPaths := make(map[string]bool)
	var packs []vtui.LanguagePack
	for _, directory := range directories {
		absolute, err := filepath.Abs(directory)
		if err == nil {
			directory = absolute
		}
		canonical := filepath.Clean(directory)
		if runtime.GOOS == "windows" {
			canonical = strings.ToLower(canonical)
		}
		if seenPaths[canonical] {
			continue
		}
		seenPaths[canonical] = true

		paths, err := filepath.Glob(filepath.Join(directory, "*.lng"))
		if err != nil {
			continue
		}
		sort.Strings(paths)
		for _, path := range paths {
			ini := ini.Load(path)
			stringsMap := i18n.LoadLangMapFromINI(ini)
			if len(stringsMap) == 0 {
				continue
			}
			packs = append(packs, vtui.LanguagePack{
				Name:    strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
				Strings: stringsMap,
			})
		}
	}
	return packs
}
