//go:build !windows

package vfs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func zeroModeFile(t *testing.T) (*OSVFS, string, VFSItem) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, nil, 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	v := NewOSVFS(dir)
	item, err := v.Stat(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return v, path, item
}

func modeOf(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

// SetUnixMode writes a mode of 000: removing the last permission of a file
// (-r-------- to ----------) used to be skipped as "no mode" (f4#1981).
func TestOSVFSSetAttributes_SetUnixModeAppliesZero(t *testing.T) {
	v, path, item := zeroModeFile(t)
	item.UnixMode, item.SetUnixMode = 0, true
	if err := v.SetAttributes(context.Background(), path, item); err != nil {
		t.Fatal(err)
	}
	if perm := modeOf(t, path); perm != 0 {
		t.Fatalf("mode = %o, want 000", perm)
	}
}

// Without SetUnixMode a zero mode still leaves the file's mode alone: the copy
// passes items that way and expects the destination to keep its rights.
func TestOSVFSSetAttributes_ZeroModeWithoutTheFlagIsKept(t *testing.T) {
	v, path, item := zeroModeFile(t)
	item.UnixMode = 0
	if err := v.SetAttributes(context.Background(), path, item); err != nil {
		t.Fatal(err)
	}
	if perm := modeOf(t, path); perm != 0o400 {
		t.Fatalf("mode = %o, want the file's own 400", perm)
	}
}
