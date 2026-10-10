//go:build !freebsd && !openbsd && !netbsd && !dragonfly && !solaris && !illumos

package archive

import (
	"archive/tar"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/tarindexcache"
)

// isolateUserCache points every platform's user cache directory at a fresh
// folder, so the tar index the test builds is the only one there.
func isolateUserCache(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("LocalAppData", dir)
	t.Setenv("HOME", dir)
}

func writeIssue1187Tar(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(f)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// listTarRoot opens the archive the way the panel does and returns the names
// in its top folder.
func listTarRoot(t *testing.T, path string) []string {
	t.Helper()
	fsys, err := openArchiveFileSystem(context.Background(), path, "", "")
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = fsys.Close() }()
	entries, err := fsys.ReadDir(".")
	if err != nil {
		t.Fatalf("list %s: %v", path, err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

// TestIssue1187ChangedTarIsNotListedFromItsOldIndex is the regression test for
// the "stale tar" report on issue #1187: the index of a tar is kept in the
// user's cache and was reused on the next open with no check that the archive
// is still the one it was built from, so a tar that had been replaced was
// listed with the contents it used to have.
func TestIssue1187ChangedTarIsNotListedFromItsOldIndex(t *testing.T) {
	isolateUserCache(t)
	path := filepath.Join(t.TempDir(), "changing.tar")

	writeIssue1187Tar(t, path, map[string]string{"old.txt": "old"})
	if got := listTarRoot(t, path); !reflect.DeepEqual(got, []string{"old.txt"}) {
		t.Fatalf("first listing = %v", got)
	}

	// A different archive under the same name, bigger than the first.
	writeIssue1187Tar(t, path, map[string]string{"new1.txt": "a longer body than before", "new2.txt": "another"})
	if got, want := listTarRoot(t, path), []string{"new1.txt", "new2.txt"}; !reflect.DeepEqual(got, want) {
		t.Errorf("listing after the archive changed = %v, want %v (the old index was reused)", got, want)
	}
}

// TestIssue1187SameSizeTarWithOtherContentIsNotListedFromItsOldIndex covers
// the case a size check alone would miss: the archive was rewritten to the same
// length (tar pads to whole blocks, so this is common).
func TestIssue1187SameSizeTarWithOtherContentIsNotListedFromItsOldIndex(t *testing.T) {
	isolateUserCache(t)
	path := filepath.Join(t.TempDir(), "same.tar")

	writeIssue1187Tar(t, path, map[string]string{"old.txt": "old"})
	before, _ := os.Stat(path)
	if got := listTarRoot(t, path); !reflect.DeepEqual(got, []string{"old.txt"}) {
		t.Fatalf("first listing = %v", got)
	}

	writeIssue1187Tar(t, path, map[string]string{"new.txt": "new"})
	after, _ := os.Stat(path)
	if before.Size() != after.Size() {
		t.Fatalf("test setup: the archives differ in size (%d and %d)", before.Size(), after.Size())
	}
	if got := listTarRoot(t, path); !reflect.DeepEqual(got, []string{"new.txt"}) {
		t.Errorf("listing after a same-size rewrite = %v, want [new.txt]", got)
	}
}

// TestIssue1187UnchangedTarKeepsItsIndex: the check must not throw the cache
// away, or there is no cache: a second open of the same archive reuses the
// index file it left.
func TestIssue1187UnchangedTarKeepsItsIndex(t *testing.T) {
	isolateUserCache(t)
	path := filepath.Join(t.TempDir(), "steady.tar")
	writeIssue1187Tar(t, path, map[string]string{"a.txt": "a"})

	listTarRoot(t, path)
	cacheDir := filepath.Join(os.Getenv("XDG_CACHE_HOME"), "f4", "tar-indexes")
	first := indexFilesIn(t, cacheDir)
	if len(first) == 0 {
		t.Skipf("no index file appeared in %s on this platform", cacheDir)
	}
	listTarRoot(t, path)
	if second := indexFilesIn(t, cacheDir); !reflect.DeepEqual(first, second) {
		t.Errorf("index files changed on an open of an unchanged archive: %v -> %v", first, second)
	}
}

func TestIssue1187ExternalRenameDropsOldContentIndex(t *testing.T) {
	isolateUserCache(t)
	oldPath := filepath.Join(t.TempDir(), "before.tar")
	newPath := filepath.Join(filepath.Dir(oldPath), "after.tar")
	writeIssue1187Tar(t, oldPath, map[string]string{"file.txt": "content"})

	listTarRoot(t, oldPath)
	if got := tarindexcache.Files(oldPath); len(got) != 1 {
		t.Fatalf("old archive index files = %v, want one", got)
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatal(err)
	}
	if got, want := listTarRoot(t, newPath), []string{"file.txt"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("renamed archive listing = %v, want %v", got, want)
	}
	if got := tarindexcache.Files(oldPath); len(got) != 0 {
		t.Fatalf("old archive index files after external rename = %v, want none", got)
	}
}

func TestIssue1187ExternalDeleteDropsOldContentIndex(t *testing.T) {
	isolateUserCache(t)
	oldPath := filepath.Join(t.TempDir(), "deleted.tar")
	writeIssue1187Tar(t, oldPath, map[string]string{"old.txt": "old"})
	listTarRoot(t, oldPath)
	if got := tarindexcache.Files(oldPath); len(got) != 1 {
		t.Fatalf("deleted archive index files = %v, want one", got)
	}
	if err := os.Remove(oldPath); err != nil {
		t.Fatal(err)
	}

	nextPath := filepath.Join(t.TempDir(), "next.tar")
	writeIssue1187Tar(t, nextPath, map[string]string{"new.txt": "new"})
	listTarRoot(t, nextPath)
	if got := tarindexcache.Files(oldPath); len(got) != 0 {
		t.Fatalf("deleted archive index files after cleanup = %v, want none", got)
	}
}

func TestIssue1187DisabledTarIndexCacheLeavesNoIndex(t *testing.T) {
	isolateUserCache(t)
	previous := config.App.ArchiveTarIndexCache
	config.App.ArchiveTarIndexCache = false
	t.Cleanup(func() { config.App.ArchiveTarIndexCache = previous })
	path := filepath.Join(t.TempDir(), "uncached.tar")
	writeIssue1187Tar(t, path, map[string]string{"file.txt": "content"})

	if got, want := listTarRoot(t, path), []string{"file.txt"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("listing = %v, want %v", got, want)
	}
	cacheDir := filepath.Join(os.Getenv("XDG_CACHE_HOME"), "f4", "tar-indexes")
	if got := indexFilesIn(t, cacheDir); len(got) != 0 {
		t.Fatalf("disabled cache left index files in %s: %v", cacheDir, got)
	}
}

func indexFilesIn(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".sqlite" {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}
