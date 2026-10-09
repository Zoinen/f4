package archive

import (
	"context"
	"io"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
)

// posixExtractionRecorderVFS mimics a remote POSIX backend (FishVFS, SFTP,
// FTP, SMB): Join/Dir/Abs speak "/" and a backslash is an ordinary byte of a
// file name, never a separator.
type posixExtractionRecorderVFS struct {
	vfs.VFS
	paths []string
}

func (v *posixExtractionRecorderVFS) Join(e ...string) string { return path.Join(e...) }
func (v *posixExtractionRecorderVFS) Dir(p string) string     { return path.Dir(p) }
func (v *posixExtractionRecorderVFS) Abs(p string) (string, error) {
	if path.IsAbs(p) {
		return path.Clean(p), nil
	}
	return path.Join("/", p), nil
}
func (v *posixExtractionRecorderVFS) MkDir(ctx context.Context, p string) error {
	v.paths = append(v.paths, p)
	return nil
}
func (v *posixExtractionRecorderVFS) Create(ctx context.Context, p string) (io.WriteCloser, error) {
	v.paths = append(v.paths, p)
	return v.VFS.Create(ctx, p)
}

// Extracting into a POSIX destination must build every target from the
// destination's own separator. Using filepath.FromSlash here would bake the
// Windows backslash into member paths, so a remote host would receive files
// named "help\ar.hlf" next to an empty "help" directory, and FishVFS would
// then refuse to list such a directory as an unsafe Windows entry name.
func TestArchiveExtractionToPOSIXDestinationKeepsForwardSlashes(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "bundle.zip")
	writeArchiveTestZIP(t, archivePath, map[string]string{
		"help/ar.hlf":                      "help",
		"licenses/VisRen-BSD-3-Clause.txt": "license",
		"plugins/dummy_internal/plug.exe":  "plugin",
	})
	archiveVFS := openMutableArchiveTestVFS(t, archivePath)

	destination := &posixExtractionRecorderVFS{VFS: vfs.NewNullVFS(0)}
	const dstDir = "/remote/f4test/linux"
	if err := archiveVFS.CopyBulk(
		context.Background(), []string{"."}, destination, dstDir, &dummyReporter{},
	); err != nil {
		t.Fatalf("extract into POSIX destination: %v", err)
	}

	want := map[string]bool{
		dstDir + "/help/ar.hlf":                      true,
		dstDir + "/licenses/VisRen-BSD-3-Clause.txt": true,
		dstDir + "/plugins/dummy_internal/plug.exe":  true,
	}
	for _, created := range destination.paths {
		if strings.Contains(created, `\`) {
			t.Errorf("target %q uses a backslash; a POSIX destination only splits on /", created)
		}
		delete(want, created)
	}
	if len(want) != 0 {
		t.Errorf("extracted members are missing from the POSIX destination: %v", want)
	}
}
