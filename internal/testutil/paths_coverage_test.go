package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestModuleRootDirFindsGoModule(t *testing.T) {
	root := ModuleRootDir(t)
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("ModuleRootDir() = %q without go.mod: %v", root, err)
	}
}

func TestSkipIfNoRelevantChangesHonoursCIBypasses(t *testing.T) {
	for _, variable := range []string{"CI", "F4_FORCE_TESTS"} {
		t.Run(variable, func(t *testing.T) {
			t.Setenv(variable, "1")
			SkipIfNoRelevantChanges(t, "paths-coverage-bypass", "[invalid")
		})
	}
}

func TestSkipIfNoRelevantChangesSkipsUnchangedFiles(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("F4_FORCE_TESTS", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "dependency.txt")
	if err := os.WriteFile(path, []byte("stable"), 0o600); err != nil {
		t.Fatal(err)
	}
	cacheName := "paths-coverage-" + filepath.Base(dir)
	pattern := filepath.Join(dir, "*.txt")

	t.Run("populate", func(t *testing.T) {
		SkipIfNoRelevantChanges(t, cacheName, pattern)
	})
	t.Run("skip", func(t *testing.T) {
		SkipIfNoRelevantChanges(t, cacheName, pattern)
		t.Fatal("unchanged dependency set was not skipped")
	})
}
