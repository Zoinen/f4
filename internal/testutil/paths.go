package testutil

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// ModuleRootDir climbs from the package directory to the directory holding
// go.mod. Repo-wide inventory tests (hardcoded strings, command-palette
// surfaces) and fixtures living outside cmd/f4 (plugins/) must resolve paths
// against the module root, not this package's directory.
func ModuleRootDir(t *testing.T) string {
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

// SkipIfNoRelevantChanges calculates a SHA256 hash of all files matching the given globs.
// If the hash matches the one saved from the last successful run, the test is skipped.
// Cache is saved in the OS temporary directory to avoid polluting the repository.
func SkipIfNoRelevantChanges(t *testing.T, cacheName string, globPatterns ...string) {
	if os.Getenv("CI") != "" || os.Getenv("F4_FORCE_TESTS") != "" {
		return // Never skip in CI or when explicitly forced
	}

	var files []string
	for _, pattern := range globPatterns {
		matches, err := filepath.Glob(pattern)
		if err == nil {
			files = append(files, matches...)
		}
	}
	sort.Strings(files)

	h := sha256.New()
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil || info.IsDir() {
			continue
		}
		b, err := os.ReadFile(f)
		if err == nil {
			if _, err := h.Write([]byte(f)); err != nil {
				t.Fatal(err)
			}
			if _, err := h.Write(b); err != nil {
				t.Fatal(err)
			}
		}
	}

	pwd, _ := os.Getwd()
	pwdHashBytes := sha256.Sum256([]byte(pwd))
	pwdHash := hex.EncodeToString(pwdHashBytes[:])[:8]

	currentHash := hex.EncodeToString(h.Sum(nil))
	cacheFile := filepath.Join(os.TempDir(), "f4_test_cache_"+cacheName+"_"+pwdHash)

	oldHash, err := os.ReadFile(cacheFile)
	if err == nil && string(oldHash) == currentHash {
		t.Skip("Skipped: no changes in " + cacheName + " dependencies")
	}

	t.Cleanup(func() {
		if !t.Failed() {
			_ = os.WriteFile(cacheFile, []byte(currentHash), 0600) // The change-detection cache is best effort.
		}
	})
}
