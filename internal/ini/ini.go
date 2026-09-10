package ini

import (
	"bufio"
	"io"
	"os"
	"strings"
)

// File represents a simple parsed INI configuration.
type File struct {
	Data map[string]map[string]string
}

// Load reads an INI file into memory. Returns an empty struct if file is missing.
func Load(filename string) *File {
	f, err := os.Open(filename)
	if err != nil {
		return New()
	}
	defer f.Close()
	return Parse(f)
}

func New() *File {
	return &File{Data: make(map[string]map[string]string)}
}

// Sections returns every section by name, keyed by section then key. The maps
// are the file's own, so a caller that writes to them writes to the file.
func (f *File) Sections() map[string]map[string]string {
	return f.Data
}

// Parse reads INI data from an arbitrary source.
func Parse(r io.Reader) *File {
	ini := New()

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
			if ini.Data[section] == nil {
				ini.Data[section] = make(map[string]string)
			}
		} else if idx := strings.Index(line, "="); idx != -1 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			if section != "" {
				ini.Data[section][key] = val
			}
		}
	}
	return ini
}

// Merge overlays settings from another File. Values in 'other' overwrite existing ones.
func (ini *File) Merge(other *File) {
	if other == nil {
		return
	}
	for section, keys := range other.Data {
		if _, ok := ini.Data[section]; !ok {
			ini.Data[section] = make(map[string]string)
		}
		for key, val := range keys {
			ini.Data[section][key] = val
		}
	}
}

// GetString safely retrieves a value or returns the default.
func (ini *File) GetString(section, key, def string) string {
	// First check environment variables for overrides (e.g. F4_PANEL_SHOW_HIDDEN_FILES)
	envUpper := "F4_" + strings.ToUpper(section) + "_" + camelToSnake(key)
	if val := os.Getenv(envUpper); val != "" {
		return val
	}
	envLower := strings.ToLower(envUpper)
	if val := os.Getenv(envLower); val != "" {
		return val
	}

	if sec, ok := ini.Data[section]; ok {
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
			if prev != '_' && (prev < 'A' || prev > 'Z') {
				res = append(res, '_')
			}
		}
		res = append(res, r)
	}
	return strings.ToUpper(string(res))
}
