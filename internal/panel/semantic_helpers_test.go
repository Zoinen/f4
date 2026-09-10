package panel

import (
	"context"
	"github.com/unxed/f4/vfs"
	"testing"
)

func readTempPanelItems(t *testing.T, tmp *TempPanelVFS) []vfs.VFSItem {
	t.Helper()
	var items []vfs.VFSItem
	if err := tmp.ReadDir(context.Background(), tmp.GetPath(), func(chunk []vfs.VFSItem) {
		items = append(items, chunk...)
	}); err != nil {
		t.Fatal(err)
	}
	return items
}
