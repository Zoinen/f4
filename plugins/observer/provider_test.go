package observer_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/plugins/observer"
	"github.com/unxed/f4/vfs"
)

// newTestModulesDir builds a modules directory containing isoimg.wasm (the
// same fixture isoimg_e2e_test.go already builds in CI, see
// scripts/build_isoimg_test_wasm.sh), or skips the test the same way
// loadOptionalFixture does when the fixture has not been built.
func newTestModulesDir(t *testing.T) string {
	t.Helper()
	wasmBytes := loadOptionalFixture(t, isoimgWasmPath)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "isoimg.wasm"), wasmBytes, 0o600); err != nil {
		t.Fatalf("writing isoimg.wasm fixture: %v", err)
	}
	return dir
}

// newTestISO copies the shared genisoimage-built fixture (see
// isoimg_e2e_test.go) into a fresh directory and returns a vfs.OSVFS rooted
// there plus the ISO's local path, the way plugins/archive's own provider
// tests build a *vfs.OSVFS parent.
func newTestISO(t *testing.T) (vfs.VFS, string) {
	t.Helper()
	isoBytes := loadOptionalFixture(t, isoimgIsoPath)
	dir := t.TempDir()
	isoPath := filepath.Join(dir, "target.iso")
	if err := os.WriteFile(isoPath, isoBytes, 0o600); err != nil {
		t.Fatalf("writing target.iso fixture: %v", err)
	}
	return vfs.NewOSVFS(dir), isoPath
}

func TestProvider_Properties(t *testing.T) {
	p := observer.NewProvider(t.TempDir())
	if p.Name() != "observer" {
		t.Errorf("Name() = %q, want %q", p.Name(), "observer")
	}
	if p.Priority() != 5 {
		t.Errorf("Priority() = %d, want 5", p.Priority())
	}
}

func TestProvider_CanOpen_RealISO(t *testing.T) {
	modulesDir := newTestModulesDir(t)
	parent, isoPath := newTestISO(t)
	p := observer.NewProvider(modulesDir)

	if !p.CanOpen(context.Background(), parent, isoPath) {
		t.Errorf("CanOpen(%q) = false, want true for a real ISO image with isoimg.wasm installed", isoPath)
	}
}

func TestProvider_CanOpen_WrongExtension(t *testing.T) {
	modulesDir := newTestModulesDir(t)
	dir := t.TempDir()
	txtPath := filepath.Join(dir, "readme.txt")
	if err := os.WriteFile(txtPath, []byte("just a text file, not an ISO"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := observer.NewProvider(modulesDir)
	parent := vfs.NewOSVFS(dir)

	if p.CanOpen(context.Background(), parent, txtPath) {
		t.Errorf("CanOpen(%q) = true, want false: extension is not gated to isoimg", txtPath)
	}
}

func TestProvider_CanOpen_GarbageISO(t *testing.T) {
	modulesDir := newTestModulesDir(t)
	dir := t.TempDir()
	garbagePath := filepath.Join(dir, "garbage.iso")
	if err := os.WriteFile(garbagePath, make([]byte, 128*1024), 0o600); err != nil {
		t.Fatal(err)
	}
	p := observer.NewProvider(modulesDir)
	parent := vfs.NewOSVFS(dir)

	if p.CanOpen(context.Background(), parent, garbagePath) {
		t.Errorf("CanOpen(%q) = true, want false: isoimg should reject non-ISO content named *.iso", garbagePath)
	}
}

func TestProvider_CanOpen_ModuleNotInstalled(t *testing.T) {
	// loadOptionalFixture is only used to decide whether to skip -- this
	// test's whole point is that isoimg.wasm is absent from modulesDir, so
	// the ISO fixture alone is enough for it to make sense at all.
	parent, isoPath := newTestISO(t)
	p := observer.NewProvider(t.TempDir()) // empty: no isoimg.wasm here

	if p.CanOpen(context.Background(), parent, isoPath) {
		t.Error("CanOpen = true, want false when isoimg.wasm is not installed in the modules directory")
	}
}

func TestProvider_Open_BrowseTreeAndReadFile(t *testing.T) {
	modulesDir := newTestModulesDir(t)
	parent, isoPath := newTestISO(t)
	p := observer.NewProvider(modulesDir)
	ctx := context.Background()

	v, err := p.Open(ctx, parent, isoPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := v.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	if !v.IsAtRoot() {
		t.Error("a freshly opened ObserverVFS should be at its own root")
	}
	if v.GetPath() != isoPath {
		t.Errorf("GetPath() = %q, want %q", v.GetPath(), isoPath)
	}

	var items []vfs.VFSItem
	if err := v.ReadDir(ctx, "", func(chunk []vfs.VFSItem) { items = append(items, chunk...) }); err != nil {
		t.Fatalf("ReadDir(root): %v", err)
	}
	if len(items) == 0 {
		t.Fatal("ReadDir(root) returned no entries for a non-empty ISO image")
	}

	var helloName string
	for _, item := range items {
		if strings.Contains(strings.ToUpper(item.Name), "HELLO.TXT") {
			helloName = item.Name
			if item.IsDir {
				t.Errorf("entry %q reported as a directory, want a file", item.Name)
			}
			if item.Size != int64(len(helloTxtContent)) {
				t.Errorf("entry %q Size = %d, want %d", item.Name, item.Size, len(helloTxtContent))
			}
		}
	}
	if helloName == "" {
		t.Fatalf("no entry containing HELLO.TXT found at root: %+v", items)
	}

	stat, err := v.Stat(ctx, v.Join(v.GetPath(), helloName))
	if err != nil {
		t.Fatalf("Stat(%s): %v", helloName, err)
	}
	if stat.IsDir {
		t.Errorf("Stat(%s).IsDir = true, want false", helloName)
	}

	rc, err := v.Open(ctx, v.Join(v.GetPath(), helloName))
	if err != nil {
		t.Fatalf("Open(%s): %v", helloName, err)
	}
	defer func() { _ = rc.Close() }()

	got := make([]byte, rc.Size())
	if _, err := io.ReadFull(&ctxReaderAdapter{ctx: ctx, rc: rc}, got); err != nil {
		t.Fatalf("reading %s: %v", helloName, err)
	}
	if string(got) != helloTxtContent {
		t.Errorf("content of %s = %q, want %q", helloName, got, helloTxtContent)
	}
}

// ctxReaderAdapter turns a vfs.ReadAtCloser (context-aware Read) into a
// plain io.Reader for io.ReadFull, the way an f4 caller normally would
// through a small wrapper of its own.
type ctxReaderAdapter struct {
	ctx context.Context
	rc  vfs.ReadAtCloser
}

func (a *ctxReaderAdapter) Read(p []byte) (int, error) { return a.rc.Read(a.ctx, p) }

func TestProvider_Clone(t *testing.T) {
	modulesDir := newTestModulesDir(t)
	parent, isoPath := newTestISO(t)
	p := observer.NewProvider(modulesDir)
	ctx := context.Background()

	v, err := p.Open(ctx, parent, isoPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = v.Close() }()

	clone := v.Clone()
	defer func() { _ = clone.Close() }()

	if _, ok := clone.(*vfs.NullVFS); ok {
		t.Fatal("Clone() returned a NullVFS, want a working second ObserverVFS instance")
	}

	var items []vfs.VFSItem
	if err := clone.ReadDir(ctx, "", func(chunk []vfs.VFSItem) { items = append(items, chunk...) }); err != nil {
		t.Fatalf("ReadDir on clone: %v", err)
	}
	if len(items) == 0 {
		t.Error("Clone()'s ReadDir returned no entries, want the same tree as the original")
	}
}
