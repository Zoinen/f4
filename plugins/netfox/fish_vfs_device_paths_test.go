package netfox

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestFishVFSDevicePublicPaths(t *testing.T) {
	fs := newLocalFishVFS(t)
	if got := fs.PanelIcon(); got != "network" {
		t.Fatalf("generic Fish icon = %q", got)
	}
	paths := vfs.DevicePath{Scheme: "android", Device: "Pixel 3"}
	fs.SetDevicePath(paths)
	if got := fs.PanelIcon(); got != "android-logo" {
		t.Errorf("Android Fish icon = %q", got)
	}
	dir := filepath.ToSlash(t.TempDir())
	if err := os.WriteFile(filepath.Join(dir, "a'b #?.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := fs.SetPath(paths.Public(dir)); err != nil {
		t.Fatal(err)
	}
	if fs.GetPath() != paths.Public(dir) {
		t.Fatalf("GetPath = %q", fs.GetPath())
	}
	if fs.PanelTitle(fs.GetPath()) != fs.GetPath() {
		t.Fatal("display path differs from public path")
	}
	p := fs.Join(fs.GetPath(), "a'b #?.txt")
	if p != paths.Public(dir+"/a'b #?.txt") {
		t.Fatalf("item path = %q", p)
	}
	f, err := fs.Open(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	clone := fs.Clone()
	if got := clone.(vfs.PanelIconProvider).PanelIcon(); got != "android-logo" {
		t.Errorf("Android clone icon = %q", got)
	}
	defer clone.Close()
	if clone.GetPath() != fs.GetPath() {
		t.Fatal("clone lost qualified path")
	}
	if err := fs.SetPathOptimistic(paths.Root()); err != nil || !fs.IsAtRoot() {
		t.Fatalf("root: %v", err)
	}
	if clone.GetPath() == fs.GetPath() {
		t.Fatal("clone shares cwd")
	}
	for _, foreign := range []string{"android://Other/tmp/a", "ios://Pixel 3/tmp/a"} {
		if _, err := fs.Abs(foreign); err == nil {
			t.Errorf("Abs accepted %q", foreign)
		}
		if err := fs.Remove(context.Background(), foreign); err == nil {
			t.Errorf("Remove accepted %q", foreign)
		}
	}
}

func TestFishVFSDevicePathsKeepAlternateTransportProviders(t *testing.T) {
	fs := &FishVFS{path: "/", conn: &fishConn{}}
	fs.SetDevicePath(vfs.DevicePath{Scheme: "android", Device: "Pixel 3"})
	const remote = "/sdcard/a'b #?.txt"
	check := func(p string) {
		t.Helper()
		if p != remote {
			t.Fatalf("transport received %q", p)
		}
	}
	fs.SetStatProvider("sync", func(_ context.Context, p string) (vfs.VFSItem, error) {
		check(p)
		return vfs.VFSItem{Name: "a'b #?.txt"}, nil
	})
	fs.SetReadProvider("sync", func(_ context.Context, p string) (vfs.ReadAtCloser, error) {
		check(p)
		return newFishMemoryReadHandle("body"), nil
	})
	fs.SetCreateProvider("sync", func(_ context.Context, p string) (io.WriteCloser, error) {
		check(p)
		return &fishCaptureWriteCloser{}, nil
	})
	clone := fs.CloneForParent(nil)
	p := clone.Join(clone.GetPath(), "sdcard", "a'b #?.txt")
	if _, err := clone.Stat(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	r, err := clone.Open(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	w, err := clone.Create(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestFishVFSDevicePathsKeepServerSideOperations(t *testing.T) {
	fs := newLocalFishVFS(t)
	paths := vfs.DevicePath{Scheme: "android", Device: "Phone's #1"}
	fs.SetDevicePath(paths)
	ctx := context.Background()
	root := filepath.ToSlash(t.TempDir())
	for _, name := range []string{"one.txt", "two.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("needle\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	public := paths.Public(root)
	checkPublic := func(t *testing.T, p string) {
		t.Helper()
		if p != "" && !strings.HasPrefix(p, paths.Root()) {
			t.Errorf("unqualified result: %q", p)
		}
	}
	t.Run("find", func(t *testing.T) {
		hits, err := fs.FindFiles(ctx, public, vfs.FindQuery{Masks: []string{"*.txt"}})
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) != 2 {
			t.Fatalf("found %d items", len(hits))
		}
		for _, hit := range hits {
			checkPublic(t, hit.Path)
			if _, err := fs.Stat(ctx, hit.Path); err != nil {
				t.Fatal(err)
			}
		}
	})
	t.Run("scan", func(t *testing.T) {
		stats, err := fs.Scan(ctx, public, []string{"one.txt", "two.txt"}, func(p string, _ vfs.OpStats) { checkPublic(t, p) })
		if err != nil || stats.Files != 2 {
			t.Fatalf("Scan: %+v, %v", stats, err)
		}
	})
	t.Run("duplicates", func(t *testing.T) {
		if !fs.Client().CanHash() {
			t.Skip("peer lacks hashing")
		}
		groups, err := fs.FindDuplicates(ctx, public, func(p vfs.DuplicateProgress) { checkPublic(t, p.Path) })
		if err != nil {
			t.Fatal(err)
		}
		if len(groups) != 1 || len(groups[0]) != 2 {
			t.Fatalf("groups = %v", groups)
		}
		for _, p := range groups[0] {
			checkPublic(t, p)
		}
	})
	t.Run("copy rename remove", func(t *testing.T) {
		copyPath := fs.Join(public, "copy #?.txt")
		if err := fs.Copy(ctx, fs.Join(public, "one.txt"), copyPath); err != nil {
			t.Fatal(err)
		}
		renamed := fs.Join(public, "new's.txt")
		if err := fs.Rename(ctx, copyPath, renamed); err != nil {
			t.Fatal(err)
		}
		if err := fs.Remove(ctx, renamed); err != nil {
			t.Fatal(err)
		}
	})
}

func TestFishVFSDevicePathsRejectForeignBeforeTransport(t *testing.T) {
	fs := &FishVFS{path: "/"}
	fs.SetDevicePath(vfs.DevicePath{Scheme: "android", Device: "Pixel 3"})
	ctx := context.Background()
	const p = "android://pixel 3/tmp/file"
	checks := []struct {
		name string
		run  func() error
	}{
		{name: "set", run: func() error { return fs.SetPath(p) }},
		{name: "list", run: func() error { return fs.ReadDir(ctx, p, nil) }},
		{name: "stat", run: func() error { _, err := fs.Stat(ctx, p); return err }},
		{name: "open", run: func() error { _, err := fs.Open(ctx, p); return err }},
		{name: "create", run: func() error { _, err := fs.Create(ctx, p); return err }},
		{name: "mkdir", run: func() error { return fs.MkDir(ctx, p) }},
		{name: "remove", run: func() error { return fs.Remove(ctx, p) }},
		{name: "rename", run: func() error { return fs.Rename(ctx, "/tmp/a", p) }},
		{name: "copy", run: func() error { return fs.Copy(ctx, p, "/tmp/a") }},
		{name: "attributes", run: func() error { return fs.SetAttributes(ctx, p, vfs.VFSItem{}) }},
		{name: "search", run: func() error { _, err := fs.Search(ctx, p, "text"); return err }},
		{name: "find", run: func() error { _, err := fs.FindFiles(ctx, p, vfs.FindQuery{}); return err }},
		{name: "duplicates", run: func() error { _, err := fs.FindDuplicates(ctx, p, nil); return err }},
		{name: "patch", run: func() error { return fs.PatchFile(ctx, "/tmp/a", p, nil) }},
		{name: "lines", run: func() error { _, err := fs.LineIndex(ctx, p, 0, 1); return err }},
		{name: "command", run: func() error { _, err := fs.RunCommand(ctx, p, "pwd", nil); return err }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.run(); err == nil {
				t.Fatal("foreign authority accepted")
			}
		})
	}
}
