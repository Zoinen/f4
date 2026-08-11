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
	Name string
	ini  *IniFile
}

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

// userColorOverridesPath points at the personal farcolors.ini that sits on top
// of whichever style is active. It is a variable for the same reason
// getUserStylesDir is: tests need to point it somewhere harmless.
var userColorOverridesPath = func() string {
	return filepath.Join(GetF4ConfigDir(), "farcolors.ini")
}

// ApplyColorStyle rebuilds the palette from scratch: built-in defaults, then
// the named style, then the user's own farcolors.ini. Every caller goes
// through here, so switching styles at runtime lands on exactly the palette a
// restart would produce — previously the overrides were applied only during
// startup and silently disappeared until the next launch.
func ApplyColorStyle(name string) error {
	for _, style := range AvailableColorStyles() {
		if strings.EqualFold(style.Name, name) {
			vtui.SetDefaultPalette()
			SetDefaultF4Palette()
			ApplyColorIni(style.ini)
			if path := userColorOverridesPath(); fileExists(path) {
				ApplyColorIni(LoadIni(path))
			}
			FinishColors()
			GlobalFileHighlighter.LoadThemeRules(style.ini)
			configureWorkspaceTabColors()
			return nil
		}
	}
	return fmt.Errorf("color style %q not found", name)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
