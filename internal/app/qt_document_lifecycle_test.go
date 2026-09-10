package app

import (
	"context"
	"github.com/unxed/f4/internal/macro"
	"github.com/unxed/f4/internal/nativeui"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/f4/internal/viewer"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"testing"
)

func TestDocumentIncrementalSceneRestoresPanelsKeyBarWithoutCellPaint(t *testing.T) {
	metadata, paged := semantic.SetPanelCatalogMetadataEnabled(true), semantic.SetPanelCatalogRowsEnabled(false)
	t.Cleanup(func() {
		semantic.SetPanelCatalogMetadataEnabled(metadata)
		semantic.SetPanelCatalogRowsEnabled(paged)
	})
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(80, 25)
	screen.FillRect(0, 0, 79, 24, '#', 123)
	vtui.FrameManager.Init(screen)
	panels := panel.NewPanelsFrame()
	defer panels.Close()
	panels.ResizeConsole(80, 25)
	vtui.FrameManager.Push(panels)
	panels.SemanticNode(nil) // The existing window has already published its catalog.
	readKeyBar := func(want *vtui.KeySet) {
		t.Helper()
		projection, ok := nativeui.BuildAppIncrementalScene(nil)
		if !ok || projection == nil {
			t.Fatal("production incremental scene projection rejected document transition")
		}
		bar, ok := projection.Scene["keyBar"].(map[string]any)
		if !ok || bar["visible"] != true {
			t.Fatalf("missing visible semantic keybar: %+v", bar)
		}
		items := semantic.AppMapSlice(bar["items"])
		for index, label := range want.Normal {
			if index >= len(items) || semantic.String(items[index]["text"]) != label {
				t.Fatalf("semantic key %d label=%v want=%q", index, items, label)
			}
		}
		for y := 0; y < 25; y++ {
			for x := 0; x < 80; x++ {
				if cell := screen.GetCell(x, y); cell.Char != '#' || cell.Attributes != 123 {
					t.Fatalf("incremental transition painted hidden cell %d,%d: %+v", x, y, cell)
				}
			}
		}
	}
	readKeyBar(panels.GetKeyLabels())
	filesystem := terminal.NewTerminalLogVFS(nil, func() []byte { return []byte("viewer\ncontent\n") })
	viewer, err := viewer.NewViewerView(context.Background(), filesystem, "Terminal Log")
	if err != nil {
		t.Fatal(err)
	}
	viewer.ResizeConsole(80, 25)
	vtui.FrameManager.AddScreen(viewer)
	readKeyBar(viewer.GetKeyLabels())
	vtui.FrameManager.CloseActiveScreen()
	readKeyBar(panels.GetKeyLabels())
	panels.ShowPanels = false // Ctrl+O retains normal-screen keybar ownership.
	readKeyBar(panels.GetKeyLabels())
	panels.TermView.UseAltScreen = true
	if header := vtui.FrameManager.ExportSemanticSceneHeader(); header["keyBar"] != nil || vtui.FrameManager.KeyBar != nil {
		t.Fatal("alternate-screen terminal retained keybar ownership")
	}
}

func TestPendingDocumentEscapeVetoAndCloseCancellation(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	panels := panel.NewPanelsFrame()
	defer panels.Close()
	request := beginPendingDocumentOpen(panels, "viewer", nil, "document.txt")
	escape := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE}
	if !panels.VetoActionKey(escape) {
		t.Fatal("pending open did not veto bound Escape action")
	}
	if !panels.ProcessKey(escape) || request.ctx.Err() != context.Canceled || PendingDocumentOpens[panels] != nil {
		t.Fatal("Escape did not cancel pending open")
	}
	request = beginPendingDocumentOpen(panels, "editor", nil, "document.txt")
	panels.Close()
	if request.ctx.Err() != context.Canceled || PendingDocumentOpens[panels] != nil {
		t.Fatal("workspace close did not cancel pending open")
	}
}

func TestApplicationActivationReloadsMacrosButInternalFocusDoesNot(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	path := filepath.Join(t.TempDir(), "macros.ini")
	writeMacro := func(key string) {
		t.Helper()
		if err := os.WriteFile(path, []byte("[KeyMacros/Common/F6]\nSequence="+key+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeMacro("A")
	previous := macro.MacroMgr
	macro.MacroMgr = macro.NewMacroManager(path)
	defer func() { macro.MacroMgr = previous }()
	panels := panel.NewPanelsFrame()
	defer panels.Close()
	writeMacro("B")
	panels.ProcessKey(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: false})
	panels.ProcessKey(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: true})
	macroKey := func() uint16 { return macro.MacroMgr.Macros["Common"]["F6"][0].VirtualKeyCode }
	if macroKey() != 'A' {
		t.Fatal("internal frame focus reloaded macros")
	}
	filter := applicationActivationFilter(nil)
	filter(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: false})
	filter(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: true})
	if macroKey() != 'B' {
		t.Fatal("real application activation did not reload shared macros")
	}
	writeMacro("C")
	filter(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: true})
	if macroKey() != 'B' {
		t.Fatal("duplicate focused event reloaded macros")
	}
}
