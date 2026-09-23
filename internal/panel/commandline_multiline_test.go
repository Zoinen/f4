package panel

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"strings"
	"testing"
)

func TestMultilineCommandArrowsStayInTextUntilBoundary(t *testing.T) {
	old := config.App
	t.Cleanup(func() { config.App = old })
	config.App.CommandLineMultiline = true
	config.App.CommandLineWordWrap = false
	config.App.NavigationMode = config.NavigationSearchFirst
	pf, _ := panelsFrameWithMouseSelect(t)
	pf.ShowPanels = true
	pf.CmdLine.SetVisible(true)
	pf.SetCommandLineFocus(true)
	pf.ResizeConsole(80, 25)
	pf.CmdLine.Edit.History = []string{"previous command"}
	pf.CmdLine.Edit.SetText("abc\nx\nabcdef")
	pf.CmdLine.Edit.HandleSemanticAction(map[string]any{
		"action": "control.select", "anchor": 9, "cursor": 9,
	})
	for _, want := range []int{5, 3} {
		pressKey(pf, &vtinput.InputEvent{
			Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP,
		})
		if got := pf.CmdLine.Edit.GetText(); got != "abc\nx\nabcdef" {
			t.Fatalf("interior Up switched history: %q", got)
		}
		if got := pf.CmdLine.Edit.SemanticNode(nil)["cursor"]; got != want {
			t.Fatalf("cursor = %v, want %d", got, want)
		}
	}
	pressKey(pf, &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP,
	})
	if got := pf.CmdLine.Edit.GetText(); got != "abc\nx\nabcdef" {
		t.Fatalf("top Up unexpectedly recalled history: %q", got)
	}
}

func TestSearchFirstCommandOwnsSelectionAndPageNavigation(t *testing.T) {
	old := config.App
	t.Cleanup(func() { config.App = old })
	config.App.CommandLineMultiline = true
	config.App.NavigationMode = config.NavigationSearchFirst
	pf, _ := panelsFrameWithMouseSelect(t)
	pf.ShowPanels = true
	pf.CmdLine.SetVisible(true)
	pf.SetCommandLineFocus(true)
	pf.ResizeConsole(80, 25)
	pf.CmdLine.Edit.SetText("first\nsecond\nthird")
	pf.CmdLine.Edit.HandleSemanticAction(map[string]any{"action": "control.select", "anchor": 8, "cursor": 8})
	for _, vk := range []uint16{vtinput.VK_LEFT, vtinput.VK_RIGHT, vtinput.VK_HOME, vtinput.VK_END, vtinput.VK_PRIOR, vtinput.VK_NEXT} {
		e := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk, ControlKeyState: vtinput.ShiftPressed}
		if !pf.VetoActionKey(e) {
			t.Fatalf("Shift+%x escaped command line to keymap", vk)
		}
		pressKey(pf, e)
		if pf.CmdLine.Edit.GetText() != "first\nsecond\nthird" {
			t.Fatalf("Shift+%x modified command", vk)
		}
	}
	if !pf.commandLineOwnsNavigation(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_PRIOR}) {
		t.Fatal("PageUp belongs to focused command line")
	}
}

func TestMultilineCommandConsoleMouseSelection(t *testing.T) {
	old := config.App
	t.Cleanup(func() { config.App = old })
	config.App.CommandLineMultiline = true
	config.App.CommandLineWordWrap = false
	pf, _ := panelsFrameWithMouseSelect(t)
	pf.ShowPanels = true
	pf.CmdLine.SetVisible(true)
	pf.ResizeConsole(80, 25)
	pf.CmdLine.Edit.SetText("first\nsecond word\nthird")
	pf.ResizeConsole(80, 25)
	x, y := pf.CmdLine.Edit.X1, pf.CmdLine.Edit.Y1
	press := func(px, py int, flags uint32) {
		t.Helper()
		pf.ProcessMouse(&vtinput.InputEvent{
			Type: vtinput.MouseEventType, KeyDown: true,
			ButtonState: vtinput.FromLeft1stButtonPressed,
			MouseX:      int16(px), MouseY: int16(py), MouseEventFlags: flags,
		})
	}
	release := func(px, py int) {
		t.Helper()
		pf.ProcessMouse(&vtinput.InputEvent{
			Type: vtinput.MouseEventType, MouseX: int16(px), MouseY: int16(py),
		})
	}
	press(x+1, y+1, 0)
	press(x+7, y+1, vtinput.MouseMoved)
	release(x+7, y+1)
	node := pf.CmdLine.Edit.SemanticNode(nil)
	if node["selectionStart"] != 7 || node["selectionEnd"] != 13 {
		t.Fatalf("drag selection = [%v,%v], want [7,13]", node["selectionStart"], node["selectionEnd"])
	}
	press(x+2, y+1, vtinput.DoubleClick)
	release(x+2, y+1)
	node = pf.CmdLine.Edit.SemanticNode(nil)
	if node["selectionStart"] != 6 || node["selectionEnd"] != 12 {
		t.Fatalf("word selection = [%v,%v]", node["selectionStart"], node["selectionEnd"])
	}
	press(x+2, y+1, vtinput.DoubleClick)
	press(x+10, y+1, vtinput.MouseMoved)
	release(x+10, y+1)
	node = pf.CmdLine.Edit.SemanticNode(nil)
	if node["selectionStart"] != 6 || node["selectionEnd"] != 17 {
		t.Fatalf("word drag selection = [%v,%v], want [6,17]", node["selectionStart"], node["selectionEnd"])
	}
	press(x+2, y+1, vtui.TripleClick)
	release(x+2, y+1)
	node = pf.CmdLine.Edit.SemanticNode(nil)
	if node["selectionStart"] != 6 || node["selectionEnd"] != 17 {
		t.Fatalf("paragraph selection = [%v,%v]", node["selectionStart"], node["selectionEnd"])
	}
}

func TestMultilineCommandResizesPanelsAndKeepsLiteralBreaks(t *testing.T) {
	oldConfig := config.App
	t.Cleanup(func() { config.App = oldConfig })
	config.App.CommandLineMultiline = true
	config.App.CommandLineWordWrap = true
	config.App.NavigationMode = config.NavigationSearchFirst
	pf, pty := panelsFrameWithMouseSelect(t)
	pf.ShowPanels = true
	pf.CmdLine.SetVisible(true)
	pf.SetCommandLineFocus(true)
	pf.ResizeConsole(80, 25)
	_, _, _, panelBottom := pf.Panels[0].GetPosition()
	pf.CmdLine.Edit.SetText("app -arg")
	pf.CmdLine.Edit.ClearSelection()
	pf.InsertCommandLineBreak()
	pf.CmdLine.InsertString("second\nthird")
	pf.ResizeConsole(80, 25)
	if got := pf.CmdLine.Edit.GetText(); got != "app -arg\nsecond\nthird" {
		t.Fatalf("text=%q", got)
	}
	if got := pf.CmdLine.Y2 - pf.CmdLine.Y1 + 1; got != 4 {
		t.Fatalf("rows=%d", got)
	}
	_, _, _, bottom := pf.Panels[0].GetPosition()
	if bottom != panelBottom-3 {
		t.Fatalf("panel bottom=%d, want %d", bottom, panelBottom-3)
	}
	if len(pty.writes) != 0 {
		t.Fatal("newline executed command")
	}
	pf.CmdLine.Edit.SetText(strings.Repeat("word ", 1000))
	pf.ResizeConsole(80, 25)
	if got := pf.CmdLine.Y2 - pf.CmdLine.Y1 + 1; got != 12 {
		t.Fatalf("height cap=%d", got)
	}
	config.App.CommandLineMultiline = false
	pf.ResizeConsole(80, 25)
	if pf.CmdLine.Y1 != pf.CmdLine.Y2 {
		t.Fatal("disabled input did not collapse")
	}
	_, _, _, bottom = pf.Panels[0].GetPosition()
	if bottom != panelBottom {
		t.Fatal("panels did not expand back")
	}
}
