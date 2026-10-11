package fileops

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// deviceAwareCountingVFS wraps a real OSVFS and counts Rename calls, while
// still answering vfs.FileIdentifier itself (embedding vfs.VFS as an
// interface field only promotes vfs.VFS's own methods, not extra optional
// capability methods like FileIdentity, so this forwards it explicitly). It
// lets a test prove that a move used a single native rename rather than the
// scan+copy+delete path, on real device ids.
type deviceAwareCountingVFS struct {
	vfs.VFS
	renameCalls int
}

func (p *deviceAwareCountingVFS) Rename(ctx context.Context, oldPath, newPath string) error {
	p.renameCalls++
	return p.VFS.Rename(ctx, oldPath, newPath)
}

func (p *deviceAwareCountingVFS) FileIdentity(ctx context.Context, path string) (device, inode uint64, ok bool) {
	if idf, ok := p.VFS.(vfs.FileIdentifier); ok {
		return idf.FileIdentity(ctx, path)
	}
	return 0, 0, false
}

// nullFileIdentifierVFS wraps a real VFS but, as a distinct concrete type
// with no FileIdentity method of its own, never satisfies
// vfs.FileIdentifier — simulating a VFS backend (a remote filesystem, or a
// platform without device ids) that cannot answer "same device".
type nullFileIdentifierVFS struct {
	vfs.VFS
}

func TestSameDeviceForMove(t *testing.T) {
	root := t.TempDir()
	aPath, bPath := filepath.Join(root, "a"), filepath.Join(root, "b")
	if err := os.MkdirAll(aPath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bPath, 0700); err != nil {
		t.Fatal(err)
	}
	a, b := vfs.NewOSVFS(aPath), vfs.NewOSVFS(bPath)

	if _, _, ok := a.FileIdentity(context.Background(), aPath); !ok {
		t.Skip("this platform's OSVFS does not implement FileIdentity; nothing to test")
	}

	if !sameDeviceForMove(context.Background(), a, aPath, b, bPath) {
		t.Error("two directories under the same temp root should report the same device")
	}

	// A destination lacking FileIdentifier (a remote VFS, or a platform
	// stub) must never be treated as same-device: sameDeviceForMove has to
	// return false, not panic or guess.
	blind := nullFileIdentifierVFS{b}
	if sameDeviceForMove(context.Background(), a, aPath, blind, bPath) {
		t.Error("a destination without FileIdentifier must not be reported as same-device")
	}
	if sameDeviceForMove(context.Background(), blind, aPath, b, bPath) {
		t.Error("a source without FileIdentifier must not be reported as same-device")
	}

	// A path that does not exist cannot be stat'd, so identity is unknown
	// rather than guessed.
	if sameDeviceForMove(context.Background(), a, filepath.Join(aPath, "missing"), b, bPath) {
		t.Error("a source Stat failure must not be reported as same-device")
	}
}

func TestExecuteFileOp_MoveSameDeviceUsesOptimizedRename(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	// A single temp root guarantees tmpSrc and tmpDst share one real
	// filesystem device, unlike TestExecuteFileOp_MoveAcrossVFS_Fallback's
	// two independent t.TempDir() calls.
	root := t.TempDir()
	tmpSrc := filepath.Join(root, "left")
	tmpDst := filepath.Join(root, "right")
	if err := os.MkdirAll(tmpSrc, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(tmpDst, 0700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(tmpSrc, "file.txt"), []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	// A directory with nested content, to prove the whole subtree moves in
	// one rename rather than being scanned and copied file by file.
	nestedDir := filepath.Join(tmpSrc, "subdir")
	if err := os.MkdirAll(filepath.Join(nestedDir, "inner"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "nested.txt"), []byte("nested"), 0600); err != nil {
		t.Fatal(err)
	}

	srcOS := vfs.NewOSVFS(tmpSrc)
	if _, _, ok := srcOS.FileIdentity(context.Background(), tmpSrc); !ok {
		t.Skip("this platform's OSVFS does not implement FileIdentity; the optimized same-device move path is unreachable here")
	}

	srcVfs := &deviceAwareCountingVFS{VFS: srcOS}
	dstVfs := &deviceAwareCountingVFS{VFS: vfs.NewOSVFS(tmpDst)}

	done := make(chan struct{})
	ExecuteFileOp(srcVfs, dstVfs, []string{"file.txt", "subdir"}, tmpDst, true, 2, func() { close(done) })

	timeout := time.After(10 * time.Second)
Loop:
	for {
		select {
		case <-done:
			break Loop
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("Move operation timed out")
		}
	}

	if data, err := os.ReadFile(filepath.Join(tmpDst, "file.txt")); err != nil || string(data) != "payload" {
		t.Fatalf("file.txt not moved correctly: data=%q err=%v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(tmpDst, "subdir", "nested.txt")); err != nil || string(data) != "nested" {
		t.Fatalf("subdir/nested.txt not moved correctly: data=%q err=%v", data, err)
	}
	if _, err := os.Stat(filepath.Join(tmpDst, "subdir", "inner")); err != nil {
		t.Fatalf("subdir/inner did not survive the move: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpSrc, "file.txt")); !os.IsNotExist(err) {
		t.Error("source file.txt still present after move")
	}
	if _, err := os.Stat(filepath.Join(tmpSrc, "subdir")); !os.IsNotExist(err) {
		t.Error("source subdir still present after move")
	}

	// Exactly 2 renames: one whole-file rename and one whole-directory
	// rename. Any more would mean subdir's contents were walked and moved
	// file by file instead of the whole tree moving in a single syscall.
	if srcVfs.renameCalls != 2 {
		t.Errorf("renameCalls = %d, want exactly 2 (one whole-file rename, one whole-directory rename, no per-file recursion)", srcVfs.renameCalls)
	}
}
