package envman

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

// absErrVFS wraps a real OSVFS but forces Abs to fail, so resolveVFSPath's
// error-wrapping branch (which OSVFS itself never takes) can be exercised.
type absErrVFS struct {
	*vfs.OSVFS
	err error
}

func (f *absErrVFS) Abs(path string) (string, error) {
	return "", f.err
}

func TestResolveVFSPathRejectsMissingFilesystemAndEmptyName(t *testing.T) {
	if _, err := resolveVFSPath(nil, "", "file.txt"); err == nil {
		t.Fatal("resolveVFSPath accepted a nil file system")
	}

	fs := vfs.NewOSVFS(t.TempDir())
	for _, raw := range []string{"", "   ", `""`, `''`} {
		if _, err := resolveVFSPath(fs, "", raw); err == nil {
			t.Errorf("resolveVFSPath(%q) accepted an empty name", raw)
		}
	}
}

func TestResolveVFSPathStripsMatchingQuotesOnly(t *testing.T) {
	root := t.TempDir()
	fs := vfs.NewOSVFS(root)

	got, err := resolveVFSPath(fs, root, `"quoted.txt"`)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "quoted.txt")
	if got != want {
		t.Fatalf("resolveVFSPath(quoted) = %q, want %q", got, want)
	}

	got, err = resolveVFSPath(fs, root, `'quoted.txt'`)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("resolveVFSPath(single-quoted) = %q, want %q", got, want)
	}

	// Mismatched quote characters must not be stripped: the leading quote
	// stays part of the file name.
	got, err = resolveVFSPath(fs, root, `'mismatched.txt"`)
	if err != nil {
		t.Fatal(err)
	}
	want = filepath.Join(root, `'mismatched.txt"`)
	if got != want {
		t.Fatalf("resolveVFSPath(mismatched quotes) = %q, want %q", got, want)
	}
}

func TestResolveVFSPathUsesPanelPathWhenBaseIsEmpty(t *testing.T) {
	root := t.TempDir()
	fs := vfs.NewOSVFS(root)

	got, err := resolveVFSPath(fs, "", "relative.txt")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "relative.txt")
	if got != want {
		t.Fatalf("resolveVFSPath(empty base) = %q, want %q", got, want)
	}
}

func TestResolveVFSPathJoinsSuppliedBase(t *testing.T) {
	root := t.TempDir()
	fs := vfs.NewOSVFS(root)
	other := filepath.Join(root, "sub")

	got, err := resolveVFSPath(fs, other, "relative.txt")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(other, "relative.txt")
	if got != want {
		t.Fatalf("resolveVFSPath(explicit base) = %q, want %q", got, want)
	}
}

func TestResolveVFSPathKeepsAbsoluteInputAsIs(t *testing.T) {
	root := t.TempDir()
	fs := vfs.NewOSVFS(root)
	absolute := filepath.Join(root, "abs.txt")

	got, err := resolveVFSPath(fs, filepath.Join(root, "ignored"), absolute)
	if err != nil {
		t.Fatal(err)
	}
	if got != absolute {
		t.Fatalf("resolveVFSPath(absolute) = %q, want %q", got, absolute)
	}
}

func TestResolveVFSPathWrapsFilesystemAbsError(t *testing.T) {
	fs := &absErrVFS{OSVFS: vfs.NewOSVFS(t.TempDir()), err: errors.New("boom")}
	if _, err := resolveVFSPath(fs, "", "file.txt"); err == nil {
		t.Fatal("resolveVFSPath swallowed the file system's Abs error")
	}
}

func TestReadVFSFileRejectsMissingFilesystem(t *testing.T) {
	if _, err := readVFSFile(context.Background(), nil, "file.txt", 0); err == nil {
		t.Fatal("readVFSFile accepted a nil file system")
	}
}

func TestReadVFSFileReturnsExactContentUsingDefaultLimit(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "data.txt")
	want := []byte("hello environment manager\n")
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatal(err)
	}
	fs := vfs.NewOSVFS(root)

	got, err := readVFSFile(context.Background(), fs, path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("readVFSFile = %q, want %q", got, want)
	}
}

func TestReadVFSFileHandlesMultipleReadLoopIterations(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "large.txt")
	want := make([]byte, 200*1024) // several times the 32KiB read buffer.
	for i := range want {
		want[i] = byte('a' + i%26)
	}
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatal(err)
	}
	fs := vfs.NewOSVFS(root)

	got, err := readVFSFile(context.Background(), fs, path, int64(len(want)))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("readVFSFile length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("readVFSFile content differs at byte %d", i)
		}
	}
}

func TestReadVFSFileRejectsFileLargerThanLimit(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "toobig.txt")
	if err := os.WriteFile(path, make([]byte, 100), 0o600); err != nil {
		t.Fatal(err)
	}
	fs := vfs.NewOSVFS(root)

	if _, err := readVFSFile(context.Background(), fs, path, 10); err == nil {
		t.Fatal("readVFSFile accepted a file larger than the limit")
	}
}

func TestReadVFSFilePropagatesOpenError(t *testing.T) {
	root := t.TempDir()
	fs := vfs.NewOSVFS(root)
	if _, err := readVFSFile(context.Background(), fs, filepath.Join(root, "missing.txt"), 0); err == nil {
		t.Fatal("readVFSFile accepted a nonexistent file")
	}
}

func TestReadVFSFileRespectsCanceledContext(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "data.txt")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	fs := vfs.NewOSVFS(root)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readVFSFile(ctx, fs, path, 0); err == nil {
		t.Fatal("readVFSFile ignored a canceled context")
	}
}

func TestWriteVFSFileRejectsMissingFilesystem(t *testing.T) {
	if err := writeVFSFile(context.Background(), nil, "file.txt", []byte("x")); err == nil {
		t.Fatal("writeVFSFile accepted a nil file system")
	}
}

func TestWriteVFSFileRespectsCanceledContext(t *testing.T) {
	root := t.TempDir()
	fs := vfs.NewOSVFS(root)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := writeVFSFile(ctx, fs, filepath.Join(root, "file.txt"), []byte("x")); err == nil {
		t.Fatal("writeVFSFile ignored a canceled context")
	}
}

func TestWriteVFSFileWritesExactContent(t *testing.T) {
	root := t.TempDir()
	fs := vfs.NewOSVFS(root)
	path := filepath.Join(root, "out.txt")
	want := []byte("A=1\nB=2\n")

	if err := writeVFSFile(context.Background(), fs, path, want); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("written content = %q, want %q", got, want)
	}
}

func TestWriteVFSFileWritesLargePayloadInFull(t *testing.T) {
	root := t.TempDir()
	fs := vfs.NewOSVFS(root)
	path := filepath.Join(root, "large-out.txt")
	want := make([]byte, 300*1024) // several times the 32KiB write buffer.
	for i := range want {
		want[i] = byte(i % 251)
	}

	if err := writeVFSFile(context.Background(), fs, path, want); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("written length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("written content differs at byte %d", i)
		}
	}
}

func TestWriteVFSFilePropagatesCreateError(t *testing.T) {
	root := t.TempDir()
	fs := vfs.NewOSVFS(root)
	// The parent directory does not exist, so Create must fail.
	path := filepath.Join(root, "missing-dir", "out.txt")

	if err := writeVFSFile(context.Background(), fs, path, []byte("x")); err == nil {
		t.Fatal("writeVFSFile accepted a path with a missing parent directory")
	}
}

func TestWriteVFSFileWritesEmptyPayload(t *testing.T) {
	root := t.TempDir()
	fs := vfs.NewOSVFS(root)
	path := filepath.Join(root, "empty.txt")

	if err := writeVFSFile(context.Background(), fs, path, nil); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("written length = %d, want 0", len(got))
	}
}
