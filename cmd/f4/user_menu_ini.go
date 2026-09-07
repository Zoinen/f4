package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// mainMenuRoot is the section prefix far2l uses for the persistent main menu.
// Keep it bit-for-bit identical so a user_menu.ini authored in far2l can be
// dropped into ~/.config/f4/settings/main_menu.ini and vice versa.
const mainMenuRoot = "UserMenu/MainMenu"

// LoadMainMenu reads a far2l-compatible main menu INI from path. A missing
// file is not an error: an empty slice is returned. Sections outside the
// UserMenu/MainMenu subtree are ignored, so the loader is safe to point at
// a mixed-content INI.
func LoadMainMenu(path string) ([]UserMenuItem, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []UserMenuItem{}, nil
		}
		return nil, err
	}
	defer f.Close()

	sections := map[string]map[string]string{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	var cur map[string]string
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r\n")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			name := trimmed[1 : len(trimmed)-1]
			if name == mainMenuRoot || strings.HasPrefix(name, mainMenuRoot+"/") {
				if _, ok := sections[name]; !ok {
					sections[name] = map[string]string{}
				}
				cur = sections[name]
			} else {
				cur = nil
			}
			continue
		}
		if cur == nil {
			continue
		}
		// Preserve the value verbatim. Keys are tidied; values can carry
		// trailing spaces that matter for shell commands, but far2l itself
		// trims them on read, so we do the same to round-trip cleanly.
		if eq := strings.IndexByte(line, '='); eq != -1 {
			key := strings.TrimSpace(line[:eq])
			val := strings.TrimSpace(line[eq+1:])
			if key != "" {
				cur[key] = val
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return buildTree(sections, mainMenuRoot), nil
}

// SaveMainMenu writes items to path in far2l-compatible flat INI form.
// Parent directories are created as needed.
func SaveMainMenu(path string, items []UserMenuItem) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}

	var buf strings.Builder
	first := true
	writeTree(&buf, items, mainMenuRoot, &first)

	return writeFileAtomically(path, []byte(buf.String()), 0o600)
}

func buildTree(sections map[string]map[string]string, prefix string) []UserMenuItem {
	var items []UserMenuItem
	for i := 0; ; i++ {
		key := fmt.Sprintf("%s/Item%d", prefix, i)
		sec, ok := sections[key]
		if !ok {
			break
		}
		item := UserMenuItem{
			HotKey: sec["HotKey"],
			Label:  sec["Label"],
		}
		if isSubmenuFlag(sec["Submenu"]) {
			children := buildTree(sections, key)
			if children == nil {
				children = []UserMenuItem{}
			}
			item.Submenu = children
		} else {
			var cmds []string
			for j := 0; ; j++ {
				cmd, ok := sec[fmt.Sprintf("Command%d", j)]
				if !ok {
					break
				}
				cmds = append(cmds, cmd)
			}
			item.Commands = cmds
		}
		items = append(items, item)
	}
	return items
}

func writeTree(buf *strings.Builder, items []UserMenuItem, prefix string, first *bool) {
	for i := range items {
		it := &items[i]
		section := fmt.Sprintf("%s/Item%d", prefix, i)
		if !*first {
			buf.WriteByte('\n')
		}
		*first = false
		buf.WriteByte('[')
		buf.WriteString(section)
		buf.WriteString("]\n")

		// Match far2l's apparent on-disk key ordering: alphabetical, which
		// also keeps writes deterministic regardless of map iteration.
		if !it.IsSubmenu() {
			for j, cmd := range it.Commands {
				buf.WriteString("Command")
				buf.WriteString(strconv.Itoa(j))
				buf.WriteByte('=')
				buf.WriteString(cmd)
				buf.WriteByte('\n')
			}
		}
		buf.WriteString("HotKey=")
		buf.WriteString(it.HotKey)
		buf.WriteByte('\n')
		buf.WriteString("Label=")
		buf.WriteString(it.Label)
		buf.WriteByte('\n')
		if it.IsSubmenu() {
			buf.WriteString("Submenu=1\n")
			writeTree(buf, it.Submenu, section, first)
		} else {
			buf.WriteString("Submenu=0\n")
		}
	}
}

func isSubmenuFlag(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return false
	}
	return n != 0
}
