package fileops_test

import (
	context "context"
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/panel"
	vfs "github.com/unxed/f4/vfs"
	testing "testing"

	filepath "path/filepath"

	os "os"
)

func TestTemporaryPanelCannotOverwriteItsReferencedSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "original.txt")
	if err := os.WriteFile(path, []byte("must survive"), 0600); err != nil {
		t.Fatal(err)
	}
	local := vfs.NewOSVFS(dir)
	tmp := panel.NewTempPanelVFS(nil, &panel.TempPanelStore{}, 0)
	if err := tmp.AddReferences(context.Background(), local, []string{"original.txt"}); err != nil {
		t.Fatal(err)
	}
	name := readTempPanelItems(t, tmp)[0].Name
	err := fileops.RecursiveCopyForTest(context.Background(), tmp, tmp.Join(tmp.GetPath(), name), local, path, &fileops.FileOpState{OverwriteAll: true}, 0)
	data, readErr := os.ReadFile(path)
	if err == nil || readErr != nil || string(data) != "must survive" {
		t.Fatalf("self-copy err=%v data=%q readErr=%v", err, data, readErr)
	}
}

func readTempPanelItems(t *testing.T, tmp *panel.TempPanelVFS) []vfs.VFSItem {
	t.Helper()
	var items []vfs.VFSItem
	if err := tmp.ReadDir(context.Background(), tmp.GetPath(), func(chunk []vfs.VFSItem) {
		items = append(items, chunk...)
	}); err != nil {
		t.Fatal(err)
	}
	return items
}
