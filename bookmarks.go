package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mattn/go-runewidth"
)

// Folder bookmarks: ten numbered slots, each remembering one directory,
// addressable by the digit hotkeys far2l uses. The on-disk layout is
// far2l's own, so ~/.config/far2l/settings/bookmarks.ini can be copied to
// ~/.config/f4/settings/bookmarks.ini (or the other way round) unchanged.

// Bookmark is a single slot of the table.
type Bookmark struct {
	Path       string // absolute filesystem path; empty means slot is unset
	Plugin     string // preserved from far2l; f4 does not act on it yet
	PluginData string
	PluginFile string
}

// IsEmpty is true when nothing meaningful is stored in this slot.
// A slot counts as non-empty if Path or Plugin is set.
func (b Bookmark) IsEmpty() bool { return b.Path == "" && b.Plugin == "" }

// BookmarkSet is the full 10-slot table. Indices map directly to the digit
// hotkey: BookmarkSet[3] is what RightCtrl+3 jumps to.
type BookmarkSet [10]Bookmark

// BookmarksFilePath returns the user-config location of the bookmark table.
// The filename matches far2l so the same file can be shared between
// ~/.config/far2l/settings/bookmarks.ini and the f4 directory without
// renaming.
func BookmarksFilePath() string {
	configDir, _ := os.UserConfigDir()
	return filepath.Join(configDir, "f4", "settings", "bookmarks.ini")
}

// LoadBookmarks reads a far2l-compatible bookmarks.ini from path. A missing
// file is not an error: an empty set is returned. Sections outside [0]..[9]
// are ignored, so the loader is safe to point at a mixed-content INI, and
// malformed lines are skipped rather than failing the whole file.
func LoadBookmarks(path string) (BookmarkSet, error) {
	var set BookmarkSet

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return set, nil
		}
		return set, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	slot := -1
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r\n")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			slot = bookmarkSlot(trimmed[1 : len(trimmed)-1])
			continue
		}
		if slot < 0 {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq == -1 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		switch key {
		case "Path":
			set[slot].Path = val
		case "Plugin":
			set[slot].Plugin = val
		case "PluginData":
			set[slot].PluginData = val
		case "PluginFile":
			set[slot].PluginFile = val
		}
	}
	if err := scanner.Err(); err != nil {
		return BookmarkSet{}, err
	}

	return set, nil
}

// SaveBookmarks writes the set to path in far2l-compatible INI form,
// atomically (temp file + rename). Parent directories are created as
// needed. Empty slots are left out entirely, the way far2l does it: a
// missing section is what marks a slot as free.
func SaveBookmarks(path string, s BookmarkSet) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	var buf strings.Builder
	first := true
	// Sections may sit in any order on disk; write them ascending so saves
	// are deterministic.
	for i := range s {
		if s[i].IsEmpty() {
			continue
		}
		if !first {
			buf.WriteByte('\n')
		}
		first = false
		buf.WriteByte('[')
		buf.WriteString(strconv.Itoa(i))
		buf.WriteString("]\n")

		// far2l emits all four keys alphabetically, even when the plugin
		// fields are empty strings.
		buf.WriteString("Path=")
		buf.WriteString(s[i].Path)
		buf.WriteByte('\n')
		buf.WriteString("Plugin=")
		buf.WriteString(s[i].Plugin)
		buf.WriteByte('\n')
		buf.WriteString("PluginData=")
		buf.WriteString(s[i].PluginData)
		buf.WriteByte('\n')
		buf.WriteString("PluginFile=")
		buf.WriteString(s[i].PluginFile)
		buf.WriteByte('\n')
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(buf.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// truncPathLeft shortens path to at most width display cells by dropping
// characters from the front and marking the cut with an ellipsis. far2l
// truncates the paths it lists in menus the same way round
// (mix/StrCells.cpp, StrCellsTruncateLeft): the tail of a path is the
// part that identifies it.
func truncPathLeft(path string, width int) string {
	const ellipsis = "…"
	if width <= 0 {
		return ""
	}
	if runewidth.StringWidth(path) <= width {
		return path
	}
	runes := []rune(path)
	for i := 1; i < len(runes); i++ {
		if runewidth.StringWidth(string(runes[i:]))+1 <= width {
			return ellipsis + string(runes[i:])
		}
	}
	return ellipsis
}

// bookmarkSlot maps a section name to its slot index, or -1 when the
// section is not one of far2l's [0]..[9] bookmark sections.
func bookmarkSlot(name string) int {
	if len(name) != 1 || name[0] < '0' || name[0] > '9' {
		return -1
	}
	return int(name[0] - '0')
}
