//go:build !lite

package netfox

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

type fakeInfo struct {
	name string
	size int64
	dir  bool
}

func (i fakeInfo) Name() string       { return i.name }
func (i fakeInfo) Size() int64        { return i.size }
func (i fakeInfo) Mode() fs.FileMode  { return 0 }
func (i fakeInfo) ModTime() time.Time { return time.Unix(1000, 0) }
func (i fakeInfo) IsDir() bool        { return i.dir }
func (i fakeInfo) Sys() any           { return nil }

type fakeSMBFile struct {
	*bytes.Reader
	closed bool
}

func (f *fakeSMBFile) Close() error { f.closed = true; return nil }
func (f *fakeSMBFile) Size() int64  { return int64(f.Len()) }

// fakeSMB serves one share "docs" (and a hidden "IPC$"):
// docs/a.txt, docs/sub/b.txt.
type fakeSMB struct {
	closes  int
	listErr error
	ops     []string
	written string
}

func (b *fakeSMB) ListShares() ([]string, error) {
	if b.listErr != nil {
		return nil, b.listErr
	}
	return []string{"docs", "IPC$"}, nil
}

func (b *fakeSMB) ReadDir(share, dir string) ([]fs.FileInfo, error) {
	switch {
	case share == "docs" && dir == "":
		return []fs.FileInfo{fakeInfo{name: "a.txt", size: 5}, fakeInfo{name: "sub", dir: true}, fakeInfo{name: ".hid", size: 1}}, nil
	case share == "docs" && dir == "sub":
		return []fs.FileInfo{fakeInfo{name: "b.txt", size: 3}}, nil
	}
	return nil, os.ErrNotExist
}

func (b *fakeSMB) Stat(share, name string) (fs.FileInfo, error) {
	switch {
	case share == "docs" && name == "a.txt":
		return fakeInfo{name: "a.txt", size: 5}, nil
	case share == "docs" && name == "sub":
		return fakeInfo{name: "sub", dir: true}, nil
	case share == "docs" && name == "sub/b.txt":
		return fakeInfo{name: "b.txt", size: 3}, nil
	}
	return nil, os.ErrNotExist
}

func (b *fakeSMB) OpenRead(share, name string) (smbFile, error) {
	if share == "docs" && name == "a.txt" {
		return &fakeSMBFile{Reader: bytes.NewReader([]byte("hello"))}, nil
	}
	return nil, os.ErrNotExist
}

func (b *fakeSMB) MkDir(share, dir string) error {
	b.ops = append(b.ops, "mkdir "+share+":"+dir)
	return nil
}

func (b *fakeSMB) RemoveAll(share, name string) error {
	b.ops = append(b.ops, "rm "+share+":"+name)
	return nil
}

func (b *fakeSMB) Rename(share, oldName, newName string) error {
	b.ops = append(b.ops, "mv "+share+":"+oldName+" "+newName)
	return nil
}

type fakeWriter struct{ b *fakeSMB }

func (w fakeWriter) Write(p []byte) (int, error) { w.b.written += string(p); return len(p), nil }
func (w fakeWriter) Close() error                { w.b.ops = append(w.b.ops, "closed"); return nil }

func (b *fakeSMB) Create(share, name string) (io.WriteCloser, error) {
	b.ops = append(b.ops, "create "+share+":"+name)
	return fakeWriter{b}, nil
}

func (b *fakeSMB) Close() error { b.closes++; return nil }

func readAll(t *testing.T, v vfs.VFS, p string) []vfs.VFSItem {
	t.Helper()
	var got []vfs.VFSItem
	if err := v.ReadDir(context.Background(), p, func(items []vfs.VFSItem) { got = append(got, items...) }); err != nil {
		t.Fatalf("ReadDir(%q): %v", p, err)
	}
	return got
}

func names(items []vfs.VFSItem) string {
	var n []string
	for _, i := range items {
		n = append(n, i.Name)
	}
	return strings.Join(n, ",")
}

func TestSplitSMBPath(t *testing.T) {
	for in, want := range map[string][2]string{
		"/": {"", ""}, "": {"", ""}, "/docs": {"docs", ""}, "/docs/": {"docs", ""},
		"/docs/a/b": {"docs", "a/b"}, "docs/x": {"docs", "x"}, "/docs/../other/x": {"other", "x"},
	} {
		if share, rel := splitSMBPath(in); share != want[0] || rel != want[1] {
			t.Errorf("splitSMBPath(%q) = %q, %q; want %q, %q", in, share, rel, want[0], want[1])
		}
	}
}

func TestSMBVFSBrowsesSharesAndDirectories(t *testing.T) {
	backend := &fakeSMB{}
	v := newSMBVFS(nil, backend, "srv")
	if v.GetTitle() != "srv" || !v.IsAtRoot() || v.GetPath() != "/" || !v.IsAbs("/x") || v.IsAbs("x") {
		t.Fatalf("fresh VFS: title=%q root=%v path=%q", v.GetTitle(), v.IsAtRoot(), v.GetPath())
	}

	root := readAll(t, v, "/")
	if names(root) != "docs,IPC$" || !root[0].IsDir || root[0].IsHidden || !root[1].IsHidden {
		t.Errorf("root = %+v", root)
	}
	if got := names(readAll(t, v, "/docs")); got != "a.txt,sub,.hid" {
		t.Errorf("/docs = %q", got)
	}

	if err := v.SetPath("/docs/sub"); err != nil {
		t.Fatal(err)
	}
	if v.IsAtRoot() || v.GetPath() != "/docs/sub" {
		t.Errorf("path = %q", v.GetPath())
	}
	if got := names(readAll(t, v, ".")); got != "b.txt" {
		t.Errorf("relative listing = %q", got)
	}
	if abs, _ := v.Abs("b.txt"); abs != "/docs/sub/b.txt" {
		t.Errorf("Abs = %q", abs)
	}
	if err := v.SetPath("/docs/a.txt"); err == nil {
		t.Error("SetPath into a file succeeded")
	}
	if err := v.SetPath("/nope"); err == nil {
		t.Error("SetPath to a missing share succeeded")
	}
	if v.Join("a", "b") != "a/b" || v.Base("/a/b") != "b" || v.Dir("/a/b") != "/a" {
		t.Error("path helpers")
	}
}

func TestSMBVFSStat(t *testing.T) {
	v := newSMBVFS(nil, &fakeSMB{}, "srv")
	ctx := context.Background()
	if it, err := v.Stat(ctx, "/"); err != nil || !it.IsDir {
		t.Errorf("Stat(/) = %+v, %v", it, err)
	}
	if it, err := v.Stat(ctx, "/DOCS"); err != nil || it.Name != "docs" || !it.IsDir {
		t.Errorf("Stat(/DOCS) = %+v, %v", it, err)
	}
	if _, err := v.Stat(ctx, "/missing"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Stat(/missing) error = %v", err)
	}
	if it, err := v.Stat(ctx, "/docs/a.txt"); err != nil || it.IsDir || it.Size != 5 || !it.SizeKnown {
		t.Errorf("Stat(a.txt) = %+v, %v", it, err)
	}
	if _, err := v.Stat(ctx, "/docs/none"); err == nil {
		t.Error("Stat of a missing file succeeded")
	}
	bad := newSMBVFS(nil, &fakeSMB{listErr: errors.New("denied")}, "srv")
	if _, err := bad.Stat(ctx, "/docs"); err == nil {
		t.Error("Stat with a failing share list succeeded")
	}
	if err := bad.ReadDir(ctx, "/", func([]vfs.VFSItem) {}); err == nil {
		t.Error("ReadDir of the root with a failing share list succeeded")
	}
}

func TestSMBVFSReadsFiles(t *testing.T) {
	v := newSMBVFS(nil, &fakeSMB{}, "srv")
	ctx := context.Background()
	f, err := v.Open(ctx, "/docs/a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if f.Size() != 5 {
		t.Errorf("size = %d", f.Size())
	}
	buf := make([]byte, 3)
	if n, err := f.ReadAt(ctx, buf, 2); n != 3 || (err != nil && err != io.EOF) || string(buf) != "llo" {
		t.Errorf("ReadAt = %d %v %q", n, err, buf)
	}
	all := make([]byte, 5)
	if n, _ := f.Read(ctx, all); n != 5 || string(all) != "hello" {
		t.Errorf("Read = %d %q", n, all)
	}
	if err := f.Close(); err != nil {
		t.Error(err)
	}
	if _, err := v.Open(ctx, "/docs"); err == nil {
		t.Error("opened a share as a file")
	}
	if _, err := v.Open(ctx, "/"); err == nil {
		t.Error("opened the root as a file")
	}
	if _, err := v.Open(ctx, "/docs/none"); err == nil {
		t.Error("opened a missing file")
	}
}

func TestSMBVFSMutations(t *testing.T) {
	backend := &fakeSMB{}
	v := newSMBVFS(nil, backend, "srv")
	ctx := context.Background()
	if !v.GetCapabilities().HasWrite || !v.GetCapabilities().HasRandomAccess {
		t.Errorf("capabilities = %+v", v.GetCapabilities())
	}
	if err := v.MkDir(ctx, "/docs/new"); err != nil {
		t.Fatal(err)
	}
	if err := v.Remove(ctx, "/docs/sub"); err != nil {
		t.Fatal(err)
	}
	if err := v.Rename(ctx, "/docs/a.txt", "/DOCS/b.txt"); err != nil {
		t.Fatal(err)
	}
	w, err := v.Create(ctx, "/docs/n.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("data"))
	_ = w.Close()
	want := []string{"mkdir docs:new", "rm docs:sub", "mv docs:a.txt b.txt", "create docs:n.txt", "closed"}
	if strings.Join(backend.ops, "|") != strings.Join(want, "|") || backend.written != "data" {
		t.Errorf("backend saw %v (wrote %q), want %v", backend.ops, backend.written, want)
	}

	// The root and a share itself are not paths a mutation may touch, and a
	// rename does not cross shares.
	for i, err := range []error{
		v.MkDir(ctx, "/"), v.MkDir(ctx, "/docs"), v.Remove(ctx, "/docs"), v.Rename(ctx, "/docs", "/docs/x"),
		v.Rename(ctx, "/docs/a", "/docs"),
	} {
		if !errors.Is(err, errSMBNoShare) {
			t.Errorf("share-level mutation %d error = %v, want errSMBNoShare", i, err)
		}
	}
	if err := v.Rename(ctx, "/docs/a", "/other/a"); !errors.Is(err, errSMBOtherShare) {
		t.Errorf("cross-share rename error = %v", err)
	}
	if _, err := v.Create(ctx, "/docs"); !errors.Is(err, errSMBNoShare) {
		t.Errorf("Create of a share error = %v", err)
	}
	if err := v.SetAttributes(ctx, "/docs/a.txt", vfs.VFSItem{}); err == nil {
		t.Error("SetAttributes succeeded")
	}
	if ch, err := v.Search(ctx, "/", "x"); ch != nil || err != nil {
		t.Errorf("Search = %v, %v", ch, err)
	}
	if v.ParentVFS() != nil {
		t.Error("ParentVFS")
	}
}

// TestSMBVFSCloneSharesTheConnection: the backend is closed with the last of
// a VFS and its clones, and Close is idempotent per VFS.
func TestSMBVFSCloneSharesTheConnection(t *testing.T) {
	backend := &fakeSMB{}
	v := newSMBVFS(nil, backend, "srv")
	if err := v.SetPath("/docs"); err != nil {
		t.Fatal(err)
	}
	c := v.Clone()
	if c.GetPath() != "/docs" {
		t.Errorf("clone path = %q", c.GetPath())
	}
	_ = v.Close()
	_ = v.Close()
	if backend.closes != 0 {
		t.Fatalf("backend closed with a clone still open (%d)", backend.closes)
	}
	_ = c.Close()
	if backend.closes != 1 {
		t.Errorf("backend closes = %d, want 1", backend.closes)
	}
	var nilVFS *smbVFS
	if err := nilVFS.Close(); err != nil {
		t.Error(err)
	}
}

func TestSMBURIProviderRejectsBadURLs(t *testing.T) {
	p := &smbURIProvider{}
	if p.Scheme() != "smb" {
		t.Fatalf("scheme = %q", p.Scheme())
	}
	for raw, want := range map[string]string{"://": "smb:", "smb:///share": "no host"} {
		if _, err := p.OpenURI(context.Background(), nil, raw); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("OpenURI(%q) error = %v, want %q", raw, err, want)
		}
	}
}

func TestBackendPath(t *testing.T) {
	if got := backendPath("a/b/c.txt"); got != `a\b\c.txt` {
		t.Errorf("backendPath = %q", got)
	}
}

func TestSMBConnectionTypeIsListed(t *testing.T) {
	listed := map[string]bool{}
	for _, p := range GetProtocols() {
		listed[p] = true
	}
	if !listed["smb"] {
		t.Fatalf("protocols = %v, want smb", GetProtocols())
	}
	ph := &smbProtocolHandler{}
	if ph.Prefix() != "smb" || ph.DefaultPort() != "445" {
		t.Errorf("smb handler = %q port %q", ph.Prefix(), ph.DefaultPort())
	}
	if ui, cleanup := ph.BuildExtraUI(&NetFoxConfig{}, 0, 0, 10, 1); ui != nil {
		cleanup()
		t.Error("smb has no extra UI")
	}
	p := &smbProvider{}
	if p.Name() == "" || p.Priority() != 100 {
		t.Errorf("provider = %q/%d", p.Name(), p.Priority())
	}
	if p.CanOpen(context.Background(), nil, "x") {
		t.Error("CanOpen accepted a non-NetFox parent")
	}
	if _, err := p.Open(context.Background(), nil, "x"); err == nil {
		t.Error("Open with a non-NetFox parent succeeded")
	}
}
