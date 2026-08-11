package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// skipIfNoRelevantChanges calculates a SHA256 hash of all files matching the given globs.
// If the hash matches the one saved from the last successful run, the test is skipped.
// Cache is saved in the OS temporary directory to avoid polluting the repository.
func skipIfNoRelevantChanges(t *testing.T, cacheName string, globPatterns ...string) {
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
			h.Write([]byte(f))
			h.Write(b)
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
			_ = os.WriteFile(cacheFile, []byte(currentHash), 0644)
		}
	})
}
