//go:build !windows

package hostfs

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPosixHostFSForwardsFileAndPathOperations(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested", "deep")
	if err := MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	single := filepath.Join(root, "single")
	if err := Mkdir(single, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	path := filepath.Join(single, "source.txt")

	f, err := OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if n, err := f.Write([]byte("hello")); err != nil || n != 5 {
		t.Fatalf("Write: n=%d err=%v", n, err)
	}
	if n, err := f.WriteAt([]byte("!"), 5); err != nil || n != 1 {
		t.Fatalf("WriteAt: n=%d err=%v", n, err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	buf := make([]byte, 6)
	if n, err := f.Read(buf); err != nil || n != len(buf) || string(buf) != "hello!" {
		t.Fatalf("Read: n=%d err=%v data=%q", n, err, buf)
	}
	readAt := make([]byte, 5)
	if n, err := f.ReadAt(readAt, 0); err != nil || n != len(readAt) || string(readAt) != "hello" {
		t.Fatalf("ReadAt: n=%d err=%v data=%q", n, err, readAt)
	}
	if err := f.Truncate(3); err != nil {
		t.Fatalf("Truncate: %v", err)
	}
	if info, err := f.Stat(); err != nil || info.Size() != 3 {
		t.Fatalf("File.Stat: info=%v err=%v", info, err)
	}
	if f.Fd() == 0 {
		t.Fatal("File.Fd returned stdin")
	}
	if err := f.Close(); err != nil {
		t.Fatalf("File.Close: %v", err)
	}

	f, err = Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Open.Close: %v", err)
	}
	if info, err := Stat(path); err != nil || info.Name() != "source.txt" {
		t.Fatalf("Stat: info=%v err=%v", info, err)
	}
	if info, err := Lstat(path); err != nil || info.Name() != "source.txt" {
		t.Fatalf("Lstat: info=%v err=%v", info, err)
	}
	entries, err := ReadDir(single)
	if err != nil || len(entries) != 1 || entries[0].Name() != "source.txt" {
		t.Fatalf("ReadDir: entries=%v err=%v", entries, err)
	}

	renamed := filepath.Join(single, "renamed.txt")
	if err := Rename(path, renamed); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if err := Chmod(renamed, 0o640); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	when := time.Unix(123, 456)
	if err := Chtimes(renamed, when, when); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	_ = Chown(renamed, os.Getuid(), os.Getgid())
	_ = Lchown(renamed, os.Getuid(), os.Getgid())

	link := filepath.Join(single, "link.txt")
	if err := Symlink(filepath.Base(renamed), link); err != nil {
		t.Skipf("Symlink is unavailable: %v", err)
	}
	if got, err := Readlink(link); err != nil || got != filepath.Base(renamed) {
		t.Fatalf("Readlink: got=%q err=%v", got, err)
	}
	if info, err := Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("Lstat symlink: info=%v err=%v", info, err)
	}
	hard := filepath.Join(single, "hard.txt")
	if err := Link(renamed, hard); err != nil {
		t.Skipf("Link is unavailable: %v", err)
	}
	if err := Remove(link); err != nil {
		t.Fatalf("Remove symlink: %v", err)
	}
	if err := Remove(hard); err != nil {
		t.Fatalf("Remove hard link: %v", err)
	}
	if err := Remove(renamed); err != nil {
		t.Fatalf("Remove file: %v", err)
	}
	if err := Remove(single); err != nil {
		t.Fatalf("Remove directory: %v", err)
	}
	if err := RemoveAll(filepath.Join(root, "nested")); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
}
