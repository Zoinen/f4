package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/unxed/vtui"
)

//go:embed styles/*.ini
var builtInStyles embed.FS

type ColorStyle struct {
	Name     string
	ini      *IniFile
	custom   bool
	baseName string
}

const customColorStyleName = "Custom"

var getUserStylesDir = func() string {
	return filepath.Join(GetF4ConfigDir(), "styles")
}

func styleFromIni(fallbackName string, ini *IniFile) ColorStyle {
	name := strings.TrimSpace(ini.GetString("style", "Name", fallbackName))
	if name == "" {
		name = fallbackName
	}
	return ColorStyle{Name: name, ini: ini}
}

func customStyleFromIni(ini *IniFile) ColorStyle {
	baseName := strings.TrimSpace(ini.GetString("style", "Base", ""))
	if baseName == "" || strings.EqualFold(baseName, customColorStyleName) {
		baseName = strings.TrimSpace(AppConfig.ColorStyle)
	}
	if baseName == "" || strings.EqualFold(baseName, customColorStyleName) {
		baseName = "Modern"
	}
	return ColorStyle{
		Name:     customColorStyleName,
		ini:      ini,
		custom:   true,
		baseName: baseName,
	}
}

func loadStylesFromFS(source fs.FS, pattern string) []ColorStyle {
	paths, _ := fs.Glob(source, pattern)
	styles := make([]ColorStyle, 0, len(paths))
	for _, path := range paths {
		f, err := source.Open(path)
		if err != nil {
			continue
		}
		ini := ParseIni(f)
		f.Close()
		fallback := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		styles = append(styles, styleFromIni(fallback, ini))
	}
	return styles
}

func AvailableColorStyles() []ColorStyle {
	byName := make(map[string]ColorStyle)
	for _, style := range loadStylesFromFS(builtInStyles, "styles/*.ini") {
		byName[strings.ToLower(style.Name)] = style
	}

	userDir := getUserStylesDir()
	if entries, err := os.ReadDir(userDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".ini") {
				continue
			}
			ini := LoadIni(filepath.Join(userDir, entry.Name()))
			fallback := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			style := styleFromIni(fallback, ini)
			byName[strings.ToLower(style.Name)] = style
		}
	}

	// An exported farcolors.ini is a complete user scheme. Keep it in the
	// selector as Custom instead of applying it to every named style: a full
	// file would otherwise overwrite every colour as soon as a built-in style
	// is selected, making the selector appear broken. Partial files retain the
	// historical overlay behavior below in ApplyColorStyle.
	if path := userColorOverridesPath(); fileExists(path) {
		byName[strings.ToLower(customColorStyleName)] = customStyleFromIni(LoadIni(path))
	}

	styles := make([]ColorStyle, 0, len(byName))
	for _, style := range byName {
		styles = append(styles, style)
	}
	sort.Slice(styles, func(i, j int) bool {
		order := func(name string) int {
			switch strings.ToLower(name) {
			case "modern":
				return 0
			case "classic":
				return 1
			default:
				return 2
			}
		}
		io, jo := order(styles[i].Name), order(styles[j].Name)
		if io != jo {
			return io < jo
		}
		return strings.ToLower(styles[i].Name) < strings.ToLower(styles[j].Name)
	})
	return styles
}

func findColorStyle(styles []ColorStyle, name string) (ColorStyle, bool) {
	for _, style := range styles {
		if strings.EqualFold(style.Name, name) {
			return style, true
		}
	}
	return ColorStyle{}, false
}

func colorIniDefinesSlot(ini *IniFile, slot ColorSlot) bool {
	if ini == nil {
		return false
	}
	section, ok := ini.data["farcolors"]
	if !ok {
		return false
	}
	if _, ok := section[slot.Canonical]; ok {
		return true
	}
	for _, alias := range slot.Aliases {
		if _, ok := section[alias]; ok {
			return true
		}
	}
	return false
}

func isCompleteColorIni(ini *IniFile) bool {
	for _, slot := range ColorSlots {
		if !colorIniDefinesSlot(ini, slot) {
			return false
		}
	}
	return true
}

func isStandaloneCustomColorIni(ini *IniFile) bool {
	if ini != nil {
		if section, ok := ini.data["style"]; ok && strings.EqualFold(strings.TrimSpace(section["Name"]), customColorStyleName) {
			return true
		}
	}
	// Recognize files exported by older f4 versions, before the explicit
	// [style] marker was added.
	return isCompleteColorIni(ini)
}

// userColorOverridesPath points at the personal farcolors.ini. A partial file
// sits on top of whichever style is active; a complete exported file is also
// available as the standalone Custom style. It is a variable for the same
// reason getUserStylesDir is: tests need to point it somewhere harmless.
var userColorOverridesPath = func() string {
	return filepath.Join(GetF4ConfigDir(), "farcolors.ini")
}

// ApplyColorStyle rebuilds the palette from scratch: built-in defaults, then
// the named style, and finally any partial farcolors.ini overrides. A complete
// farcolors.ini is applied only when Custom is selected, so a saved scheme
// cannot mask every built-in style in the selector.
func ApplyColorStyle(name string) error {
	styles := AvailableColorStyles()
	style, ok := findColorStyle(styles, name)
	if !ok {
		return fmt.Errorf("color style %q not found", name)
	}

	vtui.SetDefaultPalette()
	SetDefaultF4Palette()
	themeStyle := style
	if style.custom {
		// Custom files normally contain every exported slot. When a user edits
		// or creates a partial file, use the recorded base theme (or the
		// currently configured theme) for newly-added slots.
		base, found := findColorStyle(styles, style.baseName)
		if !found || base.custom {
			base, found = findColorStyle(styles, "Modern")
		}
		if found {
			ApplyColorIni(base.ini)
			themeStyle = base
		}
		ApplyColorIni(style.ini)
	} else {
		ApplyColorIni(style.ini)
		if path := userColorOverridesPath(); fileExists(path) {
			userIni := LoadIni(path)
			if !isStandaloneCustomColorIni(userIni) {
				ApplyColorIni(userIni)
			}
		}
	}
	FinishColors()
	GlobalFileHighlighter.LoadThemeRules(themeStyle.ini)
	configureWorkspaceTabColors()
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
