// Package tarindexcache is where f4 keeps the file indexes of tar archives it
// has opened, and the few things the rest of f4 needs to know about that place:
// which files belong to an archive, so that deleting or moving the archive can
// take them along instead of leaving them behind (#1187).
//
// A file of the cache is named "<archive name>-<hash of its path>-<fingerprint
// of its content>.index.sqlite", plus SQLite's side files. Everything up to the
// fingerprint is the prefix this package computes; the fingerprint is the
// archive plugin's business.
package tarindexcache

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

const pathMarkerSuffix = ".f4-path"

// userCacheDir is os.UserCacheDir, replaceable by the tests.
var userCacheDir = os.UserCacheDir

// Dir is the folder the indexes live in.
func Dir() string {
	base, err := userCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "f4", "tar-indexes")
}

// Prefix starts the names of every cache file of the archive at archivePath. The
// hash is of the absolute path, so two archives of one name in different
// folders do not share an index.
func Prefix(archivePath string) string {
	abs, err := filepath.Abs(archivePath)
	if err != nil {
		abs = archivePath
	}
	sum := sha256.Sum256([]byte(abs))
	return filepath.Base(archivePath) + "-" + hex.EncodeToString(sum[:16])
}

// PathFile is the small registry entry that records which archive owns a
// prefix. The index filename hashes the path, so without this entry a cache
// file left by an archive deleted outside f4 cannot be identified later.
func PathFile(archivePath string) string {
	return filepath.Join(Dir(), Prefix(archivePath)+pathMarkerSuffix)
}

// Track records the path that owns the indexes with its prefix.
func Track(archivePath string) {
	abs, err := filepath.Abs(archivePath)
	if err != nil {
		return
	}
	_ = os.WriteFile(PathFile(archivePath), []byte(abs+"\n"), 0o600)
}

// Cleanup removes indexes whose tracked archive was deleted outside f4.
// Failures are ignored: a concurrently opened or removed file is only clutter.
func Cleanup() {
	dir := Dir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, pathMarkerSuffix) {
			continue
		}
		marker := filepath.Join(dir, name)
		data, err := os.ReadFile(marker)
		if err != nil || strings.TrimSpace(string(data)) == "" {
			continue
		}
		archivePath := strings.TrimSpace(string(data))
		if _, err := os.Stat(archivePath); err == nil { // #nosec G703 -- this path is the archive path written by f4's own marker.
			continue
		}
		prefix := strings.TrimSuffix(name, pathMarkerSuffix)
		for _, cacheEntry := range entries {
			cacheName := cacheEntry.Name()
			if cacheEntry.IsDir() || !belongs(cacheName, prefix) {
				continue
			}
			_ = os.Remove(filepath.Join(dir, cacheName))
		}
		_ = os.Remove(marker)
	}
}

// belongs reports whether a cache file name is one of prefix's: the prefix must
// end where the name goes on with the fingerprint or with the library's older
// ".index.sqlite", or a longer archive name that merely starts the same would
// be taken for it.
func belongs(name, prefix string) bool {
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	rest := name[len(prefix):]
	return strings.HasPrefix(rest, "-") || strings.HasPrefix(rest, ".index.sqlite")
}

// Files lists the cache files of the archive at archivePath.
func Files(archivePath string) []string {
	dir := Dir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	prefix := Prefix(archivePath)
	var files []string
	for _, e := range entries {
		if !e.IsDir() && belongs(e.Name(), prefix) {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	return files
}

// Move gives the cache files of the archive at oldPath to newPath. Nothing in
// them refers to where the archive is, and the fingerprint is of its content, so
// a renamed or moved archive keeps its index. Failures are ignored: an index
// that stays behind is only clutter, and the next open builds a new one.
func Move(oldPath, newPath string) {
	oldPrefix, newPrefix := Prefix(oldPath), Prefix(newPath)
	if oldPrefix == newPrefix {
		return
	}
	dir := Dir()
	for _, file := range Files(oldPath) {
		name := filepath.Base(file)
		_ = os.Rename(file, filepath.Join(dir, newPrefix+name[len(oldPrefix):]))
	}
	_ = os.Rename(PathFile(oldPath), PathFile(newPath))
}

// Clear removes every cached index and reports how many files went. It is the
// "rebuild all indexes" button: what is missing is built again the next time an
// archive is opened. A file another process holds open cannot be removed on
// Windows; it is left, and does not count.
func Clear() int {
	dir := Dir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	removed := 0
	for _, e := range entries {
		if e.IsDir() || !strings.Contains(e.Name(), ".index.sqlite") {
			if strings.HasSuffix(e.Name(), pathMarkerSuffix) {
				_ = os.Remove(filepath.Join(dir, e.Name()))
			}
			continue
		}
		if os.Remove(filepath.Join(dir, e.Name())) == nil {
			removed++
		}
	}
	return removed
}
