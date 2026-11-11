package panel

import (
	errors "errors"
	"strings"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/terminal"
	vtui "github.com/unxed/vtui"
	testing "testing"
)

type lifecyclePTY struct {
	mockPty
	sizes    [][2]int
	failNext bool
}

func (pty *lifecyclePTY) SetSize(columns, rows int) {
	pty.sizes = append(pty.sizes, [2]int{columns, rows})
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

func TestDocumentTransitionsKeepHiddenPTYGeometry(t *testing.T) {
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(80, 25)
	vtui.FrameManager.Init(screen)
	vtui.FrameManager.ConfigureWorkspaceTabs(vtui.WorkspaceTabsMultiple, vtui.WorkspaceCtrlTabDirect)
	panels := NewPanelsFrame()
	defer panels.Close()
	pty := &lifecyclePTY{}
	panels.PtyMutex.Lock()
	panels.Pty = pty
	panels.PtyMutex.Unlock()
	panels.ResizeConsole(80, 25)
	vtui.FrameManager.Push(panels)
	initialWidth, initialHeight := panels.TermView.Width, panels.TermView.Height
	if len(pty.sizes) != 1 {
		t.Fatalf("initial geometry sends=%d", len(pty.sizes))
	}
	vtui.FrameManager.AddScreen(&lifecycleDocumentFrame{})
	if len(pty.sizes) != 1 || panels.TermView.Width != initialWidth || panels.TermView.Height != initialHeight {
		t.Fatalf("opening document resized hidden terminal: sends=%+v grid=%dx%d", pty.sizes, panels.TermView.Width, panels.TermView.Height)
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
	if len(pty.sizes) != 2 || panels.TermView.Width != 104 || panels.terminalGeometryPending {
		t.Fatalf("activation did not apply newest geometry once: %+v grid=%dx%d pending=%v", pty.sizes, panels.TermView.Width, panels.TermView.Height, panels.terminalGeometryPending)
	}
	last := pty.sizes[len(pty.sizes)-1]
	if last[0] != panels.TermView.Width || last[1] != panels.TermView.Height {
		t.Fatalf("child/grid mismatch: %+v grid=%dx%d", last, panels.TermView.Width, panels.TermView.Height)
	}
}

func TestNativePanelTogglePreservesTerminalAndPublishesPrompt(t *testing.T) {
	original := config.App
	t.Cleanup(func() { config.App = original })
	config.App.NavigationMode = config.NavigationSearchFirst
	config.App.SearchCommandHideUnfocused = true
	config.App.GuiPresentation = config.GuiPresentationGUI
	panels := setupMockPanelsFrame(t)
	defer panels.Close()
	screen := vtui.FrameManager.Screen()
	originalRenderer := screen.Renderer
	t.Cleanup(func() { screen.Renderer = originalRenderer })
	screen.Renderer = galleryMenuRenderer{SurfaceRenderer: originalRenderer, native: true}
	panels.ShellMode = terminal.ShellModeOwn
	panels.ShowPanels = true
	pty := &lifecyclePTY{}
	panels.Pty = pty
	panels.ResizeConsole(80, 25)
	parser := terminal.NewAnsiParser(panels.TermView, nil)
	parser.Process([]byte("\x1b[Hfinal output\r\nshell>"))
	before := len(pty.sizes)
	for _, visible := range []bool{true, false, true, false} {
		panels.TogglePanelsVisibility()
		if cmd := panels.commandLineSemanticModel(nil); cmd.Visible != visible {
			t.Fatalf("published prompt visibility = %v, want %v", cmd.Visible, visible)
		}
		if len(pty.sizes) != before {
			t.Fatalf("panel toggle changed PTY geometry: %v", pty.sizes)
		}
		if !strings.Contains(string(panels.TermView.GetAllLogBytes()), "final output") {
			t.Fatal("panel toggle lost the terminal output")
		}
	}
}

func TestNativeCommandLineWrapPreservesTerminalOutput(t *testing.T) {
	original := config.App
	t.Cleanup(func() { config.App = original })
	config.App.GuiPresentation = config.GuiPresentationGUI
	config.App.CommandLineMultiline = true
	config.App.CommandLineWordWrap = true

	panels := setupMockPanelsFrame(t)
	defer panels.Close()
	screen := vtui.FrameManager.Screen()
	originalRenderer := screen.Renderer
	t.Cleanup(func() { screen.Renderer = originalRenderer })
	screen.Renderer = galleryMenuRenderer{SurfaceRenderer: originalRenderer, native: true}
	panels.ShellMode = terminal.ShellModeOwn
	panels.ShowPanels = false
	pty := &lifecyclePTY{}
	panels.Pty = pty
	panels.ResizeConsole(80, 25)
	parser := terminal.NewAnsiParser(panels.TermView, nil)
	parser.Process([]byte("\x1b[Hfirst file\r\nlast file\r\nshell>"))
	initialHeight := panels.TermView.Height
	initialResizeCount := len(pty.sizes)

	for _, step := range []struct {
		input string
		rows  int
	}{
		{input: "ls ", rows: 1},
		{input: "ls -", rows: 2},
		{input: "ls ", rows: 1},
	} {
		panels.CmdLine.Edit.SetText(step.input)
		if got := panels.commandLineRows(80, 12); got != step.rows {
			t.Fatalf("command line %q: got %d rows, want %d", step.input, got, step.rows)
		}
		panels.ResizeConsole(80, 25)
		if panels.TermView.Height != initialHeight || len(pty.sizes) != initialResizeCount {
			t.Fatalf("command line %q resized terminal: height=%d, requests=%v",
				step.input, panels.TermView.Height, pty.sizes)
		}
		output := string(panels.TermView.GetAllLogBytes())
		if !strings.Contains(output, "first file\nlast file") {
			t.Fatalf("command line %q changed terminal output: %q", step.input, output)
		}
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
