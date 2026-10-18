package fileops

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

// exportingOSVFS hands over a ready file for the folder "key" and for the
// file "value", the way the registry exports a key or a value as .reg.
type exportingOSVFS struct{ *vfs.OSVFS }

func (s *exportingOSVFS) TransferContent(_ context.Context, srcPath string, _ vfs.VFS) ([]byte, bool, error) {
	switch s.Base(srcPath) {
	case "key":
		return []byte("the whole key"), true, nil
	case "value":
		return []byte("one value"), true, nil
	default:
		return nil, false, nil
	}
}

func (s *exportingOSVFS) TransferName(srcPath string, _ vfs.VFS) string {
	switch s.Base(srcPath) {
	case "key", "value":
		return s.Base(srcPath) + ".reg"
	default:
		return ""
	}
}

func TestRecursiveCopyWritesTheContentTheSourceHandsOver_Issue242(t *testing.T) {
	sourceRoot := t.TempDir()
	destinationRoot := t.TempDir()
	root := filepath.Join(sourceRoot, "root")
	for _, dir := range []string{filepath.Join(root, "key", "subkey"), filepath.Join(root, "plain")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for path, data := range map[string]string{
		filepath.Join(root, "key", "subkey", "inner"): "inside the key",
		filepath.Join(root, "value"):                  "as listed",
		filepath.Join(root, "plain", "file"):          "kept",
	} {
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	source := &exportingOSVFS{OSVFS: vfs.NewOSVFS(sourceRoot)}
	destination := vfs.NewOSVFS(destinationRoot)
	target := filepath.Join(destinationRoot, "copied")
	if err := recursiveCopy(context.Background(), source, root, destination, target, &FileOpState{}, 0); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		filepath.Join(target, "key.reg"):       "the whole key",
		filepath.Join(target, "value.reg"):     "one value",
		filepath.Join(target, "plain", "file"): "kept",
	} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		if string(got) != want {
			t.Errorf("%s: got %q, want %q", path, got, want)
		}
	}
	if _, err := os.Stat(filepath.Join(target, "key")); !os.IsNotExist(err) {
		t.Errorf("the key was also copied as a folder: %v", err)
	}
}

func TestCopyInsideTheSameSourceKeepsTheItem_Issue242(t *testing.T) {
	source := &exportingOSVFS{OSVFS: vfs.NewOSVFS(t.TempDir())}
	got, err := withTransferContent(context.Background(), source, "key", source)
	if err != nil {
		t.Fatal(err)
	}
	if got != vfs.VFS(source) {
		t.Fatalf("a copy inside the same file system got the exported file")
	}
}
