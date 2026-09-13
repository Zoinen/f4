package panel

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/cmdline"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestCommandLineDropInsertsQuotedPathsOnce(t *testing.T) {
	pf, pty := panelsFrameWithMouseSelect(t)
	pf.CmdLine.SetVisible(true)
	pf.CmdLine.Edit.SetText("echo ")
	pf.CmdLine.Edit.ClearSelection()
	changes := 0
	pf.CmdLine.Edit.OnTextChange = func(string) { changes++ }
	paths := []string{`D:\two words\α.txt`, `D:\next.txt`}
	if !pf.HandleSemanticAction(map[string]any{"action": "commandLine.dropPaths", "paths": paths}) {
		t.Fatal("drop rejected")
	}
	var quoted []string
	for _, path := range paths {
		word, _ := cmdline.QuoteCommandPath(userMenuCommandDialect(pf), path)
		quoted = append(quoted, word)
	}
	if got, want := pf.CmdLine.Edit.GetText(), "echo "+strings.Join(quoted, " ")+" "; got != want {
		t.Fatalf("input = %q, want %q", got, want)
	}
	if changes != 1 || len(pty.writes) != 0 {
		t.Fatalf("changes=%d, PTY writes=%d", changes, len(pty.writes))
	}
	pf.CmdLine.Edit.SelectAll()
	if !pf.handleCommandLineDrop(map[string]any{"paths": paths[:1]}) || strings.Contains(pf.CmdLine.Edit.GetText(), "echo") {
		t.Fatal("drop did not replace selection")
	}
}

func TestCommandLineDropResolvesInternalSourceAndRejectsStale(t *testing.T) {
	pf := setupMockPanelsFrame(t)
	defer pf.Close()
	pf.ShowPanels = true
	pf.CmdLine.SetVisible(true)
	panel := pf.Panels[0].(*FileSystemPanel)
	panel.Entries = []*FileEntry{{VFSItem: vfs.VFSItem{Name: "one.txt"}}}
	model := panel.SemanticPanelModel(&vtui.SemanticContext{Width: 160, Height: 50}, 0, true)
	source := map[string]any{"side": 0, "panelId": model.ID, "path": model.Path,
		"catalogRevision": model.CatalogRevision, "entryIds": []string{model.Entries[0].EntryID}}
	if !pf.handleCommandLineDrop(map[string]any{"source": source}) {
		t.Fatal("internal source rejected")
	}
	before := pf.CmdLine.Edit.GetText()
	source["path"] = "stale-path"
	if pf.handleCommandLineDrop(map[string]any{"source": source}) || pf.CmdLine.Edit.GetText() != before {
		t.Fatal("stale source changed command line")
	}
}
