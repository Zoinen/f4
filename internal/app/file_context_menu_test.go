package app

import (
	"reflect"
	"testing"

	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestFileContextMenuRegistrationAndKeyScope(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := panel.NewPanelsFrame()
	defer pf.Close()
	pf.ShowPanels = true
	pf.ResizeConsole(80, 25)
	vtui.FrameManager.Push(pf)
	a, ok := GetAction("File.ContextMenu")
	if !ok || a.MenuPath != "Files" || !reflect.DeepEqual(a.DefaultKeys, []string{"Apps:FilePanel"}) {
		t.Fatalf("registration=%+v", a)
	}
	e := keymap.ParseFarKey("Apps")
	if e == nil || e.VirtualKeyCode != vtinput.VK_APPS {
		t.Fatalf("Apps key=%+v", e)
	}
	mgr := keymap.NewHotkeyManager("")
	if got := mgr.GetAction("Shell", "Apps"); got != "File.ContextMenu" {
		t.Fatalf("Apps action=%q binding=%q top=%T closed=%v shown=%v panel=%p condition=%v", got, mgr.Bindings["Shell"]["Apps"], vtui.FrameManager.GetTopFrame(), pf.Closed, pf.ShowPanels, pf.GetActivePanel(), keymap.ConditionTrue("FilePanel"))
	}
	for _, area := range []string{"Editor", "Dialog", "Viewer", "Terminal"} {
		if got := mgr.GetAction(area, "Apps"); got != "" {
			t.Errorf("Apps captured in %s: %s", area, got)
		}
	}
	pf.ShowPanels = false
	if got := mgr.GetAction("Shell", "Apps"); got != "" {
		t.Fatalf("Apps captured with hidden panels: %s", got)
	}
	pf.ShowPanels = true
	mgr.Bindings["Shell"] = map[string]string{"Apps": "None"}
	if got := mgr.GetAction("Shell", "Apps"); got == "File.ContextMenu" {
		t.Fatal("Apps override ignored")
	}
}

func TestFileContextMenuPaletteDiscovery(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	for _, entry := range buildCommandPaletteEntries("Shell", nil) {
		if entry.ID == "File.ContextMenu" {
			return
		}
	}
	t.Fatal("context menu missing from command palette")
}
