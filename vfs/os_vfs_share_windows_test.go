//go:build windows

package vfs

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// TestOSVFS_OpenLeavesFileToOtherPrograms is #1364 as Windows sees it. While
// the editor or the viewer holds the handle Open returned, another program
// must still be able to open the file the way .NET's File.OpenRead,
// StreamReader(path) and File.ReadLines do (read access, FILE_SHARE_READ
// only), and a writer that shares only reading must get in as well. A handle
// with write access fails both with ERROR_SHARING_VIOLATION.
func TestOSVFS_OpenLeavesFileToOtherPrograms(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(path, []byte("line1 1\nline2 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	f, err := NewOSVFS(dir).Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		what   string
		access uint32
		share  uint32
	}{
		{"reader sharing only reading (.NET File.OpenRead)", windows.GENERIC_READ, windows.FILE_SHARE_READ},
		{"writer sharing only reading", windows.GENERIC_WRITE, windows.FILE_SHARE_READ},
	} {
		h, err := windows.CreateFile(name, c.access, c.share, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
		if err != nil {
			t.Errorf("%s: CreateFile while the Open handle is held: %v", c.what, err)
			continue
		}
		_ = windows.CloseHandle(h)
	}
}
