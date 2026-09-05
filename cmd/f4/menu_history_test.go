package main

import (
	"testing"

	"github.com/unxed/vtui"
)

func TestMenuHistory_ShiftF10SelectsLastExecutedItem(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	clearMenuHistory()
	t.Cleanup(clearMenuHistory)

	first := vtui.NewVMenu("&Files")
	first.AddItem(vtui.MenuItem{Text: "&View", UserData: menuHistoryItemKey("view")})
	first.AddItem(vtui.MenuItem{Text: "√ &Copy", UserData: menuHistoryItemKey("copy")})
	hookMenuHistory(first)
	recordMenuHistory(first, 1)

	second := vtui.NewVMenu("&Files")
	second.AddItem(vtui.MenuItem{Text: "&View", UserData: menuHistoryItemKey("view")})
	second.AddItem(vtui.MenuItem{Text: " &Copy", UserData: menuHistoryItemKey("copy")})
	vtui.FrameManager.Push(second)

	if !actionSelectLastMenuItem() {
		t.Fatal("Shift+F10 was not consumed")
	}
	if second.SelectPos != 1 {
		t.Fatalf("Shift+F10 selected item %d, want 1", second.SelectPos)
	}
}

func TestMenuHistory_ShiftF10OpensMainMenuAtLastExecutedItem(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	clearMenuHistory()
	t.Cleanup(clearMenuHistory)

	vtui.FrameManager.Push(vtui.NewDesktop())

	first := vtui.NewVMenu("&Commands")
	first.AddItem(vtui.MenuItem{Text: "&Open", UserData: menuHistoryItemKey("open")})
	first.AddItem(vtui.MenuItem{Text: "&Save", UserData: menuHistoryItemKey("save")})
	hookMenuHistory(first)
	recordMenuHistory(first, 1)

	menuBar := vtui.NewMenuBar([]string{"&Files", "&Commands"})
	menuBar.Items[0].SubItems = []vtui.MenuItem{{Text: "&Open"}}
	menuBar.Items[1].SubItems = []vtui.MenuItem{
		{Text: "&Open", UserData: menuHistoryItemKey("open")},
		{Text: "&Save", UserData: menuHistoryItemKey("save")},
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

	manager := NewHotkeyManager("")
	if got := manager.GetAction("Shell", "ShiftF10"); got != "App.LastMenuItem" {
		t.Fatalf("Shell ShiftF10 binding = %q, want App.LastMenuItem", got)
	}
}

func TestMenuHistory_ShiftF10DoesNotOverrideUserMenu(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	clearMenuHistory()
	t.Cleanup(clearMenuHistory)

	menu := vtui.NewVMenu("User menu")
	markUserMenu(menu)
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
