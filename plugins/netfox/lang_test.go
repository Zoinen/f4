package netfox

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/unxed/vtui"
)

// hostStringsPath points at the caption table this package's dialogs really
// use. netfox is compiled into f4, so at runtime the host has pushed
// lang/en.lng into vtui long before a dialog opens. A `go test
// ./plugins/netfox` binary has no host: without this loader every vtui.Msg
// call returns "{Key}", which is wider than the caption it stands for, and
// the layout validator then fails on text no user will ever see. That is
// exactly how TestConnectionDialogLayout broke when the connection dialog
// stopped carrying its captions as literals.
const hostStringsPath = "../../lang/en.lng"

// loadHostStrings does for the [Strings] section what f4's InitLang does:
// trim, unescape \n, push into vtui. It is a plain parser rather than a call
// into the host because package main cannot be imported from here.
func loadHostStrings() error {
	data, err := os.ReadFile(filepath.FromSlash(hostStringsPath))
	if err != nil {
		return err
	}

	table := make(map[string]string)
	inStrings := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[Strings]") {
			inStrings = true
			continue
		}
		if strings.HasPrefix(line, "[") {
			inStrings = false
			continue
		}
		if !inStrings || line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		table[key] = strings.ReplaceAll(value, `\n`, "\n")
	}

	vtui.AddStrings(table)
	return nil
}

func init() {
	// Best effort on purpose: a missing or moved file is reported once by
	// TestPluginCaptionsResolve with a readable message, instead of as a
	// wall of layout errors in every dialog test.
	_ = loadHostStrings()
}

var msgKeyRe = regexp.MustCompile(`vtui\.Msg\("([^"]+)"\)`)

// msgKeysInPackage collects every caption key the shipped sources ask for.
func msgKeysInPackage(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		for _, m := range msgKeyRe.FindAllStringSubmatch(string(data), -1) {
			seen[m[1]] = true
		}
	}

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

// TestPluginCaptionsResolve fails when a key used by this package has no
// entry in the host table and no vtui built-in, because such a key reaches
// the user as a raw "{Key}" at runtime, not just in tests.
func TestPluginCaptionsResolve(t *testing.T) {
	if err := loadHostStrings(); err != nil {
		t.Fatalf("cannot read %s: %v", hostStringsPath, err)
	}

	keys, err := msgKeysInPackage(".")
	if err != nil {
		t.Fatalf("cannot scan package sources: %v", err)
	}
	if len(keys) == 0 {
		t.Fatal("no vtui.Msg keys found, is the test running from the package directory?")
	}

	for _, key := range keys {
		if got := vtui.Msg(key); got == "{"+key+"}" {
			t.Errorf("caption key %q is missing from %s and would reach the user as %q",
				key, hostStringsPath, got)
		}
	}
}
