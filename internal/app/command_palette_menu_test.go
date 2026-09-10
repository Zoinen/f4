package app

import (
	"github.com/unxed/vtui"
	"testing"
	"time"
)

func TestCommandPaletteActionFromActiveMenuWaitsForMenuClose(t *testing.T) {
	initFrameworkActionTestScreen(t)

	menu := vtui.NewMenuBar([]string{"&Commands"})
	host := &frameworkActionTestFrame{title: "Panels", menu: menu}
	vtui.FrameManager.Push(host)
	menu.Items[0].SubItems = []vtui.MenuItem{{
		Text:    "&Command palette",
		OnClick: func() { RunAction(CommandPaletteActionName) },
	}}
	menu.Active = true
	menu.ActivateSubMenu(0)

	if vtui.FrameManager.GetTopFrame().GetType() != vtui.TypeMenu {
		t.Fatalf("top frame before click = %T, want VMenu", vtui.FrameManager.GetTopFrame())
	}
	if !RunAction(CommandPaletteActionName) {
		t.Fatal("command palette action was not handled from the active menu")
	}

	// This is the ordering used by FrameManager after VMenu.OnClick returns:
	// the menu is marked done and the menu bar is inactive before the posted
	// task is allowed to open the replacement dialog.
	menu.Active = false
	vtui.FrameManager.GetTopFrame().Close()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		vtui.FrameManager.Step(0)
		if _, ok := vtui.FrameManager.GetTopFrame().(*CommandPaletteDialog); ok {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("top frame after deferred menu action = %T, want commandPaletteDialog", vtui.FrameManager.GetTopFrame())
}
