// Command langfmt keeps f4 language files grouped by their key namespace.
//
// The English language file is the source of truth for the namespace and key
// order. A translation may omit keys, but it must keep the keys it has in the
// same deterministic order as English. The formatter only moves complete
// key/value records; values are never parsed or rewritten.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type entry struct {
	key      string
	line     string
	comments []string
}

type languageFile struct {
	prefix        []string
	entries       []entry
	trailingLines []string
	hasFinalNL    bool
	eol           string
}

type formatter struct {
	keys       []string
	keyIndex   map[string]int
	groupByKey map[string]string
}

func main() {
	var (
		write  bool
		check  bool
		source string
	)
	flag.BoolVar(&write, "w", false, "rewrite files in place")
	flag.BoolVar(&check, "check", false, "report files that are not formatted")
	flag.StringVar(&source, "source", "cmd/f4/lang/en.lng", "English source-of-truth language file")
	flag.Parse()

	if write && check {
		fatal("-w and -check cannot be used together")
	}
	if !write && !check {
		check = true
	}

	paths := flag.Args()
	var err error
	if len(paths) > 0 {
		paths, err = expandPaths(paths)
		if err != nil {
			fatal("expand language file paths: %v", err)
		}
	}
	if len(paths) == 0 {
		paths, err = filepath.Glob(filepath.Join(filepath.Dir(source), "*.lng"))
		if err != nil {
			fatal("find language files: %v", err)
		}
	}
	if len(paths) == 0 {
		fatal("no language files found")
	}

	sourceData, err := os.ReadFile(source)
	if err != nil {
		fatal("read source %s: %v", source, err)
	}
	sourceFile, err := parse(sourceData)
	if err != nil {
		fatal("parse source %s: %v", source, err)
	}
	f, err := newFormatter(sourceFile.entries)
	if err != nil {
		fatal("build format order from %s: %v", source, err)
	}

	failed := false
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			fatal("read %s: %v", path, err)
		}
		file, err := parse(data)
		if err != nil {
			fatal("parse %s: %v", path, err)
		}
		formatted, err := f.format(file)
		if err != nil {
			fatal("format %s: %v", path, err)
		}
		if bytes.Equal(data, formatted) {
			continue
		}

		if write {
			info, err := os.Stat(path)
			if err != nil {
				fatal("stat %s: %v", path, err)
			}
			// #nosec G703 -- path is the exact user-supplied language file (or a match from the user's glob) being formatted in place.
			if err := os.WriteFile(path, formatted, info.Mode().Perm()); err != nil {
				fatal("write %s: %v", path, err)
			}
			fmt.Printf("formatted %s\n", path)
		} else {
			fmt.Printf("%s is not formatted\n", path)
			failed = true
		}
	}

	if failed {
		os.Exit(1)
	}
}

func expandPaths(paths []string) ([]string, error) {
	var expanded []string
	for _, path := range paths {
		if !strings.ContainsAny(path, "*?[") {
			expanded = append(expanded, path)
			continue
		}
		matches, err := filepath.Glob(path)
		if err != nil {
			return nil, err
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("pattern %q matched no files", path)
		}
		expanded = append(expanded, matches...)
	}
	return expanded, nil
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "langfmt: "+format+"\n", args...)
	os.Exit(2)
}

func parse(data []byte) (languageFile, error) {
	normalized := strings.ReplaceAll(string(data), "\r\n", "\n")
	file := languageFile{
		hasFinalNL: strings.HasSuffix(normalized, "\n"),
		eol:        "\n",
	}
	if bytes.Contains(data, []byte("\r\n")) {
		file.eol = "\r\n"
	}
	if file.hasFinalNL {
		normalized = strings.TrimSuffix(normalized, "\n")
	}
	lines := []string{}
	if normalized != "" {
		lines = strings.Split(normalized, "\n")
	}

	stringsIndex := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "[Strings]" {
			stringsIndex = i
			break
		}
	}
	if stringsIndex < 0 {
		return languageFile{}, errors.New("missing [Strings] section")
	}
	file.prefix = append([]string(nil), lines[:stringsIndex+1]...)

	seen := make(map[string]bool)
	var pendingComments []string
	for _, line := range lines[stringsIndex+1:] {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			continue
		case strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "#"):
			pendingComments = append(pendingComments, line)
			continue
		case strings.HasPrefix(trimmed, "["):
			return languageFile{}, fmt.Errorf("unexpected section %q after [Strings]", trimmed)
		}

		key, ok := parseKey(line)
		if !ok {
			return languageFile{}, fmt.Errorf("invalid line %q", line)
		}
		if seen[key] {
			return languageFile{}, fmt.Errorf("duplicate key %q", key)
		}
		seen[key] = true
		file.entries = append(file.entries, entry{
			key:      key,
			line:     line,
			comments: pendingComments,
		})
		pendingComments = nil
	}
	file.trailingLines = pendingComments
	return file, nil
}

func parseKey(line string) (string, bool) {
	equal := strings.IndexByte(line, '=')
	if equal <= 0 {
		return "", false
	}
	key := line[:equal]
	if key != strings.TrimSpace(key) || key == "" {
		return "", false
	}
	for _, r := range key {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			continue
		}
		return "", false
	}
	return key, true
}

func namespace(key string) string {
	if dot := strings.IndexByte(key, '.'); dot >= 0 {
		return key[:dot]
	}
	return key
}

func newFormatter(source []entry) (formatter, error) {
	f := formatter{
		keyIndex:   make(map[string]int, len(source)),
		groupByKey: make(map[string]string, len(source)),
	}

	groups := make(map[string][]entry)
	seen := make(map[string]bool, len(source))
	var groupOrder []string
	for _, item := range source {
		if seen[item.key] {
			return formatter{}, fmt.Errorf("duplicate source key %q", item.key)
		}
		seen[item.key] = true
		group := namespace(item.key)
		if _, exists := groups[group]; !exists {
			groupOrder = append(groupOrder, group)
		}
		groups[group] = append(groups[group], item)
		f.groupByKey[item.key] = group
	}

	for _, group := range groupOrder {
		for _, item := range groups[group] {
			f.keyIndex[item.key] = len(f.keys)
			f.keys = append(f.keys, item.key)
		}
	}
	return f, nil
}

func (f formatter) format(file languageFile) ([]byte, error) {
	entries := make(map[string]entry, len(file.entries))
	for _, item := range file.entries {
		if _, ok := f.keyIndex[item.key]; !ok {
			return nil, fmt.Errorf("key %q is missing from English source", item.key)
		}
		entries[item.key] = item
	}

	lines := append([]string(nil), file.prefix...)
	lastGroup := ""
	for _, key := range f.keys {
		item, ok := entries[key]
		if !ok {
			continue
		}
		group := f.groupByKey[key]
		if lastGroup != "" && group != lastGroup {
			lines = append(lines, "")
		}
		lines = append(lines, item.comments...)
		lines = append(lines, item.line)
		lastGroup = group
	}
	lines = append(lines, file.trailingLines...)

	result := strings.Join(lines, "\n")
	if file.hasFinalNL {
		result += "\n"
	}
	if file.eol != "\n" {
		result = strings.ReplaceAll(result, "\n", file.eol)
	}
	return []byte(result), nil
}
