//go:build windows

package vtui

import (
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

// fontRegistryKeys is where Windows records an installed font's file: the
// machine key for a system-wide install, the user key for one made without
// administrator rights. Reading them is how a font that does not sit in
// %SystemRoot%\Fonts -- a per-user install, or a font package delivered as an
// optional feature -- becomes visible to the fallback chain at all.
var fontRegistryKeys = []struct {
	root registry.Key
	path string
}{
	{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Fonts`},
	{registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Fonts`},
}

// registryFontFiles returns the font paths named by the registry. Nothing here
// is required to exist or to be readable: the caller stats every path before
// it keeps one, and a key this account cannot open simply contributes nothing.
func registryFontFiles() []string {
	var paths []string
	for _, key := range fontRegistryKeys {
		paths = append(paths, registryKeyFontFiles(key.root, key.path)...)
	}
	return paths
}

func registryKeyFontFiles(root registry.Key, path string) []string {
	key, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer func() { _ = key.Close() }()

	names, err := key.ReadValueNames(0)
	if err != nil {
		return nil
	}
	var paths []string
	for _, name := range names {
		value, _, err := key.GetStringValue(name)
		if err != nil || value == "" {
			continue
		}
		paths = append(paths, registryFontPaths(value)...)
	}
	return paths
}

// registryFontPaths expands one registry value into the paths it may mean: the
// data is either a bare file name, which means the font directories, or an
// absolute path.
func registryFontPaths(value string) []string {
	if filepath.IsAbs(value) {
		return []string{value}
	}
	dirs := windowsFontDirs()
	paths := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		paths = append(paths, filepath.Join(dir, value))
	}
	return paths
}
