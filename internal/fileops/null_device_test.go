package fileops

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestIsNullDeviceDest(t *testing.T) {
	cases := []struct {
		dest, goos string
		want       bool
	}{
		{`\\?\nul\`, "windows", true},
		{`\\?\NUL`, "linux", true},
		{`\\.\nul`, "windows", true},
		{`//./nul/`, "linux", true},
		{"/dev/null", "linux", true},
		{"/dev/null/", "darwin", true},
		{"  /dev/null  ", "linux", true},
		{"nul", "windows", true},
		{`NUL:`, "windows", true},
		{"nul", "linux", false},
		{"nul:", "darwin", false},
		{`C:\nul`, "windows", false},
		{`C:\data\nul\`, "windows", false},
		{"/dev/nullable", "linux", false},
		{"/tmp/dev/null", "linux", false},
		{"nul*", "windows", false},
		{"", "windows", false},
	}
	for _, c := range cases {
		if got := IsNullDeviceDest(c.dest, c.goos); got != c.want {
			t.Errorf("IsNullDeviceDest(%q, %q) = %v, want %v", c.dest, c.goos, got, c.want)
		}
	}
}

func TestDiscardVFS(t *testing.T) {
	ctx := context.Background()
	d := discardVFS{}

	if st, err := d.Stat(ctx, "/"); err != nil || !st.IsDir {
		t.Fatalf("Stat(/) = %+v, %v; want a directory", st, err)
	}
	for _, p := range []string{"/a", "/a/b.txt", "b.txt"} {
		if _, err := d.Stat(ctx, p); !os.IsNotExist(err) {
			t.Errorf("Stat(%q) err = %v, want not-exist: nothing is kept", p, err)
		}
	}
	if err := d.MkDir(ctx, "/a"); err != nil {
		t.Errorf("MkDir: %v", err)
	}
	w, err := d.Create(ctx, "/a/b.txt")
	if err != nil {
		t.Fatal(err)
	}
	if n, err := w.Write([]byte("payload")); n != 7 || err != nil {
		t.Errorf("Write = %d, %v; want 7, nil", n, err)
	}
	if err := w.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if _, err := d.Stat(ctx, "/a/b.txt"); !os.IsNotExist(err) {
		t.Errorf("a written file is visible afterwards: %v", err)
	}
	if err := d.Rename(ctx, "/a", "/b"); err == nil {
		t.Error("Rename succeeded on a destination that keeps nothing")
	}
	if got := d.Join("/", "x", "y"); got != "/x/y" {
		t.Errorf("Join = %q", got)
	}
	if abs, _ := d.Abs("x"); abs != "/x" {
		t.Errorf("Abs = %q", abs)
	}
	if d.Clone() == nil || d.ParentVFS() != nil || !d.IsAtRoot() || d.GetPath() != "/" || !d.IsAbs("/x") {
		t.Error("navigation helpers of discardVFS are inconsistent")
	}
	if err := d.SetPath("/x"); err != nil {
		t.Errorf("SetPath: %v", err)
	}
	if err := d.SetAttributes(ctx, "/x", vfs.VFSItem{}); err != nil {
		t.Errorf("SetAttributes: %v", err)
	}
	if err := d.Remove(ctx, "/x"); err != nil {
		t.Errorf("Remove: %v", err)
	}
	if !d.GetCapabilities().HasWrite {
		t.Error("discardVFS must declare that it can be written")
	}
	if _, err := d.Open(ctx, "/x"); err == nil {
		t.Error("Open succeeded on a destination that keeps nothing")
	}
	if err := d.ReadDir(ctx, "/", nil); err != nil {
		t.Errorf("ReadDir: %v", err)
	}
	if ch, err := d.Search(ctx, "/", "*"); ch != nil || err != nil {
		t.Errorf("Search = %v, %v", ch, err)
	}
	if d.Base("/a/b") != "b" || d.Dir("/a/b") != "/a" || d.Close() != nil {
		t.Error("Base/Dir/Close are inconsistent")
	}
}

// countingVFS counts the bytes read out of the source files, which is what a
// copy to the null device is for: every file is read through, nothing is
// kept.
type countingVFS struct {
	vfs.VFS
	read *atomic.Int64
}

func (c countingVFS) Open(ctx context.Context, p string) (vfs.ReadAtCloser, error) {
	f, err := c.VFS.Open(ctx, p)
	if err != nil {
		return nil, err
	}
	return countingFile{ReadAtCloser: f, read: c.read}, nil
}

type countingFile struct {
	vfs.ReadAtCloser
	read *atomic.Int64
}

func (f countingFile) Read(ctx context.Context, p []byte) (int, error) {
	n, err := f.ReadAtCloser.Read(ctx, p)
	f.read.Add(int64(n))
	return n, err
}

func (f countingFile) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	n, err := f.ReadAtCloser.ReadAt(ctx, p, off)
	f.read.Add(int64(n))
	return n, err
}

// F5 onto the null device reads the selected files, tree included, and
// leaves nothing behind: no file named after the device appears next to the
// sources, and the sources stay.
func TestExecuteFileOp_CopyToNullDeviceReadsEverythingKeepsNothing(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	srcRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(srcRoot, "tree", "deep"), 0o700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"single.txt":                           "0123456789",
		"tree/a.txt":                           "abcde",
		filepath.Join("tree", "deep", "b.txt"): "xyz",
	}
	var total int64
	for rel, body := range files {
		if err := os.WriteFile(filepath.Join(srcRoot, rel), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		total += int64(len(body))
	}

	for _, dest := range []string{"/dev/null", "/dev/null/", `\\?\nul\`, `\\.\NUL`} {
		var read atomic.Int64
		src := countingVFS{VFS: vfs.NewOSVFS(srcRoot), read: &read}
		done := make(chan struct{})
		ExecuteFileOpAt(src, vfs.NewOSVFS(srcRoot), srcRoot, []string{"single.txt", "tree"}, dest, false, 2, func() { close(done) })
		waitForFileOpTest(t, done)

		if got := read.Load(); got != total {
			t.Errorf("copy to %q read %d bytes, want %d", dest, got, total)
		}
		entries, err := os.ReadDir(srcRoot)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 2 {
			t.Errorf("copy to %q changed the source folder: %v", dest, entries)
		}
		for rel, body := range files {
			if got, err := os.ReadFile(filepath.Join(srcRoot, rel)); err != nil || string(got) != body {
				t.Errorf("copy to %q damaged %s: %q, %v", dest, rel, got, err)
			}
		}
	}
}

// F6 onto the null device used to treat /dev/null as an existing file, offer
// to overwrite it and ask for the root password to replace the device node
// (f4#1966, Linux report). It is refused up front instead: the sources stay,
// nothing is asked, and the panels are told the operation is over.
func TestExecuteFileOp_MoveToNullDeviceIsRefused(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	srcRoot := t.TempDir()
	srcFile := filepath.Join(srcRoot, "keep.txt")
	if err := os.WriteFile(srcFile, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, dest := range []string{"/dev/null", `\\?\nul\`} {
		completed := false
		ExecuteFileOpAt(vfs.NewOSVFS(srcRoot), vfs.NewOSVFS(srcRoot), srcRoot, []string{"keep.txt"}, dest, true, 2, func() { completed = true })

		waitForDialog(t, " Error ")
		vtui.FrameManager.Pop()

		if !completed {
			t.Errorf("move to %q did not report completion", dest)
		}
		if got, err := os.ReadFile(srcFile); err != nil || string(got) != "payload" {
			t.Errorf("move to %q damaged the source: %q, %v", dest, got, err)
		}
		entries, err := os.ReadDir(srcRoot)
		if err != nil || len(entries) != 1 {
			t.Errorf("move to %q changed the source folder: %v, %v", dest, entries, err)
		}
	}
}
