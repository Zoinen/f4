package panel

import (
	"strings"
	"testing"

	"github.com/unxed/vtinput"
)

func TestConsolePasteCommitsOnce(t *testing.T) {
	pf, pty := panelsFrameWithMouseSelect(t)
	pf.CmdLine.SetVisible(true)
	pf.CmdLine.Edit.SetText("prefix ")
	pf.CmdLine.Edit.ClearSelection()
	changes := 0
	pf.CmdLine.Edit.OnTextChange = func(string) { changes++ }
	text := strings.Repeat("▶ Moving DSC00100.MP4 ━━ 100%\n", 20)
	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.PasteEventType, PasteStart: true, InputSource: "extui"})
	for _, r := range text {
		pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: r, InputSource: "extui_paste"})
	}
	if changes != 0 {
		t.Fatalf("paste was shown incrementally: %d text changes before paste end", changes)
	}
	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.PasteEventType, InputSource: "extui"})
	if changes != 1 {
		t.Fatalf("paste changes=%d, want one", changes)
	}
	if got, want := pf.CmdLine.Edit.GetText(), "prefix "+text; got != want {
		t.Fatalf("paste=%q, want %q", got, want)
	}
	if len(pty.writes) != 0 {
		t.Fatal("command-line paste executed text in PTY")
	}
}

func TestConsolePasteReplacesSelectionAndResumesTyping(t *testing.T) {
	pf, _ := panelsFrameWithMouseSelect(t)
	pf.CmdLine.SetVisible(true)
	pf.CmdLine.Edit.SetText("replace me")
	pf.CmdLine.Edit.SelectAll()
	pf.CmdLine.Edit.HistoryPos = 2
	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.PasteEventType, PasteStart: true})
	for _, r := range "α\r\nβ" {
		pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: r})
	}
	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.PasteEventType})
	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: '!'})
	if got := pf.CmdLine.Edit.GetText(); got != "α\nβ!" {
		t.Fatalf("paste and next key = %q", got)
	}
	if pf.CmdLine.Edit.HistoryPos != -1 || pf.commandLinePasting {
		t.Fatal("paste did not reset input transaction/history state")
	}
}
