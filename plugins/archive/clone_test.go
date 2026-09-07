package archive

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

func TestArchiveVFSCloneHasIndependentPath(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "sample.zip")
	if err := os.WriteFile(archivePath, archiveFixtureZIP(t, 128), 0o600); err != nil {
		t.Fatal(err)
	}

	original, err := NewArchiveVFS(vfs.NewOSVFS(tmp), "sample.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer original.Close()

	if err := original.SetPath(original.Join(original.GetPath(), "folder")); err != nil {
		t.Fatal(err)
	}
	originalPath := original.GetPath()

	cloned := original.Clone()
	clone, ok := cloned.(*ArchiveVFS)
	if !ok {
		t.Fatalf("clone type = %T, want *ArchiveVFS", cloned)
	}
	defer clone.Close()
	if clone == original {
		t.Fatal("archive clone shares the source VFS")
	}
	if clone.GetPath() != originalPath {
		t.Fatalf("clone path = %q, want %q", clone.GetPath(), originalPath)
	}

	if err := clone.SetPath(clone.Join(clone.GetPath(), "..")); err != nil {
		t.Fatal(err)
	}
	if got := original.GetPath(); got != originalPath {
		t.Fatalf("changing clone path changed source path to %q", got)
	}
}

func TestArchiveVFSRemoteCloneRetainsMaterialization(t *testing.T) {
	prepareArchiveCacheTest(t)
	remote := &remoteArchiveFixtureVFS{
		session: new(int), uri: "cloud://test/clone/sample.zip", name: "sample.zip",
		data: archiveFixtureZIP(t, 128), modified: time.Now(), revision: "clone-revision",
	}

	original, err := NewArchiveVFSContext(context.Background(), remote, remote.uri)
	if err != nil {
		t.Fatal(err)
	}
	var clone *ArchiveVFS
	t.Cleanup(func() {
		closeArchiveFixtureImmediately(original)
		if clone != nil {
			closeArchiveFixtureImmediately(clone)
		}
		closeSharedArchiveMaterializations()
	})
	if err := original.SetPath(original.Join(original.GetPath(), "folder")); err != nil {
		t.Fatal(err)
	}
	originalPath := original.GetPath()

	cloned := original.Clone()
	var ok bool
	clone, ok = cloned.(*ArchiveVFS)
	if !ok {
		t.Fatalf("clone type = %T, want *ArchiveVFS", cloned)
	}
	if clone == original {
		t.Fatal("remote archive clone shares the source VFS")
	}
	if clone.backingPath != original.backingPath {
		t.Fatalf("clone did not reuse the cached materialization: %q != %q", clone.backingPath, original.backingPath)
	}

	closeArchiveFixtureImmediately(original)
	if _, err := os.Stat(clone.backingPath); err != nil {
		t.Fatalf("source cleanup removed backing used by clone: %v", err)
	}
	clonePath := clone.GetPath()
	if err := clone.SetPath(clone.Join(clone.GetPath(), "..")); err != nil {
		t.Fatal(err)
	}
	if got := original.GetPath(); got != originalPath {
		t.Fatalf("changing remote clone path changed source path to %q", got)
	}
	if clone.GetPath() == clonePath {
		t.Fatal("changing remote clone path did not change the clone")
	}
	backingPath := clone.backingPath
	closeArchiveFixtureImmediately(clone)
	closeSharedArchiveMaterializations()
	if _, err := os.Stat(backingPath); !os.IsNotExist(err) {
		t.Fatalf("shared backing survived final cleanup: %v", err)
	}
}
