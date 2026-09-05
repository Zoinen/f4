package main

import (
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// discoverInstalledGuiFonts is a variable so the settings dialog and the
// language recommendation can share one catalog while tests can avoid
// depending on the host's installed fonts.
var discoverInstalledGuiFonts = platformGuiFontFiles

// guiFontChoices keeps the current value even when it is a manually entered
// path or family name that the platform catalog cannot discover.
func guiFontChoices(language, current string) []string {
	return guiFontChoicesFromInstalled(current, discoverInstalledGuiFonts(language))
}

func guiFontChoicesFromInstalled(current string, installed []string) []string {
	choices := make([]string, 0)
	appendUnique := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		for _, existing := range choices {
			if sameGuiFontValue(existing, value) {
				return
			}
		}
		choices = append(choices, value)
	}

	appendUnique(current)
	for _, path := range installed {
		appendUnique(path)
	}
	return choices
}

// platformGuiFontDisplayChoices and platformGuiFontDisplayName are indirection
// variables so the non-Windows build never references the Windows-only font
// name helpers: those live in the //go:build windows file and override these
// from init there. The non-Windows default uses the font file's short name.
var platformGuiFontDisplayChoices = func(language, current string) []string {
	installed := discoverInstalledGuiFonts(language)
	choices := make([]string, 0)
	seen := make(map[string]struct{})
	for _, value := range guiFontChoicesFromInstalled(current, installed) {
		display := guiFontDisplayValueFromInstalled(value, installed)
		if display == "" {
			continue
		}
		key := strings.ToLower(display)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		choices = append(choices, display)
	}
	return choices
}

var platformGuiFontDisplayName = defaultGuiFontDisplayName

// platformGuiFontDisplayNameFromInstalled is the same mapping as
// platformGuiFontDisplayName, but receives the catalog used for the current
// call. Windows uses that catalog to keep tests and callers that provide a
// custom discovery function isolated from the live registry.
var platformGuiFontDisplayNameFromInstalled = func(value string, _ []string) string {
	return platformGuiFontDisplayName(value)
}

// guiFontDisplayChoices returns the strings shown in the font picker. On
// Windows these are font family names (e.g. "Cascadia Mono"); on other
// platforms they are short names derived from the discovered font files.
func guiFontDisplayChoices(language, current string) []string {
	return platformGuiFontDisplayChoices(language, current)
}

// guiFontDisplayValue shortens discovered file paths for the picker but keeps
// an unknown manually entered path intact. The latter is important: replacing
// a custom path with its basename would make merely opening and accepting the
// settings dialog silently point at another file.
func guiFontDisplayValue(language, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return guiFontDisplayValueFromInstalled(value, discoverInstalledGuiFonts(language))
}

func guiFontDisplayValueFromInstalled(value string, installed []string) string {
	for _, path := range installed {
		if sameGuiFontValue(value, path) {
			return platformGuiFontDisplayNameFromInstalled(value, installed)
		}
	}
	return value
}

func guiFontCurrentDisplayName(language, current string) string {
	return guiFontDisplayValue(language, current)
}

// guiFontValueForDisplay converts a picker label back to the value consumed by
// the GUI backend. Manual input that is not one of the catalog labels passes
// through unchanged.
func guiFontValueForDisplay(language, current, display string) string {
	display = strings.TrimSpace(display)
	if display == "" {
		return ""
	}
	installed := discoverInstalledGuiFonts(language)
	for _, value := range guiFontChoicesFromInstalled(current, installed) {
		if strings.EqualFold(guiFontDisplayValueFromInstalled(value, installed), display) {
			return value
		}
	}
	return display
}

func sameGuiFontValue(left, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func shouldSuggestFontForLanguage(language, current string) bool {
	if !isCJKLanguage(language) {
		return false
	}
	if strings.TrimSpace(current) == "" {
		return len(discoverInstalledGuiFonts(language)) > 0
	}
	for _, path := range discoverInstalledGuiFonts(language) {
		if sameGuiFontValue(current, path) {
			return false
		}
	}
	return true
}

func isCJKLanguage(language string) bool {
	language = strings.ToLower(strings.TrimSpace(language))
	if language == "" {
		return false
	}
	for _, prefix := range []string{"zh", "ja", "ko"} {
		if language == prefix || strings.HasPrefix(language, prefix+"_") || strings.HasPrefix(language, prefix+"-") {
			return true
		}
	}
	return false
}

func cjkFontconfigPattern(language string) string {
	language = strings.ToLower(strings.TrimSpace(language))
	switch {
	case strings.HasPrefix(language, "ja"):
		return ":lang=ja"
	case strings.HasPrefix(language, "ko"):
		return ":lang=ko"
	case strings.HasPrefix(language, "zh"):
		return ":lang=zh"
	default:
		// 100 is fontconfig's spacing value for monospace fonts. The
		// explicit value is more portable than the human-readable alias.
		return ":spacing=100"
	}
}

func isFontFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".otc", ".otf", ".ttc", ".ttf":
		return true
	default:
		return false
	}
}

func sortCJKFontPaths(paths []string, language string) {
	if !isCJKLanguage(language) {
		sort.Strings(paths)
		return
	}
	sort.SliceStable(paths, func(i, j int) bool {
		iCJK := looksLikeCJKFontPath(paths[i])
		jCJK := looksLikeCJKFontPath(paths[j])
		if iCJK != jCJK {
			return iCJK
		}
		return paths[i] < paths[j]
	})
}

func looksLikeCJKFontPath(path string) bool {
	path = strings.ToLower(path)
	for _, marker := range []string{
		"cjk", "chinese", "droid", "gothic", "han", "japan", "jp", "korea", "ko", "ming", "noto", "simsun", "song", "wqy", "yahei",
	} {
		if strings.Contains(path, marker) {
			return true
		}
	}
	return false
}

func parseFontconfigPaths(output string) []string {
	var paths []string
	seen := make(map[string]struct{})
	for _, line := range strings.Split(output, "\n") {
		path := strings.TrimSpace(line)
		if path == "" || !isFontFile(path) {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func defaultGuiFontDisplayName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	base := filepath.Base(value)
	if isFontFile(base) {
		base = strings.TrimSuffix(base, filepath.Ext(base))
	}
	return base
}
