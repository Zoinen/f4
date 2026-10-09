package viewer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func viewerScreenRowText(scr *vtui.ScreenBuf, y, width int) string {
	var row strings.Builder
	for x := 0; x < width; x++ {
		row.WriteString(vtui.CellString(scr.GetCell(x, y).Char))
	}
	return row.String()
}

// TestViewerView_AlwaysShowMenuBarKeepsTheBarAboveTheTitle covers issue #1153:
// AlwaysShowMenuBar kept the menu bar only over the panels, so it vanished as
// soon as a file was opened in the viewer. A pinned bar takes the workspace's
// top row, as it does over the panels, and the viewer with its title bar
// starts one row lower instead of being covered by it.
func TestViewerView_AlwaysShowMenuBarKeepsTheBarAboveTheTitle(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	theme.SetDefaultF4Palette()
	oldAlways, oldFullPath := config.App.AlwaysShowMenuBar, config.App.DisplayFullPathInTitle
	t.Cleanup(func() {
		config.App.AlwaysShowMenuBar = oldAlways
		config.App.DisplayFullPathInTitle = oldFullPath
	})
	config.App.DisplayFullPathInTitle = false

	root := t.TempDir()
	path := filepath.Join(root, "pinned.txt")
	if err := os.WriteFile(path, []byte("text\n"), 0600); err != nil {
		t.Fatal(err)
	}
	vv, err := NewViewerView(context.Background(), vfs.NewOSVFS(root), path)
	if err != nil {
		t.Fatalf("NewViewerView: %v", err)
	}
	defer vv.Close()
	// GetMenuBar only rebuilds the items through App, which tests leave unset.
	vv.menuBar.Items = []vtui.MenuBarItem{{Label: "&Pinned"}}

	config.App.AlwaysShowMenuBar = true
	vv.ResizeConsole(80, 25)
	if _, y1, _, _ := vv.GetPosition(); y1 != 1 {
		t.Fatalf("viewer top row = %d, want 1 below the pinned menu bar", y1)
	}
	if _, menuY, _, _ := vv.GetMenuBar().GetPosition(); menuY != 0 {
		t.Fatalf("pinned menu bar row = %d, want 0", menuY)
	}
	vv.SetVisible(true)
	vv.Show(scr)
	if row := viewerScreenRowText(scr, 0, 80); !strings.Contains(row, "Pinned") {
		t.Errorf("row 0 = %q, want the pinned menu bar", row)
	}
	if row := viewerScreenRowText(scr, 1, 80); !strings.Contains(row, "pinned.txt") {
		t.Errorf("row 1 = %q, want the title bar below the menu bar", row)
	}

	// Without the setting the viewer starts on the top row again, and the bar
	// F9 raises sits over the title.
	config.App.AlwaysShowMenuBar = false
	vv.ResizeConsole(80, 25)
	if _, y1, _, _ := vv.GetPosition(); y1 != 0 {
		t.Fatalf("viewer top row = %d, want 0 without AlwaysShowMenuBar", y1)
	}
	if _, menuY, _, _ := vv.GetMenuBar().GetPosition(); menuY != 0 {
		t.Fatalf("menu bar row = %d, want 0, the title row", menuY)
	}
	vv.Show(scr)
	if row := viewerScreenRowText(scr, 0, 80); strings.Contains(row, "Pinned") || !strings.Contains(row, "pinned.txt") {
		t.Errorf("row 0 = %q, want the title bar and no menu bar", row)
	}
}
