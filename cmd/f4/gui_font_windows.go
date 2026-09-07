//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
)

type fontEntry struct {
	base string
	file string
}

// windowsFontFile resolves a Windows font family name (as shown in settings)
// to the actual font file path via the registry. vtui's getFontCandidates only
// matches file names literally, so "Cascadia Mono" would otherwise miss
// CascadiaMono.ttf and silently fall back to Consolas. Returns "" if the
// family is not found; callers then keep the original name.
func windowsFontFile(fontName string) string {
	return matchWindowsFontFamily(fontName, windowsFontEntries())
}

func matchWindowsFontFamily(fontName string, entries []fontEntry) string {
	fontName = strings.TrimSpace(fontName)
	if fontName == "" {
		return ""
	}
	want := strings.ToLower(fontName)

	for _, e := range entries {
		if strings.ToLower(e.base) == want {
			return fontFilePath(e.file)
		}
	}
	// The registry records families with their style, e.g. "Cascadia Mono
	// Regular", so accept family + " <style>". Prefer the regular weight when
	// more than one style is installed.
	var first string
	for _, e := range entries {
		got := strings.ToLower(e.base)
		if !strings.HasPrefix(got, want+" ") {
			continue
		}
		if strings.Contains(got, "regular") {
			return fontFilePath(e.file)
		}
		if first == "" {
			first = fontFilePath(e.file)
		}
	}
	return first
}

func fontFilePath(f string) string {
	if filepath.IsAbs(f) {
		return f
	}
	dir := os.Getenv("WINDIR")
	if dir == "" {
		dir = `C:\Windows`
	}
	return filepath.Join(dir, "Fonts", f)
}
