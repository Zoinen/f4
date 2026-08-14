package vfs

import (
	"context"
	"testing"
)

func TestDisksVFS_Basics(t *testing.T) {
	v := NewDisksVFS()
	if v.GetPath() != "disks://" {
		t.Errorf("GetPath = %q, want disks://", v.GetPath())
	}
	if !v.IsAbs("disks://sda") {
		t.Error("IsAbs should be true")
	}

	var found []VFSItem
	err := v.ReadDir(context.Background(), "disks://", func(items []VFSItem) {
		found = append(found, items...)
	})
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
}
