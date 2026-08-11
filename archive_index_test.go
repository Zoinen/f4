//go:build !dragonfly && !netbsd

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/tar"
	"github.com/unxed/vtui"
)

type remoteArchiveIndexVFS struct {
	vfs.VFS
	absCalls int
}

func (v *remoteArchiveIndexVFS) Abs(path string) (string, error) {
	v.absCalls++
	return path, nil
}

func TestArchiveIndex_RemoteURIIsNeverTreatedAsLocalSidecar(t *testing.T) {
	remote := &remoteArchiveIndexVFS{}
	local := vfs.NewOSVFS(t.TempDir())
	handleArchiveIndexOp(
		remote,
		"cloud://yandex/11111111-1111-1111-1111-111111111111/archive.tar",
		local,
		"archive.tar",
		false,
	)
	if remote.absCalls != 0 {
		t.Fatalf("remote archive index path reached OS path handling %d times", remote.absCalls)
	}
}

func TestArchiveIndex_AutoOp(t *testing.T) {
	tmpSrc := t.TempDir()
	tmpDst := t.TempDir()
	srcVfs := vfs.NewOSVFS(tmpSrc)
	dstVfs := vfs.NewOSVFS(tmpDst)

	archiveName := "test.tar.gz"
	absOld, _ := srcVfs.Abs(archiveName)
	oldIdx, _ := tar.GetStandardIndexPath(absOld)

	// Ensure cache directory exists and create dummy index
	os.MkdirAll(filepath.Dir(oldIdx), 0755)
	os.WriteFile(oldIdx, []byte("fake index"), 0644)
	defer os.Remove(oldIdx)

	t.Run("Copy Index", func(t *testing.T) {
		newArchiveName := "copied.tar.gz"
		absNew, _ := dstVfs.Abs(newArchiveName)
		newIdx, _ := tar.GetStandardIndexPath(absNew)
		defer os.Remove(newIdx)

		handleArchiveIndexOp(srcVfs, archiveName, dstVfs, newArchiveName, false)

		if _, err := os.Stat(newIdx); os.IsNotExist(err) {
			t.Error("Archive index was not copied")
		}
		if _, err := os.Stat(oldIdx); os.IsNotExist(err) {
			t.Error("Original index was removed during copy")
		}
	})

	t.Run("Move Index", func(t *testing.T) {
		newArchiveName := "moved.tar.gz"
		absNew, _ := dstVfs.Abs(newArchiveName)
		newIdx, _ := tar.GetStandardIndexPath(absNew)
		defer os.Remove(newIdx)

		// Restore old index if previous test cleaned it
		os.WriteFile(oldIdx, []byte("fake index"), 0644)

		handleArchiveIndexOp(srcVfs, archiveName, dstVfs, newArchiveName, true)

		if _, err := os.Stat(newIdx); os.IsNotExist(err) {
			t.Error("Archive index was not moved")
		}
		if _, err := os.Stat(oldIdx); err == nil {
			t.Error("Original index still exists after move")
		}
	})
}

func TestArchiveIndex_AutoDelete(t *testing.T) {
	tmp := t.TempDir()
	v := vfs.NewOSVFS(tmp)

	t.Run("Delete single file index", func(t *testing.T) {
		name := "to_delete.tgz"
		os.WriteFile(filepath.Join(tmp, name), []byte("arc"), 0644)
		abs, _ := v.Abs(name)
		idx, _ := tar.GetStandardIndexPath(abs)
		os.MkdirAll(filepath.Dir(idx), 0755)
		os.WriteFile(idx, []byte("idx"), 0644)

		handleArchiveIndexDelete(context.Background(), v, v.Join(v.GetPath(), name))

		if _, err := os.Stat(idx); err == nil {
			t.Error("Archive index was not deleted")
		}
	})

	t.Run("Recursive delete index", func(t *testing.T) {
		os.MkdirAll(filepath.Join(tmp, "subdir"), 0755)
		name := "subdir/nested.tar.zst"
		os.WriteFile(filepath.Join(tmp, name), []byte("arc"), 0644)
		abs, _ := v.Abs(name)
		idx, _ := tar.GetStandardIndexPath(abs)
		os.WriteFile(idx, []byte("idx"), 0644)

		handleArchiveIndexDelete(context.Background(), v, v.Join(v.GetPath(), "subdir"))

		if _, err := os.Stat(idx); err == nil {
			t.Error("Nested archive index was not deleted during recursive folder cleanup")
		}
	})
}

func TestExecuteFileOp_ArchiveIndexMigration(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tmpSrc := t.TempDir()
	tmpDst := t.TempDir()
	srcVfs := vfs.NewOSVFS(tmpSrc)
	dstVfs := vfs.NewOSVFS(tmpDst)

	name := "migrate.tar.gz"
	os.WriteFile(filepath.Join(tmpSrc, name), []byte("content"), 0644)
	absOld, _ := srcVfs.Abs(name)
	oldIdx, _ := tar.GetStandardIndexPath(absOld)
	os.MkdirAll(filepath.Dir(oldIdx), 0755)
	os.WriteFile(oldIdx, []byte("index-data"), 0644)
	defer os.Remove(oldIdx)

	done := make(chan struct{})
	ExecuteFileOp(nil, srcVfs, dstVfs, []string{name}, tmpDst, true, 2, func() { close(done) })

	timeout := time.After(2 * time.Second)
loop:
	for {
		select {
		case <-done:
			break loop
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("Timeout")
		}
	}

	absNew, _ := dstVfs.Abs(name)
	newIdx, _ := tar.GetStandardIndexPath(absNew)
	defer os.Remove(newIdx)

	if _, err := os.Stat(newIdx); os.IsNotExist(err) {
		t.Error("Archive index did not migrate during ExecuteFileOp (Move)")
	}
	if _, err := os.Stat(oldIdx); err == nil {
		t.Error("Old index remained after ExecuteFileOp (Move)")
	}
}
