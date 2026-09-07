package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type lifecyclePTY struct {
	mockPty
	sizes    []ptyGeometry
	failNext bool
}

func (pty *lifecyclePTY) SetSize(columns, rows int) {
	pty.sizes = append(pty.sizes, ptyGeometry{columns: columns, rows: rows})
}

func (pty *lifecyclePTY) SetSizeChecked(columns, rows int) error {
	pty.SetSize(columns, rows)
	if pty.failNext {
		pty.failNext = false
		return errors.New("temporary resize failure")
	}
	return nil
}

type lifecycleDocumentFrame struct{ vtui.BaseFrame }

func (*lifecycleDocumentFrame) GetType() vtui.FrameType { return vtui.TypeUser }

func TestDocumentIncrementalSceneRestoresPanelsKeyBarWithoutCellPaint(t *testing.T) {
	metadata, paged := setExtUiPanelCatalogMetadataEnabled(true), setExtUiPanelCatalogRowsEnabled(false)
	t.Cleanup(func() { setExtUiPanelCatalogMetadataEnabled(metadata); setExtUiPanelCatalogRowsEnabled(paged) })
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(80, 25)
	screen.FillRect(0, 0, 79, 24, '#', 123)
	vtui.FrameManager.Init(screen)
	panels := NewPanelsFrame()
	defer panels.Close()
	panels.ResizeConsole(80, 25)
	vtui.FrameManager.Push(panels)
	panels.SemanticNode(nil) // The existing window has already published its catalog.
	readKeyBar := func(want *vtui.KeySet) {
		t.Helper()
		projection, ok := BuildAppIncrementalScene(nil)
		if !ok || projection == nil {
			t.Fatal("production incremental scene projection rejected document transition")
		}
		bar, ok := projection.Scene["keyBar"].(map[string]any)
		if !ok || bar["visible"] != true {
			t.Fatalf("missing visible semantic keybar: %+v", bar)
		}
		items := appMapSlice(bar["items"])
		for index, label := range want.Normal {
			if index >= len(items) || semanticString(items[index]["text"]) != label {
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
	viewer := cachedSemanticViewer([]byte("viewer\ncontent\n"))
	viewer.menuBar = vtui.NewMenuBar(nil)
	viewer.ResizeConsole(80, 25)
	vtui.FrameManager.AddScreen(viewer)
	readKeyBar(viewer.GetKeyLabels())
	vtui.FrameManager.CloseActiveScreen()
	readKeyBar(panels.GetKeyLabels())
	panels.showPanels = false // Ctrl+O retains normal-screen keybar ownership.
	readKeyBar(panels.GetKeyLabels())
	panels.termView.UseAltScreen = true
	if header := vtui.FrameManager.ExportSemanticSceneHeader(); header["keyBar"] != nil || vtui.FrameManager.KeyBar != nil {
		t.Fatal("alternate-screen terminal retained keybar ownership")
	}
}

func TestDocumentTransitionsKeepHiddenPTYGeometry(t *testing.T) {
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(80, 25)
	vtui.FrameManager.Init(screen)
	vtui.FrameManager.ConfigureWorkspaceTabs(vtui.WorkspaceTabsMultiple, vtui.WorkspaceCtrlTabDirect)
	panels := NewPanelsFrame()
	defer panels.Close()
	pty := &lifecyclePTY{}
	panels.ptyMutex.Lock()
	panels.pty = pty
	panels.ptyMutex.Unlock()
	panels.ResizeConsole(80, 25)
	vtui.FrameManager.Push(panels)
	initialWidth, initialHeight := panels.termView.Width, panels.termView.Height
	if len(pty.sizes) != 1 {
		t.Fatalf("initial geometry sends=%d", len(pty.sizes))
	}
	vtui.FrameManager.AddScreen(&lifecycleDocumentFrame{})
	if len(pty.sizes) != 1 || panels.termView.Width != initialWidth || panels.termView.Height != initialHeight {
		t.Fatalf("opening document resized hidden terminal: sends=%+v grid=%dx%d", pty.sizes, panels.termView.Width, panels.termView.Height)
	}
	vtui.FrameManager.CloseActiveScreen()
	if len(pty.sizes) != 1 {
		t.Fatalf("closing document renegotiated unchanged terminal: %+v", pty.sizes)
	}
	vtui.FrameManager.AddScreen(&lifecycleDocumentFrame{})
	screen.AllocBuf(104, 39)
	vtui.FrameManager.ResizeAllScreens()
	if len(pty.sizes) != 1 || !panels.terminalGeometryPending {
		t.Fatalf("hidden resize applied early: %+v pending=%v", pty.sizes, panels.terminalGeometryPending)
	}
	vtui.FrameManager.SwitchScreen(0)
	if len(pty.sizes) != 2 || panels.termView.Width != 104 || panels.terminalGeometryPending {
		t.Fatalf("activation did not apply newest geometry once: %+v grid=%dx%d pending=%v", pty.sizes, panels.termView.Width, panels.termView.Height, panels.terminalGeometryPending)
	}
	last := pty.sizes[len(pty.sizes)-1]
	if last.columns != panels.termView.Width || last.rows != panels.termView.Height {
		t.Fatalf("child/grid mismatch: %+v grid=%dx%d", last, panels.termView.Width, panels.termView.Height)
	}
}

func TestPTYGeometryNegotiationTracksLiveChildrenWithoutRedundantResize(t *testing.T) {
	panels := &PanelsFrame{}
	first, second := &lifecyclePTY{}, &lifecyclePTY{}
	panels.negotiatePTYGeometry(first, 80, 24, 10, 20)
	panels.negotiatePTYGeometry(first, 80, 24, 12, 24) // A non-pixel backend sees no change.
	panels.negotiatePTYGeometry(second, 80, 24, 10, 20)
	panels.negotiatePTYGeometry(second, 80, 24, 10, 20)
	panels.negotiatePTYGeometry(first, 100, 30, 10, 20)
	if len(first.sizes) != 2 || len(second.sizes) != 1 {
		t.Fatalf("first=%+v second=%+v", first.sizes, second.sizes)
	}
}

func TestPTYGeometryFailedResizeIsRetried(t *testing.T) {
	panels := &PanelsFrame{}
	pty := &lifecyclePTY{failNext: true}
	panels.negotiatePTYGeometry(pty, 80, 24, 0, 0)
	if _, known := panels.lastPTYGeometry[pty]; known {
		t.Fatal("failed geometry was acknowledged")
	}
	panels.negotiatePTYGeometry(pty, 80, 24, 0, 0)
	panels.negotiatePTYGeometry(pty, 80, 24, 0, 0)
	if len(pty.sizes) != 2 {
		t.Fatalf("resize attempts=%d, want failed attempt plus successful retry", len(pty.sizes))
	}
}

func TestPendingDocumentEscapeVetoAndCloseCancellation(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	panels := NewPanelsFrame()
	defer panels.Close()
	request := beginPendingDocumentOpen(panels, "viewer", nil, "document.txt")
	escape := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE}
	if !panels.VetoActionKey(escape) {
		t.Fatal("pending open did not veto bound Escape action")
	}
	if !panels.ProcessKey(escape) || request.ctx.Err() != context.Canceled || pendingDocumentOpens[panels] != nil {
		t.Fatal("Escape did not cancel pending open")
	}
	request = beginPendingDocumentOpen(panels, "editor", nil, "document.txt")
	panels.Close()
	if request.ctx.Err() != context.Canceled || pendingDocumentOpens[panels] != nil {
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
	previous := MacroMgr
	MacroMgr = NewMacroManager(path)
	defer func() { MacroMgr = previous }()
	panels := NewPanelsFrame()
	defer panels.Close()
	writeMacro("B")
	panels.ProcessKey(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: false})
	panels.ProcessKey(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: true})
	macroKey := func() uint16 { return MacroMgr.Macros["Common"]["F6"][0].VirtualKeyCode }
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
