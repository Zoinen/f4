package vfs

import (
	"context"
	"errors"
	"os"
	"testing"
)

type failingSeeker struct{}

func (failingSeeker) Seek(int64, int) (int64, error) { return 0, errors.New("seek failed") }

func TestDiskFileWrapperReadsAndHonorsCancellation(t *testing.T) {
	path := t.TempDir() + "/disk"
	if err := os.WriteFile(path, []byte("abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	wrapper := &diskFileWrapper{File: f, size: 6}
	if wrapper.Size() != 6 {
		t.Fatalf("Size=%d", wrapper.Size())
	}
	buf := make([]byte, 3)
	if n, err := wrapper.Read(context.Background(), buf); n != 3 || err != nil || string(buf) != "abc" {
		t.Fatalf("Read=(%d,%v,%q)", n, err, buf)
	}
	if n, err := wrapper.ReadAt(context.Background(), buf, 2); n != 3 || err != nil || string(buf) != "cde" {
		t.Fatalf("ReadAt=(%d,%v,%q)", n, err, buf)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if n, err := wrapper.Read(canceled, buf); n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Read=(%d,%v)", n, err)
	}
	if n, err := wrapper.ReadAt(canceled, buf, 0); n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled ReadAt=(%d,%v)", n, err)
	}

	if size, found, err := probeSeekSize(f); size != 6 || !found || err != nil {
		t.Fatalf("probeSeekSize=(%d,%v,%v)", size, found, err)
	}
	if size, found, err := probeSeekSize(failingSeeker{}); size != 0 || found || err != nil {
		t.Fatalf("failed probe=(%d,%v,%v)", size, found, err)
	}
}

func TestDisksVFSPathAndUnsupportedOperations(t *testing.T) {
	v := NewDisksVFS()
	if !v.IsAtRoot() || v.GetPath() != "disks://" || !v.IsAbs("anything") {
		t.Fatal("unexpected disk VFS identity")
	}
	for _, path := range []string{"disks://", "", "/"} {
		if err := v.SetPath(path); err != nil {
			t.Errorf("SetPath(%q): %v", path, err)
		}
	}
	if err := v.SetPath("disks://sda"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("invalid SetPath=%v", err)
	}
	if v.Base("disks://sda") != "sda" || v.Dir("disks://sda") != "disks://" || v.Join("disks://", "sda") != "disks://sda" || v.Join() != "disks://" {
		t.Error("path helpers returned an unexpected value")
	}
	if clone, ok := v.Clone().(*DisksVFS); !ok || clone.GetPath() != "disks://" {
		t.Error("Clone did not return a disk VFS")
	}
	if _, err := v.Create(context.Background(), "disks://sda"); err == nil {
		t.Error("Create unexpectedly succeeded")
	}
	if err := v.ReadDir(context.Background(), "disks://", nil); err != nil {
		t.Errorf("ReadDir without callback: %v", err)
	}
	if err := v.PatchInPlace(context.Background(), "disks://missing", []PatchPiece{{Length: 1, Data: []byte{1}}, {Length: 0}}); err == nil {
		t.Error("PatchInPlace unexpectedly opened a missing device")
	}
}
