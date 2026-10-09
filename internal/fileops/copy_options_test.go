package fileops

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/unxed/f4/vfs"
)

// flakySource fails its first failRead Reads and its first failReadAt ReadAts.
type flakySource struct {
	data       []byte
	pos        int
	failRead   int
	failReadAt int
}

var errBadSector = errors.New("bad sector")

func (f *flakySource) Read(_ context.Context, p []byte) (int, error) {
	if f.failRead > 0 {
		f.failRead--
		return 0, errBadSector
	}
	if f.pos >= len(f.data) {
		return 0, io.EOF
	}
	n := copy(p, f.data[f.pos:])
	f.pos += n
	return n, nil
}

func (f *flakySource) ReadAt(_ context.Context, p []byte, off int64) (int, error) {
	if f.failReadAt > 0 {
		f.failReadAt--
		return 0, errBadSector
	}
	if off >= int64(len(f.data)) {
		return 0, io.EOF
	}
	n := copy(p, f.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (f *flakySource) Size() int64  { return int64(len(f.data)) }
func (f *flakySource) Close() error { return nil }

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

// TestPumpFileReadAttempts pins "Read attempts" (#722): failures in a row, a
// retry by offset, and no retry at all unless read errors are ignored.
func TestPumpFileReadAttempts(t *testing.T) {
	data := bytes.Repeat([]byte("0123456789abcdef"), 20000)
	buf := make([]byte, 128*1024)

	t.Run("a retry at the same offset finishes the file", func(t *testing.T) {
		src := &flakySource{data: data, failRead: 1, failReadAt: 1}
		var out bytes.Buffer
		state := &FileOpState{IgnoreReadErrors: true, ReadAttempts: 3}
		if err := pumpFile(context.Background(), state, src, &out, buf, "f"); err != nil {
			t.Fatalf("pumpFile: %v", err)
		}
		if !bytes.Equal(out.Bytes(), data) {
			t.Fatalf("copied %d bytes, want the %d source bytes", out.Len(), len(data))
		}
	})

	t.Run("the last attempt gives up as a read failure", func(t *testing.T) {
		src := &flakySource{data: data, failRead: 1, failReadAt: 5}
		state := &FileOpState{IgnoreReadErrors: true, ReadAttempts: 3}
		err := pumpFile(context.Background(), state, src, io.Discard, buf, "f")
		var failure *readFailure
		if !errors.As(err, &failure) || !errors.Is(err, errBadSector) {
			t.Fatalf("pumpFile = %v, want a read failure wrapping the source error", err)
		}
	})

	t.Run("without ignoring, the first failure is final", func(t *testing.T) {
		src := &flakySource{data: data, failRead: 1, failReadAt: 1}
		err := pumpFile(context.Background(), &FileOpState{ReadAttempts: 3}, src, io.Discard, buf, "f")
		if !errors.Is(err, errBadSector) || src.failReadAt != 1 {
			t.Fatalf("pumpFile = %v with %d ReadAt failures left, want the error and no retry", err, src.failReadAt)
		}
	})

	t.Run("a write failure is told apart", func(t *testing.T) {
		src := &flakySource{data: data}
		err := pumpFile(context.Background(), &FileOpState{IgnoreReadErrors: true}, src, failingWriter{}, buf, "f")
		var failure *writeFailure
		if !errors.As(err, &failure) {
			t.Fatalf("pumpFile = %v, want a write failure", err)
		}
	})
}

func TestIgnoredReadErrorLeavesOnlyThatFileBehind(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits")
	}
	if os.Geteuid() == 0 {
		t.Skip("root reads a file without read permission")
	}
	src, dst := t.TempDir(), t.TempDir()
	tree := filepath.Join(src, "tree")
	if err := os.Mkdir(tree, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFileWithRights(t, filepath.Join(tree, "good.txt"), 0o644)
	writeFileWithRights(t, filepath.Join(tree, "bad.txt"), 0o000)

	state := &FileOpState{Buffer: make([]byte, 32*1024), IgnoreReadErrors: true, ReadAttempts: 2}
	if err := recursiveCopy(context.Background(), vfs.NewOSVFS(src), tree, vfs.NewOSVFS(dst), filepath.Join(dst, "tree"), state, 0); err != nil {
		t.Fatalf("copy with read errors ignored: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "tree", "good.txt")); err != nil {
		t.Errorf("the readable file was not copied: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dst, "tree", "bad.txt")); !os.IsNotExist(err) {
		t.Errorf("the unreadable file left something at the destination: %v", err)
	}
	if state.FailedCount != 1 {
		t.Errorf("FailedCount = %d, want 1", state.FailedCount)
	}
}

func TestCopySymlinkContentsChoice(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symbolic links needs a privilege on Windows")
	}
	src := t.TempDir()
	writeFileWithRights(t, filepath.Join(src, "target.txt"), 0o644)
	if err := os.Symlink("target.txt", filepath.Join(src, "link")); err != nil {
		t.Fatal(err)
	}

	t.Run("off copies the link itself", func(t *testing.T) {
		dst := t.TempDir()
		state := &FileOpState{Buffer: make([]byte, 32*1024), CopySymlinksAsLinks: true}
		if err := recursiveCopy(context.Background(), vfs.NewOSVFS(src), filepath.Join(src, "link"), vfs.NewOSVFS(dst), filepath.Join(dst, "link"), state, 0); err != nil {
			t.Fatal(err)
		}
		if target, err := os.Readlink(filepath.Join(dst, "link")); err != nil || target != "target.txt" {
			t.Errorf("destination link points at %q (%v), want the unchanged relative target", target, err)
		}
	})

	t.Run("on copies what it points at", func(t *testing.T) {
		dst := t.TempDir()
		state := &FileOpState{Buffer: make([]byte, 32*1024)}
		if err := recursiveCopy(context.Background(), vfs.NewOSVFS(src), filepath.Join(src, "link"), vfs.NewOSVFS(dst), filepath.Join(dst, "link"), state, 0); err != nil {
			t.Fatal(err)
		}
		info, err := os.Lstat(filepath.Join(dst, "link"))
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			t.Errorf("destination is %v (%v), want a regular file with the target's contents", info, err)
		}
	})
}

func TestInheritReachesARenamedTree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits")
	}
	dst := t.TempDir()
	if err := os.Chmod(dst, 0o750); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(dst, "moved")
	if err := os.Mkdir(moved, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(moved, 0o777); err != nil {
		t.Fatal(err)
	}
	writeFileWithRights(t, filepath.Join(moved, "f.txt"), 0o777)

	inheritMovedTree(context.Background(), &FileOpState{AccessRights: AccessRightsInherit}, vfs.NewOSVFS(dst), moved, 0)

	if got := fileRights(t, moved); got != 0o750 {
		t.Errorf("renamed folder has rights %04o, want 0750 from its new parent", got)
	}
	if got := fileRights(t, filepath.Join(moved, "f.txt")); got != 0o640 {
		t.Errorf("file inside it has rights %04o, want 0640", got)
	}
}

func TestFileOpOptionsApplyTo(t *testing.T) {
	copyState := &FileOpState{}
	FileOpOptions{ExistingFiles: ExistingFilesSkip}.applyTo(copyState, false)
	if !copyState.SkipAll || copyState.OverwriteAll || copyState.CopySymlinksAsLinks || copyState.ReadAttempts != 1 {
		t.Errorf("copy state = %+v, want skip-all, links followed, one read attempt", copyState)
	}

	moveState := &FileOpState{}
	FileOpOptions{ExistingFiles: ExistingFilesOverwrite, ReadAttempts: 1000}.applyTo(moveState, true)
	if !moveState.OverwriteAll || !moveState.CopySymlinksAsLinks || moveState.ReadAttempts != MaxReadAttempts {
		t.Errorf("move state = %+v, want overwrite-all, links as links, attempts capped", moveState)
	}

	if !(FileOpOptions{}).bulkCompatible() || (FileOpOptions{ExistingFiles: ExistingFilesSkip}).bulkCompatible() {
		t.Error("only the default choices may leave the operation to a bulk copier")
	}
	if ExistingFilesModeFromChoice(7) != ExistingFilesAsk {
		t.Error("an unknown list position must ask")
	}
}
