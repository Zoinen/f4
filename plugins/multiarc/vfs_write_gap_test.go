package multiarc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// vfs_write_test.go's fakeWriter never fails to list, its tests never hand
// writableKey an already-canceled context, and Create never sees the
// refusals that its sibling MkDir's tests already exercise (root, and a
// parent that is a file) or a stageDirFor that cannot make its work
// directory. This file covers those setup failures, plus the one way
// stagedMember.Close itself can fail once the change has already been
// staged: its own file refusing to close.

var errMultiArcListFailed = errors.New("multiarc test: listing failed")

// writableKey (shared by MkDir, Remove and Create) starts by checking ctx
// and, on the first call, listing the archive -- neither failure is
// reachable through any test in vfs_write_test.go.
func TestMultiArcVFSWritableKeyContextCanceled(t *testing.T) {
	w := &fakeWriter{}
	v := newWritableTestVFS(t, w)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := v.MkDir(ctx, "/x"); !errors.Is(err, context.Canceled) {
		t.Errorf("MkDir with a canceled context = %v, want context.Canceled", err)
	}
	if len(w.adds) != 0 {
		t.Fatalf("a canceled MkDir still ran add: %#v", w.adds)
	}
}

func TestMultiArcVFSWritableKeyListingFailurePropagates(t *testing.T) {
	tests := []struct {
		name string
		run  func(v *MultiArcVFS) error
	}{
		{"MkDir", func(v *MultiArcVFS) error { return v.MkDir(context.Background(), "/x") }},
		{"Remove", func(v *MultiArcVFS) error { return v.Remove(context.Background(), "/x") }},
		{"Create", func(v *MultiArcVFS) error { _, err := v.Create(context.Background(), "/x"); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &fakeWriter{listErr: errMultiArcListFailed}
			v := newWritableTestVFS(t, w)
			if err := tt.run(v); !errors.Is(err, errMultiArcListFailed) {
				t.Errorf("%s = %v, want %v", tt.name, err, errMultiArcListFailed)
			}
		})
	}
}

// Create shares writableKey's root refusal and checkNewMember's
// file-ancestor refusal with MkDir, but only MkDir's own tests drive those
// two branches.
func TestMultiArcVFSCreateSharedRefusals(t *testing.T) {
	w := &fakeWriter{entries: []entry{{Path: "file.txt"}}}
	v := newWritableTestVFS(t, w)
	ctx := context.Background()

	if _, err := v.Create(ctx, "/"); err == nil {
		t.Error("Create on the archive root should be refused")
	}
	if _, err := v.Create(ctx, "/file.txt/sub"); err == nil || !strings.Contains(err.Error(), "is a file") {
		t.Errorf("Create under a file = %v, want a refusal", err)
	}
	if len(w.adds) != 0 {
		t.Fatalf("a refused Create still ran add: %#v", w.adds)
	}
}

// stageDirFor (workDirNextTo) needs to create a directory next to the
// archive; neither MkDir's nor Create's tests ever make that fail. Removing
// the archive's own directory after the VFS is built does, for both.
func TestMultiArcVFSStageDirForFailure(t *testing.T) {
	dir := t.TempDir()
	localPath := filepath.Join(dir, "test.fake")
	if err := os.WriteFile(localPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	w := &fakeWriter{entries: []entry{{Path: "top.txt"}}}
	v := NewMultiArcVFS(nil, localPath, "test.fake", w, "fakew")
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if err := v.MkDir(ctx, "/new"); err == nil {
		t.Error("MkDir should fail when its archive's directory is gone")
	}
	if _, err := v.Create(ctx, "/new.txt"); err == nil {
		t.Error("Create should fail when its archive's directory is gone")
	}
	if len(w.adds) != 0 {
		t.Fatalf("a failed stage directory still ran add: %#v", w.adds)
	}
}

// Close's own os.File.Close is the one failure between a successful stage
// and the commit that no existing test reaches: force it by closing the
// staged file out from under Close before calling it.
func TestMultiArcVFSCreateCloseFileErrorPropagates(t *testing.T) {
	w := &fakeWriter{}
	v := newWritableTestVFS(t, w)
	wc, err := v.Create(context.Background(), "/x")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	sm, ok := wc.(*stagedMember)
	if !ok {
		t.Fatalf("Create returned %T, want *stagedMember", wc)
	}
	if err := sm.file.Close(); err != nil {
		t.Fatalf("closing the staged file directly: %v", err)
	}

	if err := wc.Close(); err == nil {
		t.Error("Close should surface the staged file's own Close error")
	}
	if len(w.adds) != 0 {
		t.Fatalf("Close should not commit once its own file failed to close: %#v", w.adds)
	}
	assertNoScratchLeft(t, v.localPath)
}
