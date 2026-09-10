package vtvibe

import (
	"context"
	"errors"
	"github.com/unxed/f4/vfs"
	"io"
	"os"
	"testing"
)

func TestAIVFSNavigationAndPathRules(t *testing.T) {
	s := NewSession()
	v := NewVFS(s)
	if v.Session() != s || v.SessionKey() != s || !v.IsAtRoot() || v.GetPath() != "/" {
		t.Fatal("new VFS did not expose its session and root")
	}
	if !v.IsAbs("/ctx") || !v.IsAbs("ai://ctx") || v.IsAbs("ctx") {
		t.Fatal("IsAbs path rules are incorrect")
	}
	if v.GetTitle() != "ai" || v.PanelTitle("/ctx") != "ai://ctx" {
		t.Fatalf("title = %q, panel title = %q", v.GetTitle(), v.PanelTitle("/ctx"))
	}
	if err := v.SetPath("ai://ctx"); err != nil {
		t.Fatal(err)
	}
	if v.GetPath() != "/ctx" || v.IsAtRoot() {
		t.Fatalf("SetPath left cwd at %q", v.GetPath())
	}
	if err := v.SetPath("/missing"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("SetPath missing = %v, want not-exist", err)
	}
	if got, err := v.Abs("child"); err != nil || got != "/ctx/child" {
		t.Fatalf("Abs(child) = %q, %v", got, err)
	}
	if got := v.Base("/ctx/child"); got != "child" {
		t.Fatalf("Base = %q", got)
	}
	if got := v.Dir("child"); got != "/" {
		t.Fatalf("Dir = %q", got)
	}
	if got := v.Dir("/"); got != "/" {
		t.Fatalf("Dir(root) = %q", got)
	}
	if got := v.Join(); got != "/" {
		t.Fatalf("Join() = %q", got)
	}
	if got := v.Join("/ctx", "file"); got != "/ctx/file" {
		t.Fatalf("Join = %q", got)
	}
	for _, tc := range []struct {
		path string
		want bool
	}{
		{draftFile, true},
		{ctxDir, true},
		{ctxDir + "/file", true},
		{outDir, true},
		{outDir + "/file", true},
		{chatDir, false},
		{"/other/file", false},
	} {
		if got := writable(tc.path); got != tc.want {
			t.Errorf("writable(%q) = %t, want %t", tc.path, got, tc.want)
		}
	}
}

func TestAIVFSReadDirStatAndOpenUseSharedTree(t *testing.T) {
	s := NewSession()
	v := NewVFS(s)
	write := func(name, data string) {
		t.Helper()
		w, err := v.Create(context.Background(), name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	}
	write("/ctx/readme.txt", "hello")
	write("/out/result.txt", "answer")

	var items []vfs.VFSItem
	if err := v.ReadDir(context.Background(), "/ctx", func(got []vfs.VFSItem) { items = got }); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "readme.txt" || items[0].Size != 5 {
		t.Fatalf("ReadDir = %#v", items)
	}
	if err := v.ReadDir(context.Background(), "/ctx/readme.txt", nil); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ReadDir file = %v, want not-exist", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := v.ReadDir(ctx, "/ctx", func([]vfs.VFSItem) {}); !errors.Is(err, context.Canceled) {
		t.Fatalf("ReadDir canceled = %v", err)
	}

	root, err := v.Stat(context.Background(), "/")
	if err != nil || root.Name != "/" || !root.IsDir {
		t.Fatalf("Stat root = %#v, %v", root, err)
	}
	if item, err := v.Stat(context.Background(), "/chat/readme.txt"); err != nil || item.Name != "readme.txt" {
		t.Fatalf("Stat ctx alias = %#v, %v", item, err)
	}
	if item, err := v.Stat(context.Background(), "/out/result.txt"); err != nil || item.Name != "result.txt" {
		t.Fatalf("Stat out = %#v, %v", item, err)
	}
	if _, err := v.Stat(context.Background(), "/unknown"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat missing = %v", err)
	}

	read := func(name string) string {
		t.Helper()
		r, err := v.Open(context.Background(), name)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = r.Close() }()
		buf := make([]byte, 32)
		n, err := r.Read(context.Background(), buf)
		if err != nil && !errors.Is(err, io.EOF) {
			t.Fatal(err)
		}
		return string(buf[:n])
	}
	if got := read("/chat/readme.txt"); got != "hello" {
		t.Fatalf("Open ctx alias = %q", got)
	}
	if got := read("/out/result.txt"); got != "answer" {
		t.Fatalf("Open out = %q", got)
	}
	if _, err := v.Open(context.Background(), "/unknown"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Open missing = %v", err)
	}
}

func TestAIVFSMutationsRedirectAndProtectPaths(t *testing.T) {
	s := NewSession()
	v := NewVFS(s)
	if err := v.MkDir(context.Background(), "/chat/new"); err != nil {
		t.Fatal(err)
	}
	if item, err := v.Stat(context.Background(), "/ctx/new"); err != nil || !item.IsDir {
		t.Fatalf("redirected MkDir = %#v, %v", item, err)
	}
	if err := v.MkDir(context.Background(), "/mem/private"); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("protected MkDir = %v", err)
	}
	if err := v.MkDir(context.Background(), "/"); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("root MkDir = %v", err)
	}

	w, err := v.Create(context.Background(), "/chat/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("data"))
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Create(context.Background(), "/ctx"); !errors.Is(err, os.ErrExist) {
		t.Fatalf("Create directory = %v", err)
	}
	if _, err := v.Create(context.Background(), "/other/file"); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("Create protected path = %v", err)
	}

	if err := v.Rename(context.Background(), "/ctx/file.txt", "/out/moved.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Stat(context.Background(), "/out/moved.txt"); err != nil {
		t.Fatal(err)
	}
	if err := v.Rename(context.Background(), "/out/moved.txt", "/out/moved.txt"); !errors.Is(err, os.ErrExist) {
		t.Fatalf("Rename existing = %v", err)
	}
	if err := v.Rename(context.Background(), "/out/missing", "/out/new"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Rename missing = %v", err)
	}
	if err := v.Rename(context.Background(), "/out/moved.txt", "/other/new"); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("Rename protected = %v", err)
	}
	if err := v.Remove(context.Background(), "/out/moved.txt"); err != nil {
		t.Fatal(err)
	}
	if err := v.Remove(context.Background(), "/out/moved.txt"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Remove missing = %v", err)
	}
	if err := v.Remove(context.Background(), outDir); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("Remove out root = %v", err)
	}
	if err := v.Remove(context.Background(), "/"); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("Remove root = %v", err)
	}
	if err := v.SetAttributes(context.Background(), "/ctx", vfs.VFSItem{}); err != nil {
		t.Fatal(err)
	}
	if caps := v.GetCapabilities(); !caps.HasRandomAccess {
		t.Fatal("AIVFS should have random access")
	}
	if ch, err := v.Search(context.Background(), "/", "*"); ch != nil || err != nil {
		t.Fatalf("Search = %v, %v", ch, err)
	}
	if v.ParentVFS() != nil || v.Clone() == v || v.Close() != nil {
		t.Fatal("mount lifecycle methods are incorrect")
	}
}

func TestAIVFSMemReaderAndWriterBoundaries(t *testing.T) {
	s := NewSession()
	v := NewVFS(s)
	r := &memReader{data: []byte("hello")}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.ReadAt(ctx, make([]byte, 1), 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("ReadAt canceled = %v", err)
	}
	if n, err := r.ReadAt(context.Background(), make([]byte, 2), 99); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("ReadAt past end = %d, %v", n, err)
	}
	if n, err := r.ReadAt(context.Background(), make([]byte, 10), 3); n != 2 || !errors.Is(err, io.EOF) {
		t.Fatalf("ReadAt short = %d, %v", n, err)
	}
	if n, err := r.ReadAt(context.Background(), make([]byte, 2), 0); n != 2 || err != nil {
		t.Fatalf("ReadAt full = %d, %v", n, err)
	}
	if r.Size() != 5 || r.Close() != nil {
		t.Fatal("memReader size/close failed")
	}

	w := &memWriter{v: v, path: "/ctx/write.txt"}
	if n, err := w.Write([]byte("ok")); n != 2 || err != nil {
		t.Fatalf("Write = %d, %v", n, err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.tree.readFile("/ctx/write.txt"); !ok {
		t.Fatal("writer did not commit its file")
	}
	bad := &memWriter{v: v, path: "/ctx/too-large"}
	if _, err := bad.Write(make([]byte, maxFileBytes+1)); !errors.Is(err, errTooLarge) {
		t.Fatalf("oversized Write = %v", err)
	}
	if _, err := bad.Write([]byte("again")); !errors.Is(err, errTooLarge) || !errors.Is(bad.Close(), errTooLarge) {
		t.Fatal("failed writer did not remain failed")
	}
	if err := (&memWriter{v: v, path: "/"}).Close(); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("writer at root close = %v", err)
	}
	if err := (&memWriter{v: v, path: "/ctx"}).Close(); !errors.Is(err, os.ErrExist) {
		t.Fatalf("writer at directory close = %v", err)
	}
}
