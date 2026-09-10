package app

import (
	"github.com/unxed/f4/internal/history"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/vtui"
	"testing"
)

func TestMenuHistory_ShiftF10SelectsLastExecutedItem(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	history.ClearMenuHistory()
	t.Cleanup(history.ClearMenuHistory)

	first := vtui.NewVMenu("&Files")
	first.AddItem(vtui.MenuItem{Text: "&View", UserData: history.MenuHistoryItemKey("view")})
	first.AddItem(vtui.MenuItem{Text: "√ &Copy", UserData: history.MenuHistoryItemKey("copy")})
	history.HookMenuHistory(first)
	history.RecordMenuHistory(first, 1)

	second := vtui.NewVMenu("&Files")
	second.AddItem(vtui.MenuItem{Text: "&View", UserData: history.MenuHistoryItemKey("view")})
	second.AddItem(vtui.MenuItem{Text: " &Copy", UserData: history.MenuHistoryItemKey("copy")})
	vtui.FrameManager.Push(second)

	if !actionSelectLastMenuItem() {
		t.Fatal("Shift+F10 was not consumed")
	}
	if second.SelectPos != 1 {
		t.Fatalf("Shift+F10 selected item %d, want 1", second.SelectPos)
	}
}

func TestMenuHistory_ShiftF10OpensMainMenuAtLastExecutedItem(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	history.ClearMenuHistory()
	t.Cleanup(history.ClearMenuHistory)

	vtui.FrameManager.Push(vtui.NewDesktop())

	first := vtui.NewVMenu("&Commands")
	first.AddItem(vtui.MenuItem{Text: "&Open", UserData: history.MenuHistoryItemKey("open")})
	first.AddItem(vtui.MenuItem{Text: "&Save", UserData: history.MenuHistoryItemKey("save")})
	history.HookMenuHistory(first)
	history.RecordMenuHistory(first, 1)

	menuBar := vtui.NewMenuBar([]string{"&Files", "&Commands"})
	menuBar.Items[0].SubItems = []vtui.MenuItem{{Text: "&Open"}}
	menuBar.Items[1].SubItems = []vtui.MenuItem{
		{Text: "&Open", UserData: history.MenuHistoryItemKey("open")},
		{Text: "&Save", UserData: history.MenuHistoryItemKey("save")},
	}
	vtui.FrameManager.MenuBar = menuBar

	if !actionSelectLastMenuItem() {
		t.Fatal("Shift+F10 was not consumed")
	}
	menu, ok := vtui.FrameManager.GetTopFrame().(*vtui.VMenu)
	if !ok {
		t.Fatalf("top frame = %T, want main menu", vtui.FrameManager.GetTopFrame())
	}
	if menu.GetTitle() != "Commands" {
		t.Fatalf("main menu title = %q, want Commands", menu.GetTitle())
	}
	if menu.SelectPos != 1 {
		t.Fatalf("main menu selected item %d, want 1", menu.SelectPos)
	}
}

func TestLastMenuItemActionHasShiftF10Default(t *testing.T) {
	action, ok := GetAction("App.LastMenuItem")
	if !ok {
		t.Fatal("App.LastMenuItem is not registered")
	}
	if len(action.DefaultKeys) != 1 || action.DefaultKeys[0] != "ShiftF10" {
		t.Fatalf("App.LastMenuItem default keys = %v, want [ShiftF10]", action.DefaultKeys)
	}

	manager := keymap.NewHotkeyManager("")
	if got := manager.GetAction("Shell", "ShiftF10"); got != "App.LastMenuItem" {
		t.Fatalf("Shell ShiftF10 binding = %q, want term.App.LastMenuItem", got)
	}
}

func TestMenuHistory_ShiftF10DoesNotOverrideUserMenu(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	history.ClearMenuHistory()
	t.Cleanup(history.ClearMenuHistory)

	menu := vtui.NewVMenu("User menu")
	history.MarkUserMenu(menu)
	menu.AddItem(vtui.MenuItem{Text: "First"})
	menu.AddItem(vtui.MenuItem{Text: "Second"})
	menu.SetSelectPos(1)
	vtui.FrameManager.Push(menu)

	if actionSelectLastMenuItem() {
		t.Fatal("Shift+F10 unexpectedly intercepted user menu")
	}
	if menu.SelectPos != 1 {
		t.Fatalf("user menu selection changed to %d", menu.SelectPos)
	}
}
