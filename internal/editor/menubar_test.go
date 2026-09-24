package editor

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtui"
)

func editorScreenRowText(scr *vtui.ScreenBuf, y, width int) string {
	var row strings.Builder
	for x := 0; x < width; x++ {
		row.WriteString(vtui.CellString(scr.GetCell(x, y).Char))
	}
	return row.String()
}

// TestEditorView_AlwaysShowMenuBarKeepsTheBarAboveTheTitle covers issue #1153:
// AlwaysShowMenuBar kept the menu bar only over the panels, so it vanished as
// soon as a file was opened in the editor. A pinned bar takes the workspace's
// top row, as it does over the panels, and the editor with its title bar
// starts one row lower instead of being covered by it.
func TestEditorView_AlwaysShowMenuBarKeepsTheBarAboveTheTitle(t *testing.T) {
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
	oldItems := MenuBarItems
	MenuBarItems = func(string) []vtui.MenuBarItem {
		return []vtui.MenuBarItem{{Label: "&Pinned"}}
	}
	t.Cleanup(func() { MenuBarItems = oldItems })

	ev := NewEditorView(piecetable.New([]byte("hello\n")), nil, "test.txt")
	t.Cleanup(ev.Close)

	config.App.AlwaysShowMenuBar = true
	ev.ResizeConsole(80, 25)
	if _, y1, _, _ := ev.GetPosition(); y1 != 1 {
		t.Fatalf("editor top row = %d, want 1 below the pinned menu bar", y1)
	}
	if _, menuY, _, _ := ev.GetMenuBar().GetPosition(); menuY != 0 {
		t.Fatalf("pinned menu bar row = %d, want 0", menuY)
	}
	ev.Show(scr)
	if row := editorScreenRowText(scr, 0, 80); !strings.Contains(row, "Pinned") {
		t.Errorf("row 0 = %q, want the pinned menu bar", row)
	}
	if row := editorScreenRowText(scr, 1, 80); !strings.Contains(row, "test.txt") {
		t.Errorf("row 1 = %q, want the title bar below the menu bar", row)
	}

	// Without the setting the editor starts on the top row again, and the bar
	// F9 raises sits over the title.
	config.App.AlwaysShowMenuBar = false
	ev.ResizeConsole(80, 25)
	if _, y1, _, _ := ev.GetPosition(); y1 != 0 {
		t.Fatalf("editor top row = %d, want 0 without AlwaysShowMenuBar", y1)
	}
	if _, menuY, _, _ := ev.GetMenuBar().GetPosition(); menuY != 0 {
		t.Fatalf("menu bar row = %d, want 0, the title row", menuY)
	}
	ev.Show(scr)
	if row := editorScreenRowText(scr, 0, 80); strings.Contains(row, "Pinned") || !strings.Contains(row, "test.txt") {
		t.Errorf("row 0 = %q, want the title bar and no menu bar", row)
	}
}
