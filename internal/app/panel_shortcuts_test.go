package app

import (
	keymap "github.com/unxed/f4/internal/keymap"
	macro "github.com/unxed/f4/internal/macro"
	"github.com/unxed/f4/internal/panel"
	vtinput "github.com/unxed/vtinput"
	vtui "github.com/unxed/vtui"
	testing "testing"
)

func TestPanelsFrame_CtrlViewModes(t *testing.T) {
	vtui.SetDefaultPalette()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)

	pf := panel.NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	vtui.FrameManager.Push(pf)

	fsp := pf.Panels[pf.ActiveIdx].(*panel.FileSystemPanel)

	// 1. Изначально устанавливаем режим Medium
	fsp.SetViewMode(panel.ViewModeMedium)
	oldHotkeys := keymap.GlobalHotkeysMgr
	keymap.GlobalHotkeysMgr = keymap.NewHotkeyManager("")
	defer func() { keymap.GlobalHotkeysMgr = oldHotkeys }()
	oldMacroMgr := macro.MacroMgr
	macro.MacroMgr = &macro.MacroManager{Macros: make(map[string]map[string][]*vtinput.InputEvent)}
	defer func() { macro.MacroMgr = oldMacroMgr }()
	rightCtrl3 := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: '3', ControlKeyState: vtinput.RightCtrlPressed}
	if macroFilter(macro.MacroMgr, rightCtrl3) {
		t.Fatal("RightCtrl+3 was consumed by the configurable hotkey filter")
	}
	if !pressKey(pf, rightCtrl3) {
		t.Fatal("RightCtrl+3 was not handled by bookmarks")
	}
	if fsp.ViewMode != panel.ViewModeMedium {
		t.Fatalf("RightCtrl+3 changed panel mode to %v", fsp.ViewMode)
	}

	for _, tc := range []struct {
		key  uint16
		mode panel.ViewMode
	}{{'1', panel.ViewModeMedium}, {'2', panel.ViewModeBrief}, {'3', panel.ViewModeDetailed}} {
		pressKey(pf, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: tc.key, ControlKeyState: vtinput.LeftCtrlPressed})
		if fsp.ViewMode != tc.mode || pf.WidePanel != -1 {
			t.Errorf("Ctrl+%c: mode=%v wide=%d, want mode=%v wide=-1", tc.key, fsp.ViewMode, pf.WidePanel, tc.mode)
		}
	}

	pressKey(pf, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: '4', ControlKeyState: vtinput.LeftCtrlPressed})
	if pf.WidePanel != pf.ActiveIdx || !fsp.Wide || len(fsp.Table.Columns) != 3 {
		t.Fatalf("Ctrl+4 did not enter Wide: wide=%d active=%d columns=%d", pf.WidePanel, pf.ActiveIdx, len(fsp.Table.Columns))
	}
	x1, _, x2, _ := fsp.GetPosition()
	if x1 != 0 || x2 != 79 {
		t.Fatalf("Wide geometry = %d..%d, want 0..79", x1, x2)
	}
	originalMode := fsp.ViewMode
	pressKey(pf, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if pf.WidePanel != pf.ActiveIdx || pf.ActiveIdx != 0 {
		t.Fatalf("Tab did not transfer Wide to left panel: wide=%d active=%d", pf.WidePanel, pf.ActiveIdx)
	}
	if fsp.ViewMode != originalMode {
		t.Error("Wide changed the right panel's normal view mode")
	}
	pressKey(pf, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: '2', ControlKeyState: vtinput.LeftCtrlPressed})
	if pf.WidePanel != pf.ActiveIdx || pf.Panels[0].(*panel.FileSystemPanel).ViewMode != panel.ViewModeBrief {
		t.Error("Ctrl+2 did not change renderer while retaining independent Wide")
	}
	pressKey(pf, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: '4', ControlKeyState: vtinput.LeftCtrlPressed})
	if pf.WidePanel != -1 || pf.Wide {
		t.Fatal("second Ctrl+4 did not toggle Wide off")
	}

}
