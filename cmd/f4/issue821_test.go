package main

import (
	"strings"
	"testing"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestIssue821CommandHistoryEnterPastesSelectedEntry(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	SetDefaultF4Palette()

	previousHistory := vtui.GlobalHistoryProvider
	vtui.GlobalHistoryProvider = stubHistoryProvider{}
	t.Cleanup(func() { vtui.GlobalHistoryProvider = previousHistory })

	pf := setupMockPanelsFrame(t)
	t.Cleanup(pf.Close)
	pf.ResizeConsole(80, 25)
	pf.cmdLine.Edit.History = []string{"pbrush.exe", "selected-command", "older-command"}

	actionCommandHistory(pf)
	menu, ok := vtui.FrameManager.GetTopFrame().(*vtui.VMenu)
	if !ok {
		t.Fatalf("top frame = %T, want *vtui.VMenu", vtui.FrameManager.GetTopFrame())
	}

	// The dialog displays the oldest record first, so select the middle row.
	menu.SetSelectPos(1)
	menu.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_RETURN,
	})

	if got := pf.cmdLine.Edit.GetText(); got != "selected-command" {
		t.Fatalf("selected history entry pasted %q, want %q", got, "selected-command")
	}
}

func TestIssue821CommandHistoryKeepsLongEntryInsideDialog(t *testing.T) {
	const (
		screenWidth  = 100
		screenHeight = 20
	)
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(screenWidth, screenHeight)
	vtui.FrameManager.Init(scr)
	SetDefaultF4Palette()

	menu := vtui.NewVMenu("History")
	search := newHistorySearch(menu, []HistoryRecord{{Name: strings.Repeat("x", 200)}}, "")
	t.Cleanup(search.cleanup)
	const sentinel = '~'
	scr.FillRect(0, 0, screenWidth-1, screenHeight-1, sentinel, 0)
	menu.Show(scr)
	search.draw(scr)

	for x := 0; x < screenWidth; x++ {
		if x >= menu.X1 && x <= menu.X2 {
			continue
		}
		if got := testRune(scr.GetCell(x, menu.Y1+1).Char); got != sentinel {
			t.Fatalf("long history entry escaped dialog at column %d as %q", x, got)
		}
	}
}
