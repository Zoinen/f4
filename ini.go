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
	section := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
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
	if sec, ok := ini.data[section]; ok {
		if val, ok := sec[key]; ok {
			return val
		}
	}
	return def
}
