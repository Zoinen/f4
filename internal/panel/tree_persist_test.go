package panel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTreeExpandPersistenceRoundTrip(t *testing.T) {
	root := t.TempDir()
	kept := filepath.Join(root, "kept")
	if err := os.Mkdir(kept, 0o750); err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(root, "gone") // never created: must not be written back
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	treeExpandCacheMu.Lock()
	saved := treeExpandCache
	treeExpandCache = map[string]struct{}{kept: {}, gone: {}, file: {}}
	treeExpandCacheMu.Unlock()
	state := filepath.Join(t.TempDir(), "cfg", "tree_expanded.txt")
	EnableTreeExpandPersistence(state)
	t.Cleanup(func() {
		EnableTreeExpandPersistence("")
		treeExpandCacheMu.Lock()
		treeExpandCache = saved
		treeExpandCacheMu.Unlock()
	})

	saveTreeExpanded()
	data, err := os.ReadFile(state)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(data)); got != kept {
		t.Fatalf("saved %q, want only the existing directory %q", got, kept)
	}

	// A fresh process: empty cache, the file is merged in once.
	treeExpandCacheMu.Lock()
	treeExpandCache = map[string]struct{}{}
	treeExpandCacheMu.Unlock()
	EnableTreeExpandPersistence(state)
	paths := treeExpandedSnapshot()
	if len(paths) != 1 || paths[0] != kept {
		t.Fatalf("loaded %v, want [%s]", paths, kept)
	}

	// An expand schedules a write; collapsing everything writes an empty file.
	forgetTreeExpanded(kept)
	saveTreeExpanded()
	if data, _ := os.ReadFile(state); len(strings.TrimSpace(string(data))) != 0 {
		t.Fatalf("collapsed tree still saved: %q", data)
	}
}
