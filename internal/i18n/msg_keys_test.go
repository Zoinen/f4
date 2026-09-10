package i18n

import (
	"fmt"
	"github.com/unxed/f4/internal/ini"
	"github.com/unxed/f4/internal/testutil"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// msgKeyCall finds the calls whose key is a literal. The receiver is left out
// on purpose: i18n.Msg, vtui.Msg and the bare Msg inside this package all read
// the same table, so all three belong in the sweep. A key built by
// concatenation — Msg("Menu." + name) — is invisible to this pattern and is
// meant to be: only the literal half would be checkable, and half a key
// resolves against nothing.
var msgKeyCall = regexp.MustCompile(`(?:Msg|HelpMsg)\("([A-Za-z0-9_.]+)"\)`)

// msgKeyShape matches a literal shaped like a key in en.lng: capitalised, and
// dotted at least once. Most keys never reach Msg directly — they are handed
// to a helper that looks them up — so this is what finds them.
var msgKeyShape = regexp.MustCompile(`"([A-Z][A-Za-z0-9_]*(?:\.[A-Za-z0-9_]+)+)"`)

// Files whose keys are fixtures rather than captions: they name keys that are
// deliberately absent, to exercise what a lookup does when it misses.
var msgKeyFixtureFiles = map[string]bool{
	"tools/hardcode/hardcode_test.go": true,
	"internal/i18n/lang_test.go":      true,
}

// Keys the code calls that no language file defines. A miss is not an error at
// runtime — vtui.Msg renders it as "{key}" — which is exactly why it needs a
// test: the caption reaches the user in braces and nothing else complains.
var msgKeysWithoutAString = map[string]string{
	// internal/editor/view.go:3919. Absent from upstream's en.lng too, so the
	// fix is a string in their table, not a change here.
	"KeyBar.EditorAltF8": "upstream's gap, not ours",
}

// A key that reaches the user has to exist in en.lng. Nothing else checks this
// direction: lang_consistency_test.go compares the other .lng files against
// en.lng, and a key that only the code names is in neither set.
//
// What it does not cover: a key deleted from en.lng that the code only ever
// names indirectly. 775 of the 1804 keys are never written inside a Msg call,
// so neither sweep would see them go. That direction is left open on purpose —
// en.lng is edited by hand and a line is not removed from it by accident.
func TestEveryLiteralMsgKeyExists(t *testing.T) {
	known := languageKeys(t)
	seen := make(map[string]bool)
	var misses []string

	walkGoFiles(t, func(relative string, source []byte) {
		if msgKeyFixtureFiles[relative] {
			return
		}
		for _, match := range msgKeyCall.FindAllSubmatchIndex(source, -1) {
			key := string(source[match[2]:match[3]])
			seen[key] = true
			if _, ok := known[key]; ok {
				continue
			}
			if _, ok := msgKeysWithoutAString[key]; ok {
				continue
			}
			misses = append(misses, location(relative, source, match[0])+key)
		}
	})

	// A sweep that finds nothing passes, and would keep passing after the
	// pattern or the walk broke. The floor is well below the current count and
	// only catches that.
	if len(seen) < 900 {
		t.Fatalf("swept %d distinct keys, want at least 900: the pattern or the walk is broken, not the tree", len(seen))
	}
	if len(misses) > 0 {
		t.Errorf("%d key(s) reach the user as \"{key}\" — no string in lang/en.lng:\n%s",
			len(misses), strings.Join(misses, "\n"))
	}
}

// The check above sees a key only where it is written inside the call. Most
// are not: they are handed to a helper that does the lookup, and the one
// confirmed case of this damage was exactly that shape —
// Msg(key) two frames away from the literal.
//
// So this looks for the damage instead of the call. A rename that rewrites a
// string literal as though it were an identifier — IniFile becoming ini.File —
// leaves a key that differs from a real one only in dots and case. A literal
// that collides with a real key once dots, underscores and case are removed,
// and is not that key, is that rewrite. Nothing else produces one, which is why
// this can be a whole-tree sweep with no exemptions.
func TestNoLiteralIsANearMissOfAKey(t *testing.T) {
	known := languageKeys(t)
	normalized := make(map[string]string, len(known))
	for key := range known {
		normalized[normalizeKey(key)] = key
	}

	var misses []string
	walkGoFiles(t, func(relative string, source []byte) {
		for _, match := range msgKeyShape.FindAllSubmatchIndex(source, -1) {
			literal := string(source[match[2]:match[3]])
			if _, ok := known[literal]; ok {
				continue
			}
			real, ok := normalized[normalizeKey(literal)]
			if !ok {
				continue
			}
			misses = append(misses, fmt.Sprintf("%s%q, and lang/en.lng has %q", location(relative, source, match[0]), literal, real))
		}
	})

	if len(misses) > 0 {
		t.Errorf("%d literal(s) differ from a real key only in dots and case, which is what a rename rewriting a string produces:\n%s",
			len(misses), strings.Join(misses, "\n"))
	}
}

func normalizeKey(key string) string {
	return strings.ToLower(strings.NewReplacer(".", "", "_", "").Replace(key))
}

func languageKeys(t *testing.T) map[string]string {
	t.Helper()
	known := LoadLangMapFromINI(ini.Parse(strings.NewReader(defaultLangData)))
	if len(known) == 0 {
		t.Fatal("the embedded en.lng parsed to no strings at all")
	}
	return known
}

// location renders "path:line: " for the byte offset of a match.
func location(relative string, source []byte, offset int) string {
	return fmt.Sprintf("%s:%d: ", relative, 1+strings.Count(string(source[:offset]), "\n"))
}

func walkGoFiles(t *testing.T, visit func(relative string, source []byte)) {
	t.Helper()
	// The sweep spans the whole module — plugins and tools included, since a
	// caption is a caption wherever it is written.
	root := testutil.ModuleRootDir(t)
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path == root {
				return nil
			}
			// Match Go module discovery and exclude tooling snapshots and dependencies.
			if strings.HasPrefix(entry.Name(), ".") || strings.HasPrefix(entry.Name(), "_") {
				return fs.SkipDir
			}
			if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
				return fs.SkipDir
			}
			// A nested repository or worktree carries its own copy of the tree,
			// commonly under _work/. Reading it would sweep a second, older
			// version of every file here.
			if _, markerErr := os.Lstat(filepath.Join(path, ".git")); markerErr == nil {
				return fs.SkipDir
			} else if !os.IsNotExist(markerErr) {
				return markerErr
			}
			switch entry.Name() {
			case "vendor", "testdata":
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".go") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)

	// Reading happens after the walk, not inside it: a filesystem call in the
	// callback races the walk's own traversal.
	for _, path := range paths {
		source, err := os.ReadFile(path) // #nosec G304 -- path comes from walking the module root.
		if err != nil {
			t.Fatal(err)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}
		visit(filepath.ToSlash(relative), source)
	}
}
