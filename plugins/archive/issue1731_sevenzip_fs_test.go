package archive

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"testing"
)

func openIssue1731SevenZipFS(t *testing.T, members, size int) *sevenZipFileSystem {
	t.Helper()
	archivePath := issue1731BuildSolidSevenZip(t, t.TempDir(), members, size)
	fsys, err := openArchiveFileSystem(context.Background(), archivePath, filepath.Base(archivePath), "")
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	sz, ok := fsys.(*sevenZipFileSystem)
	if !ok {
		_ = fsys.Close()
		t.Fatalf("7z opened as %T, want the one-reader *sevenZipFileSystem", fsys)
	}
	t.Cleanup(func() { _ = sz.Close() })
	return sz
}

// Every member of a solid block comes out of the one reader the filesystem
// holds, in any order, and listing and Stat do not need a member opened.
func TestIssue1731SevenZipFSReadsMembersFromOneReader(t *testing.T) {
	const members, size = 5, 48 * 1024
	sz := openIssue1731SevenZipFS(t, members, size)

	entries, err := sz.ReadDir(".")
	if err != nil || len(entries) != members {
		t.Fatalf("ReadDir = %d entries, %v; want %d", len(entries), err, members)
	}
	for _, index := range []int{0, 1, 4, 2, 3, 1} {
		name := fmt.Sprintf("member%02d.bin", index)
		info, err := sz.Stat(name)
		if err != nil || info.IsDir() || info.Size() != size {
			t.Fatalf("Stat %s = %v, %v; want a file of %d bytes", name, info, err, size)
		}
		file, err := sz.Open(name)
		if err != nil {
			t.Fatalf("open %s: %v", name, err)
		}
		data, err := io.ReadAll(file)
		_ = file.Close()
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if !bytes.Equal(data, issue1731Payload(index, size)) {
			t.Fatalf("%s: content differs from the stored member", name)
		}
	}
	if _, err := sz.Stat("missing.bin"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Stat of a missing member = %v, want fs.ErrNotExist", err)
	}
	if _, err := sz.Open("missing.bin"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Open of a missing member = %v, want fs.ErrNotExist", err)
	}
	root, err := sz.Stat(".")
	if err != nil || !root.IsDir() {
		t.Fatalf("Stat(.) = %v, %v; want a directory", root, err)
	}
}

// A password change replaces the filesystem while a handle from the old one
// may still be read: Close must not cut that handle off, and the reader goes
// away only after the handle is closed.
func TestIssue1731SevenZipFSCloseWaitsForOpenMembers(t *testing.T) {
	const members, size = 3, 64 * 1024
	sz := openIssue1731SevenZipFS(t, members, size)

	file, err := sz.Open("member01.bin")
	if err != nil {
		t.Fatal(err)
	}
	head := make([]byte, 1024)
	if _, err := io.ReadFull(file, head); err != nil {
		t.Fatal(err)
	}
	if err := sz.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	sz.mu.Lock()
	released := sz.done
	sz.mu.Unlock()
	if released {
		t.Fatal("the reader was released while a member is still open")
	}
	if _, err := sz.Open("member00.bin"); err == nil {
		t.Fatal("Open after Close succeeded")
	}
	rest, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("read after Close: %v", err)
	}
	if !bytes.Equal(append(head, rest...), issue1731Payload(1, size)) {
		t.Fatal("member read across Close differs from the stored one")
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("second member Close: %v", err)
	}
	sz.mu.Lock()
	released, open := sz.done, sz.open
	sz.mu.Unlock()
	if !released || open != 0 {
		t.Fatalf("after the last member closed: released=%v open=%d, want true 0", released, open)
	}
	if err := sz.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// ArchiveVFS opens a 7z through the one-reader filesystem, and members read
// without a progress dialog still decode once.
func TestIssue1731ArchiveVFSUsesOneSevenZipReader(t *testing.T) {
	const members, size = 4, 32 * 1024
	v, _ := openIssue1731SolidArchive(t, members, size)
	v.mu.Lock()
	inner := v.fsys.(*issue1731CountingFS).FileSystem
	v.mu.Unlock()
	if _, ok := inner.(*sevenZipFileSystem); !ok {
		t.Fatalf("ArchiveVFS holds %T for a 7z, want *sevenZipFileSystem", inner)
	}
}
