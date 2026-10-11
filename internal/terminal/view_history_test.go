package terminal

import (
	"fmt"
	"strings"
	"testing"
)

func TestCloneHistoryFromCopiesOutputWithoutSharingSession(t *testing.T) {
	source := NewTerminalView(32, 4)
	parser := NewAnsiParser(source, nil)
	parser.Process([]byte("\x1b[H"))
	for i := 0; i < 2200; i++ {
		parser.Process([]byte(fmt.Sprintf("saved %04d\r\n", i)))
	}
	parser.Process([]byte("old prompt>"))
	if source.Pt.Size() == 0 || len(source.GridHistory) == 0 {
		t.Fatal("fixture did not exercise saved and recent scrollback")
	}
	sourceLog := string(source.GetAllLogBytes())
	source.Pty = &mockPtyForTerminal{}
	source.Win32InputMode = true
	source.BracketedPasteMode = true
	source.KittyFlags.Store(7)
	source.SelActive = true
	destination := NewTerminalView(32, 4)
	destinationPTY := &mockPtyForTerminal{}
	destination.Pty = destinationPTY
	destination.CloneHistoryFrom(source, true)
	copyLog := string(destination.GetAllLogBytes())
	if want := strings.TrimSuffix(sourceLog, "old prompt>"); copyLog != want {
		t.Fatal("inherited history lost or duplicated output across the history stores")
	}
	if destination.Pty != destinationPTY || destination.Win32InputMode || destination.BracketedPasteMode ||
		destination.KittyFlags.Load() != 0 || destination.SelActive || destination.UseAltScreen {
		t.Fatal("history inheritance copied source shell ownership or protocol state")
	}
	if source.Pt == destination.Pt || source.Li == destination.Li || source.Engine == destination.Engine {
		t.Fatal("inherited history shares source storage")
	}
	parser.Process([]byte("\r\nlater source output\r\n"))
	if string(destination.GetAllLogBytes()) != copyLog {
		t.Fatal("later source output changed the copied history")
	}
	sourceLog = string(source.GetAllLogBytes())
	NewAnsiParser(destination, nil).Process([]byte("\x1b[Hnew shell startup\r\nnew result\r\nnew prompt>"))
	if got := string(destination.GetAllLogBytes()); !strings.HasPrefix(got, copyLog) ||
		!strings.HasSuffix(got, "new shell startup\nnew result\nnew prompt>") {
		t.Fatal("new shell overwrote inherited history or placed it after its output")
	}
	if string(source.GetAllLogBytes()) != sourceLog {
		t.Fatal("new shell output changed the original history")
	}
}
