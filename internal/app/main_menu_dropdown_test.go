package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/viewer"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// TestMainMenuActionDropsTheEditorAndViewerFileMenuDown covers issue #1149.
// #1144 made F9 raise the menu bar alone, which is far2l's behavior over the
// panels (ShellOptions(0)), but it did so for every screen. far2l's editor and
// viewer open their menu with the File dropdown already down
// (EditorShellOptions, ViewerShellOptions: Show() then ProcessKey(KEY_DOWN)
// from the first item), so F9 there has to do the same.
func TestMainMenuActionDropsTheEditorAndViewerFileMenuDown(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "menu.txt")
	if err := os.WriteFile(path, []byte("text\n"), 0600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		frame func(t *testing.T) vtui.Frame
	}{
		{"editor", func(t *testing.T) vtui.Frame {
			ev := editor.NewEditorView(piecetable.New([]byte("text\n")), nil, path)
			t.Cleanup(ev.Close)
			return ev
		}},
		{"viewer", func(t *testing.T) vtui.Frame {
			vv, err := viewer.NewViewerView(context.Background(), vfs.NewOSVFS(root), path)
			if err != nil {
				t.Fatalf("NewViewerView: %v", err)
			}
			t.Cleanup(vv.Close)
			return vv
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			initFrameworkActionTestScreen(t)
			old := keymap.GlobalHotkeysMgr
			keymap.GlobalHotkeysMgr = keymap.NewHotkeyManager("")
			t.Cleanup(func() { keymap.GlobalHotkeysMgr = old })

			frame := tc.frame(t)
			frame.ResizeConsole(100, 30)
			vtui.FrameManager.Push(frame)
			menu := frame.GetMenuBar()
			if len(menu.Items) < 2 {
				t.Fatalf("%s menu has %d top-level items, want at least 2", tc.name, len(menu.Items))
			}
			// A menu visited earlier does not decide where F9 opens.
			menu.SelectPos = 1

			if !actionActivateMainMenu() || !menu.Active {
				t.Fatalf("main menu action did not activate the %s menu", tc.name)
			}
			if menu.SelectPos != 0 {
				t.Errorf("%s menu position = %d, want 0, the File menu", tc.name, menu.SelectPos)
			}
			dropdown := vtui.FrameManager.GetTopFrame()
			if dropdown == nil || dropdown.GetType() != vtui.TypeMenu {
				t.Fatalf("top frame = %T, want the %s's File dropdown", dropdown, tc.name)
			}
		})
	}
}
