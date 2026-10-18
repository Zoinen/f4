package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/internal/action"
)

// The Explorer context menu opens for the item under the cursor, and has no
// key of its own until one is chosen for it (f4#2041).
func TestAction_FileExplorerMenu_OpensForTheCursorItem(t *testing.T) {
	a, ok := action.Lookup("File.ExplorerMenu")
	if !ok {
		t.Fatal("File.ExplorerMenu is not registered")
	}
	if len(a.DefaultKeys) != 0 {
		t.Fatalf("File.ExplorerMenu keys = %q, want none yet", a.DefaultKeys)
	}

	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	pf := seedPanelForCopyName(t, tmp)
	opened := make(chan string, 1)
	old := explorerMenuShower
	explorerMenuShower = func(path string) error {
		opened <- path
		return nil
	}
	t.Cleanup(func() { explorerMenuShower = old })

	fsp := pf.GetActivePanel()
	fsp.SelectName("a.txt")
	actionFileExplorerMenu(pf)

	abs, err := fsp.Vfs.Abs(tmp)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-opened:
		if want := fsp.Vfs.Join(abs, "a.txt"); got != want {
			t.Fatalf("menu opened for %q, want %q", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the menu was not opened")
	}
}
