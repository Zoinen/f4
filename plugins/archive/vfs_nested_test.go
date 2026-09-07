package archive

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/zip"
	"github.com/unxed/zipper/archive"
)

func TestArchiveVFS_NestedZip(t *testing.T) {
	tmpDir := t.TempDir()

	innerZipPath := filepath.Join(tmpDir, "inner.zip")
	innerF, err := os.Create(innerZipPath)
	if err != nil {
		t.Fatal(err)
	}
	innerZw := zip.NewWriter(innerF)
	innerFile, err := innerZw.Create("test.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := innerFile.Write([]byte("nested file content")); err != nil {
		t.Fatal(err)
	}
	if err := innerZw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := innerF.Close(); err != nil {
		t.Fatal(err)
	}

	outerZipPath := filepath.Join(tmpDir, "outer.zip")
	opts := archive.Options{
		Method: "zstd",
		Solid:  true,
		Xattrs: true,
	}
	a, err := archive.NewArchiver(outerZipPath, tmpDir, opts)
	if err != nil {
		t.Fatal(err)
	}

	innerFi, _ := os.Stat(innerZipPath)
	err = a.Archive(context.Background(), map[string]os.FileInfo{
		innerZipPath: innerFi,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}

	vOuter, err := NewArchiveVFS(&vfs.OSVFS{}, outerZipPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := vOuter.Close(); err != nil {
			t.Errorf("close outer archive VFS: %v", err)
		}
	})

	solidPath := vOuter.Join(outerZipPath, "Solid.zip")
	t.Logf("Opening Solid.zip: %q", solidPath)
	vSolid, err := NewArchiveVFS(vOuter, solidPath)
	if err != nil {
		t.Fatalf("FAILED to open Solid.zip: %v", err)
	}
	t.Cleanup(func() {
		if err := vSolid.Close(); err != nil {
			t.Errorf("close solid archive VFS: %v", err)
		}
	})

	nestedPath := vSolid.Join(solidPath, "inner.zip")
	t.Logf("Opening nested archive VFS: %q", nestedPath)

	vInner, err := NewArchiveVFS(vSolid, nestedPath)
	if err != nil {
		t.Fatalf("FAILED to open nested ZIP (inner.zip): %v", err)
	}
	t.Cleanup(func() {
		if err := vInner.Close(); err != nil {
			t.Errorf("close nested archive VFS: %v", err)
		}
	})

	rc, err := vInner.Open(context.Background(), vInner.Join(nestedPath, "test.txt"))
	if err != nil {
		t.Fatalf("FAILED to open file inside nested ZIP: %v", err)
	}
	defer func() { _ = rc.Close() }()

	data, err := io.ReadAll(ctxReader{rc, context.Background()})
	if err != nil {
		t.Fatalf("FAILED to read content from nested ZIP: %v", err)
	}

	if string(data) != "nested file content" {
		t.Errorf("Content mismatch: expected 'nested file content', got %q", string(data))
	}
}
