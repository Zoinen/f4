//go:build !windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

var runFontconfigList = func(pattern string) ([]string, error) {
	output, err := exec.Command("fc-list", "--format=%{file}\\n", pattern).Output()
	if err != nil {
		return nil, err
	}
	return parseFontconfigPaths(string(output)), nil
}

func platformGuiFontFiles(language string) []string {
	if runtime.GOOS == "linux" {
		if paths, err := runFontconfigList(cjkFontconfigPattern(language)); err == nil && len(paths) > 0 {
			sortCJKFontPaths(paths, language)
			return paths
		}
	}

	paths := scanStandardFontDirectories(language)
	sortCJKFontPaths(paths, language)
	return paths
}

func scanStandardFontDirectories(language string) []string {
	dirs := []string{
		"/usr/share/fonts",
		"/usr/local/share/fonts",
		"/System/Library/Fonts",
		"/Library/Fonts",
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".fonts"), filepath.Join(home, ".local", "share", "fonts"), filepath.Join(home, "Library", "Fonts"))
	}

	var paths []string
	seen := make(map[string]struct{})
	for _, root := range dirs {
		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry == nil || entry.IsDir() || !isFontFile(path) {
				return nil
			}
			if _, ok := seen[path]; ok {
				return nil
			}
			seen[path] = struct{}{}
			paths = append(paths, path)
			return nil
		})
	}
	return paths
}
