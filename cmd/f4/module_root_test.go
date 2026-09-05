package main

import (
	"os"
	"path/filepath"
	"testing"
)

// moduleRootDir climbs from the package directory to the directory holding
// go.mod. Repo-wide inventory tests (hardcoded strings, command-palette
// surfaces) and fixtures living outside cmd/f4 (plugins/) must resolve paths
// against the module root, not this package's directory.
func moduleRootDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("module root with go.mod not found above the test directory")
		}
		dir = parent
	}
}
