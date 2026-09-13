package panel

import (
	"github.com/unxed/f4/internal/config"
	"strings"
	"testing"
)

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
