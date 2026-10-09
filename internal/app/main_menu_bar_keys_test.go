package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/macro"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/viewer"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// initMenuKeyRoute sets up the route a key takes in the running program:
// FrameManager's input channel, not InjectEvents, so that f4's EventFilter
// (macroFilter, which dispatches the configured hotkeys) sees it before
// vtui's menu interception and the frames do.
func initMenuKeyRoute(t *testing.T) {
	t.Helper()
	initFrameworkActionTestScreen(t)
	oldHotkeys, oldMacros, oldEscToggle := keymap.GlobalHotkeysMgr, macro.MacroMgr, config.App.EscTogglePanels
	t.Cleanup(func() {
		keymap.GlobalHotkeysMgr, macro.MacroMgr, config.App.EscTogglePanels = oldHotkeys, oldMacros, oldEscToggle
	})
	keymap.GlobalHotkeysMgr = keymap.NewHotkeyManager("")
	macro.MacroMgr = macro.NewMacroManager("")
	vtui.FrameManager.EventChan = make(chan *vtinput.InputEvent, 4)
	vtui.FrameManager.EventFilter = func(e *vtinput.InputEvent) bool {
		return macroFilter(macro.MacroMgr, e)
	}
}

func pressMenuRouteKey(t *testing.T, vk uint16, char rune) {
	t.Helper()
	for _, down := range []bool{true, false} {
		vtui.FrameManager.EventChan <- &vtinput.InputEvent{
			Type: vtinput.KeyEventType, KeyDown: down, VirtualKeyCode: vk, Char: char,
		}
		for i := 0; len(vtui.FrameManager.EventChan) > 0; i++ {
			if i == 100 {
				t.Fatalf("key 0x%X was never dispatched", vk)
			}
			vtui.FrameManager.Step(0)
		}
	}
}

// TestRaisedMenuBarOwnsKeysOverThePanels covers issue #1144. F9 over the
// panels raises the bar without a dropdown, so the panels stay the top frame,
// and their Shell bindings used to run before the bar could see the key: with
// "Escape toggles panels" on, Esc hid the panels and left the bar up, and F10
// asked to quit instead of closing the menu.
func TestRaisedMenuBarOwnsKeysOverThePanels(t *testing.T) {
	cases := []struct {
		name      string
		vk        uint16
		char      rune
		barClosed bool
	}{
		{"Esc", vtinput.VK_ESCAPE, 27, true},
		{"F10", vtinput.VK_F10, 0, true},
		// far2l's HMenu ignores Del; it must not reach Panel.Toggle either.
		{"Del", vtinput.VK_DELETE, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			initMenuKeyRoute(t)
			config.App.EscTogglePanels = true
			pf := panel.NewPanelsFrame()
			t.Cleanup(pf.Close)
			pf.ResizeConsole(100, 30)
			vtui.FrameManager.Push(pf)
			menu := vtui.FrameManager.GetActiveMenuBar()

			pressMenuRouteKey(t, vtinput.VK_F9, 0)
			if !menu.Active || vtui.FrameManager.GetTopFrame() != vtui.Frame(pf) {
				t.Fatalf("F9 left active=%v top=%T, want the bar raised over the panels", menu.Active, vtui.FrameManager.GetTopFrame())
			}

			pressMenuRouteKey(t, tc.vk, tc.char)
			if menu.Active == tc.barClosed {
				t.Errorf("after %s the menu bar active = %v, want %v", tc.name, menu.Active, !tc.barClosed)
			}
			if !pf.ShowPanels {
				t.Errorf("%s hid the panels while the menu bar held the keyboard", tc.name)
			}
			if top := vtui.FrameManager.GetTopFrame(); top != vtui.Frame(pf) {
				t.Errorf("after %s the top frame is %T, want the panels", tc.name, top)
			}
		})
	}
}

// TestF9DropsFileDownAfterAnotherMenuWasOpen covers the other half of #1144.
// F9 in the editor and the viewer is vtui's native fallback, which drops down
// the bar's SelectPos. far2l builds these menus afresh with File selected, so
// a menu the user opened and closed before must not come back down.
func TestF9DropsFileDownAfterAnotherMenuWasOpen(t *testing.T) {
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
			initMenuKeyRoute(t)
			frame := tc.frame(t)
			frame.ResizeConsole(100, 30)
			vtui.FrameManager.Push(frame)
			menu := frame.GetMenuBar()
			if len(menu.Items) < 3 {
				t.Fatalf("%s menu has %d top-level items, want at least 3", tc.name, len(menu.Items))
			}

			pressMenuRouteKey(t, vtinput.VK_F9, 0)
			pressMenuRouteKey(t, vtinput.VK_RIGHT, 0)
			pressMenuRouteKey(t, vtinput.VK_RIGHT, 0)
			if menu.SelectPos != 2 {
				t.Fatalf("Right, Right moved the %s menu to %d, want 2", tc.name, menu.SelectPos)
			}
			pressMenuRouteKey(t, vtinput.VK_ESCAPE, 27)
			if menu.Active {
				t.Fatalf("Esc left the %s menu bar active", tc.name)
			}

			pressMenuRouteKey(t, vtinput.VK_F9, 0)
			if !menu.Active || menu.SelectPos != 0 {
				t.Errorf("second F9: %s menu active=%v position=%d, want active at 0, File", tc.name, menu.Active, menu.SelectPos)
			}
			if top := vtui.FrameManager.GetTopFrame(); top == nil || top.GetType() != vtui.TypeMenu {
				t.Errorf("second F9: top frame = %T, want the File dropdown", top)
			}
		})
	}
}
