package main

import (
	"bufio"
	"io"
	"os"
	"strings"
)

// IniFile represents a simple parsed INI configuration.
type IniFile struct {
	data map[string]map[string]string
}

// LoadIni reads an INI file into memory. Returns an empty struct if file is missing.
func LoadIni(filename string) *IniFile {
	f, err := os.Open(filename)
	if err != nil {
		return newIniFile()
	}
	defer f.Close()
	return ParseIni(f)
}

func newIniFile() *IniFile {
	return &IniFile{data: make(map[string]map[string]string)}
}

// ParseIni reads INI data from an arbitrary source.
func ParseIni(r io.Reader) *IniFile {
	ini := newIniFile()

	scanner := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024) // 10MB maximum line length
	section := ""
	isFirst := true
	for scanner.Scan() {
		line := scanner.Text()
		if isFirst {
			line = strings.TrimPrefix(line, "\xef\xbb\xbf")
			isFirst = false
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = line[1 : len(line)-1]
			if ini.data[section] == nil {
				ini.data[section] = make(map[string]string)
			}
		} else if idx := strings.Index(line, "="); idx != -1 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			if section != "" {
				ini.data[section][key] = val
			}
		}
	}
	return ini
}

// Merge overlays settings from another IniFile. Values in 'other' overwrite existing ones.
func (ini *IniFile) Merge(other *IniFile) {
	if other == nil {
		return
	}
	for section, keys := range other.data {
		if _, ok := ini.data[section]; !ok {
			ini.data[section] = make(map[string]string)
		}
		for key, val := range keys {
			ini.data[section][key] = val
		}
	}
}

// GetString safely retrieves a value or returns the default.
func (ini *IniFile) GetString(section, key, def string) string {
	// First check environment variables for overrides (e.g. F4_PANEL_SHOW_HIDDEN_FILES)
	envUpper := "F4_" + strings.ToUpper(section) + "_" + camelToSnake(key)
	if val := os.Getenv(envUpper); val != "" {
		return val
	}
	envLower := strings.ToLower(envUpper)
	if val := os.Getenv(envLower); val != "" {
		return val
	}

	if sec, ok := ini.data[section]; ok {
		if val, ok := sec[key]; ok {
			return val
		}
	}
	return def
}

func camelToSnake(s string) string {
	var res []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			prev := res[len(res)-1]
			if prev != '_' && !(prev >= 'A' && prev <= 'Z') {
				res = append(res, '_')
			}
		}
		res = append(res, r)
	}
	return strings.ToUpper(string(res))
}
