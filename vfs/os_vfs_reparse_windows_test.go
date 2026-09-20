//go:build windows

package vfs

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/unxed/f4/vfs/hostmode"
)

// TestOSVFS_ReparseTagTellsJunctionFromSymlink checks that OSVFS reports the
// reparse tag a junction and a symlink really carry, so the panel can show
// Junction and Symlink the way Far3 does (#392).
func TestOSVFS_ReparseTagTellsJunctionFromSymlink(t *testing.T) {
	if hostmode.Posix() {
		t.Skip("the posix personality has no reparse points")
	}
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	junction := filepath.Join(root, "junction")
	// Creating a junction needs no privilege. cmd /s /c strips exactly the
	// outer quotes, so paths with spaces survive.
	mklink := exec.Command("cmd")
	mklink.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd /s /c "mklink /J "` + junction + `" "` + target + `""`}
	if out, err := mklink.CombinedOutput(); err != nil {
		t.Fatalf("mklink /J: %v: %s", err, out)
	}
	symlink := filepath.Join(root, "symlink")
	symlinkErr := os.Symlink(target, symlink)

	v := NewOSVFS(root)
	items := make(map[string]VFSItem)
	if err := v.ReadDir(context.Background(), root, func(chunk []VFSItem) {
		for _, it := range chunk {
			items[it.Name] = it
		}
	}); err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	check := func(what string, it VFSItem, tag uint32, kind LinkKind) {
		t.Helper()
		if it.ReparseTag != tag || LinkKindOf(&it) != kind {
			t.Errorf("%s: ReparseTag=%#x kind=%d, want %#x kind=%d (item %+v)", what, it.ReparseTag, LinkKindOf(&it), tag, kind, it)
		}
	}
	check("ReadDir target", items["target"], 0, LinkNone)
	check("ReadDir junction", items["junction"], ReparseTagMountPoint, LinkJunction)

	if it, err := v.Lstat(context.Background(), junction); err != nil {
		t.Errorf("Lstat junction: %v", err)
	} else {
		check("Lstat junction", it, ReparseTagMountPoint, LinkJunction)
	}
	if it, err := v.Stat(context.Background(), junction); err != nil {
		t.Errorf("Stat junction: %v", err)
	} else {
		check("Stat junction", it, ReparseTagMountPoint, LinkJunction)
	}

	if symlinkErr != nil {
		t.Logf("symlink part skipped, creation unavailable: %v", symlinkErr)
		return
	}
	check("ReadDir symlink", items["symlink"], ReparseTagSymlink, LinkSymlink)
}
