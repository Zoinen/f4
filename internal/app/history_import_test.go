package app

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/internal/history"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestFar3ImportSurvivesNormalHistoryUpdates(t *testing.T) {
	previous := vtui.GlobalHistoryProvider
	hp := history.NewProviderAtPath(filepath.Join(t.TempDir(), "history.json"))
	vtui.GlobalHistoryProvider = hp
	t.Cleanup(func() { _ = hp.Close(); vtui.GlobalHistoryProvider = previous })
	var source history.Far3History
	folderRoot := t.TempDir()
	for i := 0; i < 250; i++ {
		name := fmt.Sprintf("old-%d", i)
		source.Commands = append(source.Commands, history.HistoryRecord{Name: name})
		source.Folders = append(source.Folders, history.HistoryRecord{Name: filepath.Join(folderRoot, name)})
		source.Files = append(source.Files, history.ViewerEditorRecord{Path: name, Display: name,
			Local: true, VFSType: "*vfs.OSVFS", Mode: history.HistoryModeView, Timestamp: time.Unix(int64(i), 0)})
	}
	hp.MergeFar3History(source)
	pf := panel.NewPanelsFrame()
	defer pf.Close()
	if pf.CmdLine.Edit.HistoryLimit < 250 {
		t.Fatal("new workspace loses imported history capacity")
	}
	pf.AddCommandHistory("new command")
	history.AddFolderHistory(filepath.Join(t.TempDir(), "new-folder"))
	root := t.TempDir()
	RememberViewerEditorHistory(vfs.NewOSVFS(root), filepath.Join(root, "new-file"), HistoryModeEdit)
	for _, id := range []string{"cmdline", "folders", history.ViewerEditorHistoryID} {
		if got := len(hp.LoadHistory(id)); got != 250 {
			t.Fatalf("%s shrank after use: %d", id, got)
		}
	}
}

func TestFar3HistoryImportActionRegistration(t *testing.T) {
	a, ok := GetAction("History.ImportFar3")
	if !ok || a.MenuPath != "Commands" || a.MenuSubPath != "History" || a.Handler == nil {
		t.Fatalf("import action missing from Commands / History: %+v", a)
	}
}
