package panel

import (
	"testing"

	"github.com/unxed/f4/internal/cmdline"
)

func TestPanelsFrameCommandLineAndHistoryGuards(t *testing.T) {
	pf := &PanelsFrame{CmdLine: cmdline.NewCommandLine(">")}
	pf.InsertPathToCmdLine("")
	if got := pf.CmdLine.Edit.GetText(); got != "" {
		t.Fatalf("empty path changed command line to %q", got)
	}
	pf.InsertPathToCmdLine("two words")
	if got := pf.CmdLine.Edit.GetText(); got == "" {
		t.Fatal("path with spaces was not inserted")
	}
	pf.AddCommandHistory("echo coverage")
	if got := pf.CmdLine.Edit.History; len(got) == 0 || got[0] != "echo coverage" {
		t.Fatalf("command history = %v", got)
	}
}
