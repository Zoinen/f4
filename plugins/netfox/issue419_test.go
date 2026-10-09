package netfox

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

// TestNetFoxRootRowsAreNotExecutable is the regression test for issue #419:
// the "<Add connection>" row and the saved connections were reported as
// executable files, so an "Executable" rule in highlight.ini painted the whole
// NetFox panel. They are virtual rows, not programs.
func TestNetFoxRootRowsAreNotExecutable(t *testing.T) {
	nf := NewNetFoxVFS(filepath.Join(t.TempDir(), "NetFox.json"))
	if err := nf.SaveConfig("My Server", NetFoxConfig{Type: "sftp", Host: "1.2.3.4", User: "root"}); err != nil {
		t.Fatal(err)
	}

	var items []vfs.VFSItem
	if err := nf.ReadDir(context.Background(), "net://", func(chunk []vfs.VFSItem) {
		items = append(items, chunk...)
	}); err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("root rows = %d, want \"<Add connection>\" and one saved connection", len(items))
	}
	for _, item := range items {
		if item.IsExecutable {
			t.Errorf("ReadDir row %q is marked executable", item.Name)
		}
		stat, err := nf.Stat(context.Background(), "net://"+item.Name)
		if err != nil {
			t.Fatalf("Stat(%q): %v", item.Name, err)
		}
		if stat.IsExecutable {
			t.Errorf("Stat row %q is marked executable", item.Name)
		}
	}
}
