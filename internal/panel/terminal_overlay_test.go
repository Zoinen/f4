package panel

import (
	semantic "github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/internal/terminal"
	vtui "github.com/unxed/vtui"
	strings "strings"
	testing "testing"
)

func TestPanelsSemanticProjectionAppliesVisibleCommandLineOverlay(t *testing.T) {
	vtui.SetDefaultPalette()
	pf := setupMockPanelsFrame(t)
	defer pf.Close()
	pf.ResizeConsole(40, 8)

	terminal.NewAnsiParser(pf.TermView, nil).Process([]byte("visible output\r\nidle prompt % "))

	pf.CmdLine.SetVisible(true)
	terminal := semantic.AppMap(pf.SemanticNode(nil)["terminal"])
	text := terminalModelText(semantic.AppMapSlice(terminal["windowRows"]))
	if strings.Contains(text, "idle prompt") {
		t.Fatalf("visible command line did not cover terminal prompt: %q", text)
	}
	if !strings.Contains(text, "visible output") {
		t.Fatalf("visible output disappeared with command line: %q", text)
	}

	pf.CmdLine.SetVisible(false)
	terminal = semantic.AppMap(pf.SemanticNode(nil)["terminal"])
	text = terminalModelText(semantic.AppMapSlice(terminal["windowRows"]))
	if !strings.Contains(text, "idle prompt") {
		t.Fatalf("hidden command line did not restore terminal prompt: %q", text)
	}
}
