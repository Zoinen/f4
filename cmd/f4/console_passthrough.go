package main

import (
	"fmt"
	"strings"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// mutedPTY wraps a PtyBackend to silence automated parser and terminal responses
// (such as CPR, DSR, DA, OSC 52, far2l APC) while mirroring in ShellModeHost.
// The host terminal provides real responses, so the internal mirror must stay mute.
// Note: mutedPTY intentionally does NOT implement PtyPixelSizer.
type mutedPTY struct {
	backend PtyBackend
}

func (m mutedPTY) Read(p []byte) (int, error)            { return m.backend.Read(p) }
func (m mutedPTY) Write(p []byte) (int, error)           { return len(p), nil }
func (m mutedPTY) Close() error                          { return m.backend.Close() }
func (m mutedPTY) SetSize(cols, rows int)                { m.backend.SetSize(cols, rows) }
func (m mutedPTY) Wait() error                           { return m.backend.Wait() }
func (m mutedPTY) Run(name string, args ...string) error { return m.backend.Run(name, args...) }
func (m mutedPTY) IsBusy() bool                          { return m.backend.IsBusy() }

func (pf *PanelsFrame) isHostConsoleActive() bool {
	pf.hostConsoleMu.Lock()
	defer pf.hostConsoleMu.Unlock()
	return pf.hostConsoleActive
}

// SetBusy sets the frame's busy state to suppress rendering during host console operations.
func (pf *PanelsFrame) SetBusy(busy bool) {
	pf.Busy = busy
}

// consoleStyle returns the console view style effective for this frame.
func (pf *PanelsFrame) consoleStyle() string {
	return consoleViewStyleFor(pf.shellMode)
}

// overlayLines returns the number of bottom rows reserved for the f4 overlay (0, 1, or 2).
func (pf *PanelsFrame) overlayLines() int {
	if pf.consoleStyle() != ConsoleViewFar {
		return 0
	}
	n := 1 // CommandLine
	if pf.showKeyBar {
		n++
	}
	return n
}

// consoleOverlayContent is the backend-independent description of the overlay:
// what to put on the command line row, where the cursor belongs, and the keybar
// labels. The ANSI and Win32 Console emitters below render the same struct.
type consoleOverlayContent struct {
	Lines     int
	Cmd       string
	CursorCol int
	Keys      []overlayKeySlot
	Popup     *overlayPopupContent
}

type overlayPopupContent struct {
	X         int
	Y         int
	Width     int
	Height    int
	SelectPos int
	Items     []string
}

// overlayKeySlot is one F-key cell of the overlay keybar: the number, its
// label, and the column the number starts at.
type overlayKeySlot struct {
	Col   int
	Num   string
	Label string
}

// overlayKeybarSlots lays the keybar out exactly the way vtui.KeyBar does, so
// the console overlay and the panel keybar agree on slot width and label
// truncation. The overlay used to hardcode five columns per label, which is why
// it showed "RenMo" where the real keybar had room for "Rename or move".
// Column math only, no drawing: unit-testable and shared by both emitters.
func overlayKeybarSlots(labels vtui.KeyBarLabels, width int) []overlayKeySlot {
	if width <= 0 {
		return nil
	}
	slotWidth := width / 12
	if slotWidth < 3 {
		slotWidth = 3
	}
	slots := make([]overlayKeySlot, 0, 12)
	for i := 0; i < 12; i++ {
		x := i * slotWidth
		if x > width-1 {
			break
		}
		num := fmt.Sprintf("%d", i+1)
		labelX := x + len([]rune(num))
		labelW := slotWidth - len([]rune(num)) - 1
		if i == 11 {
			// The last slot swallows the rounding remainder, as in vtui.
			labelW = width - labelX
		}
		if available := width - labelX; labelW > available {
			labelW = available
		}
		if labelW < 0 {
			labelW = 0
		}
		label := []rune(labels[i])
		if len(label) > labelW {
			label = label[:labelW]
		}
		slots = append(slots, overlayKeySlot{
			Col:   x,
			Num:   num,
			Label: fmt.Sprintf("%-*s", labelW, string(label)),
		})
	}
	return slots
}

// buildConsoleOverlayContent collects the overlay text from the live UI state.
func (pf *PanelsFrame) buildConsoleOverlayContent() consoleOverlayContent {
	ov := consoleOverlayContent{Lines: pf.overlayLines()}

	var sb strings.Builder
	for _, ci := range pf.buildPrompt() {
		sb.WriteString(vtui.CellString(ci.Char))
	}
	if pf.cmdLine != nil && pf.cmdLine.Edit != nil {
		sb.WriteString(pf.cmdLine.Edit.GetText())
	}
	ov.Cmd = sb.String()
	// Edit keeps its caret position private, so the cursor is parked at the end
	// of the typed text. Exact placement needs an accessor in vtui.
	ov.CursorCol = len([]rune(ov.Cmd))

	if pf.showKeyBar && ov.Lines >= 2 {
		if labels := pf.GetKeyLabels(); labels != nil {
			ov.Keys = overlayKeybarSlots(labels.Normal, pf.lastW)
		}
	}

	if vtui.FrameManager != nil {
		if ac, ok := vtui.FrameManager.GetTopFrame().(*vtui.AutoCompleteMenu); ok && ac != nil && ac.HasMatches() {
			x1, y1, x2, y2 := ac.GetPosition()
			ov.Popup = &overlayPopupContent{
				X:         x1,
				Y:         y1,
				Width:     x2 - x1 + 1,
				Height:    y2 - y1 + 1,
				SelectPos: ac.SelectPos(),
				Items:     ac.Matches,
			}
		}
	}
	return ov
}

// consoleOverlayUsesWinAPI reports whether the overlay must be painted with the
// Windows Console API instead of ANSI: under the winapi renderer the visible
// screen buffer is not a VT stream and escape sequences would land in it as
// literal text.
func consoleOverlayUsesWinAPI() bool {
	if SelectedTTYBackend != "winapi" && SelectedTTYBackend != "win32" {
		return false
	}
	return winConsoleOverlayAvailable()
}

// consoleViewActive reports whether the frame currently shows a console view
// (host console with a live shell, or the no-PTY console) rather than panels.
func (pf *PanelsFrame) consoleViewActive() bool {
	switch pf.shellMode {
	case ShellModeHost:
		return pf.isHostConsoleActive()
	case ShellModeSimpleInline:
		return !pf.showPanels
	}
	return false
}

// isTopFrame reports whether pf itself is the frame currently on top of the
// active screen's stack. consoleViewActive() alone tracks only pf.showPanels,
// which does NOT flip when some other frame -- an editor or viewer opened via
// F3/F4 while the console view is showing, a dialog -- gets pushed on top
// of pf. We also treat an active AutoCompleteMenu as part of the panels
// frame if it is directly on top of it, so the overlay isn't frozen while
// autocomplete hints are visible during typing.
func (pf *PanelsFrame) isTopFrame() bool {
	if vtui.FrameManager == nil || len(vtui.FrameManager.Screens) == 0 {
		return false
	}
	idx := vtui.FrameManager.ActiveIdx
	if idx < 0 || idx >= len(vtui.FrameManager.Screens) {
		return false
	}
	frames := vtui.FrameManager.Screens[idx].Frames
	if len(frames) == 0 {
		return false
	}
	top := frames[len(frames)-1]
	if top == vtui.Frame(pf) {
		return true
	}
	if _, ok := top.(*vtui.AutoCompleteMenu); ok {
		if len(frames) >= 2 && frames[len(frames)-2] == vtui.Frame(pf) {
			return true
		}
	}
	return false
}

// drawConsoleOverlay paints the f4 command line and keybar over the console.
func (pf *PanelsFrame) drawConsoleOverlay() {
	if pf.overlayLines() == 0 || !pf.consoleViewActive() {
		return
	}
	// zoin-bot: the console overlay owns the physical bottom rows; leaving the
	// frame-manager keybar registered would repaint a second copy afterwards.
	pf.suppressFrameManagerKeyBar()
	ov := pf.buildConsoleOverlayContent()
	// The single most useful line in a Wine bug report: which emitter ran and
	// what geometry it believed in. "Overlay drew at the top of the window"
	// is either a wrong pf.lastH (ansi) or a wrong srWindow (winapi), and
	// this tells the two apart without guessing.
	p := probeConsole()
	vtui.DebugLog("OVERLAY: backend=%q winapi=%v lastW=%d lastH=%d lines=%d keys=%d csbi=%v win=%dx%d",
		SelectedTTYBackend, consoleOverlayUsesWinAPI(), pf.lastW, pf.lastH, ov.Lines, len(ov.Keys),
		p.OK, p.WinCols(), p.WinRows())
	if consoleOverlayUsesWinAPI() {
		winDrawConsoleOverlay(ov)
		return
	}
	pf.emitAnsiConsoleOverlay(ov)
}

// zoin-bot: suppressFrameManagerKeyBar prevents the normal ScreenBuf renderer from
// drawing a second keybar while the console overlay paints directly to the
// host terminal. The next panel redraw registers it again when appropriate.
func (pf *PanelsFrame) suppressFrameManagerKeyBar() {
	if vtui.FrameManager != nil && vtui.FrameManager.KeyBar == pf.keyBar {
		vtui.FrameManager.KeyBar = nil
	}
}

// clearConsoleOverlay takes the overlay off the screen and gives the cursor back
// to the spot where the previous output ended. Without it, a command's output
// scrolls a copy of the command line into the console history.
func (pf *PanelsFrame) clearConsoleOverlay() {
	n := pf.overlayLines()
	if n == 0 {
		return
	}
	if consoleOverlayUsesWinAPI() {
		winClearConsoleOverlay(n)
		return
	}
	h := pf.lastH
	if h <= 0 {
		return
	}
	var sb strings.Builder
	sb.WriteString("\x1b7")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&sb, "\x1b[%d;1H\x1b[0m\x1b[2K", h-n+1+i)
	}
	sb.WriteString("\x1b8")
	vtui.WritePassthrough([]byte(sb.String()))
}

// drawHostConsoleOverlay is kept as the name used by the host console call sites.
func (pf *PanelsFrame) drawHostConsoleOverlay() {
	pf.drawConsoleOverlay()
}

// emitAnsiConsoleOverlay renders the overlay directly to the host terminal
// using minimal ANSI escape sequences without involving ScreenBuf.
func (pf *PanelsFrame) emitAnsiConsoleOverlay(ov consoleOverlayContent) {
	h := pf.lastH
	if h <= 0 {
		return
	}
	n := ov.Lines
	cmdRow := h - n + 1 // 1-based row index

	var sb strings.Builder
	// 1. Save cursor position
	sb.WriteString("\x1b7")

	// 2. Draw CommandLine
	fmt.Fprintf(&sb, "\x1b[%d;1H\x1b[0m\x1b[2K", cmdRow)
	sb.WriteString(ov.Cmd)

	// 3. Draw KeyBar if visible
	if len(ov.Keys) > 0 {
		fmt.Fprintf(&sb, "\x1b[%d;1H\x1b[0m\x1b[2K", h)
		for _, k := range ov.Keys {
			// Slots carry their own start column now: rounding leftovers make
			// the widths uneven, so walking them by concatenation would drift.
			fmt.Fprintf(&sb, "\x1b[%d;%dH", h, k.Col+1)
			// Matches vtui's real KeyBar palette (ColKeyBarNum/ColKeyBarText:
			// "LightGray on DarkGray / DarkGray on Teal"). This used to be
			// swapped, which made the overlay look like a different, alien
			// keybar sitting next to the real one instead of matching it.
			fmt.Fprintf(&sb, "\x1b[0;37;40m%s\x1b[0;30;46m%s", k.Num, k.Label)
		}
	}

	// 4. Restore cursor position and visibility
	sb.WriteString("\x1b[0m\x1b8")

	// Without a PTY there is no shell to own the cursor, so f4 parks it in its
	// own command line: that blinking caret is what tells the user the console
	// is waiting for a command rather than hung.
	if pf.shellMode == ShellModeSimpleInline {
		fmt.Fprintf(&sb, "\x1b[%d;%dH\x1b[?25h", cmdRow, ov.CursorCol+1)
	}
	vtui.WritePassthrough([]byte(sb.String()))
}

// syncAutoCompleteSuppression keeps CommandLine.AutoCompleteSuppressed in
// step with where the popup can actually be drawn safely. winDrawConsoleOverlay
// (console_overlay_windows.go) paints AutoCompleteMenu directly with the
// Windows Console API, so it's safe there. The ANSI console-view path has no
// such renderer yet -- pushing the menu there would go through vtui's normal
// full-frame Show(), which is exactly the leak documented in WINE.md
// §2c/§2j.5 (one frame of panels/keybar flashed onto the live console).
// Suppress only in that gap; panels mode and the winapi console view are
// unaffected.
func (pf *PanelsFrame) syncAutoCompleteSuppression() {
	if pf.cmdLine == nil {
		return
	}
	pf.cmdLine.AutoCompleteSuppressed = pf.consoleViewActive() && !consoleOverlayUsesWinAPI()
}

// handleHostConsoleTab completes a TAB in the f4-owned command line before it
// can be forwarded to the shell behind a host-console overlay. The normal
// autocomplete menu is safe on the WinAPI overlay because it has a native
// renderer. ANSI host consoles cannot repaint a popup over the shell's screen
// without a readable backing buffer, so they accept the first selectable item
// directly and redraw only the overlay.
func (pf *PanelsFrame) handleHostConsoleTab(e *vtinput.InputEvent) bool {
	if e == nil || e.Type != vtinput.KeyEventType || !e.KeyDown || e.VirtualKeyCode != vtinput.VK_TAB {
		return false
	}
	if e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed|vtinput.LeftAltPressed|vtinput.RightAltPressed|vtinput.ShiftPressed) != 0 {
		return false
	}
	if pf.cmdLine == nil || pf.cmdLine.IsEmpty() || !AppConfig.CommandLineAutoComplete || vtui.FrameManager == nil {
		return false
	}

	if ac, ok := vtui.FrameManager.GetTopFrame().(*vtui.AutoCompleteMenu); ok && ac != nil {
		if ac.Edit != pf.cmdLine.Edit || !ac.HasMatches() {
			return false
		}
		ac.ProcessKey(e)
		pf.drawHostConsoleOverlay()
		return true
	}

	ac := vtui.NewAutoCompleteMenu(pf.cmdLine.Edit)
	if !ac.HasMatches() {
		return false
	}
	if consoleOverlayUsesWinAPI() {
		vtui.FrameManager.Push(ac)
	} else {
		// There is no safe ANSI readback for restoring the shell output under a
		// popup. Complete the same selected item without painting the popup.
		ac.ProcessKey(e)
	}
	pf.drawHostConsoleOverlay()
	return true
}

// enterHostConsole switches the physical terminal to the primary screen and activates
// live passthrough of PTY output directly to the host console.
func (pf *PanelsFrame) enterHostConsole() {
	if pf.shellMode != ShellModeHost {
		return
	}
	pf.hostConsoleMu.Lock()
	if pf.hostConsoleActive {
		pf.hostConsoleMu.Unlock()
		return
	}
	pf.hostConsoleActive = true
	pf.hostConsoleMu.Unlock()
	pf.syncAutoCompleteSuppression()

	pf.SetBusy(true)
	pf.suppressFrameManagerKeyBar()
	vtui.SetAltScreen(false)

	n := pf.overlayLines()
	if n > 0 && pf.lastH > n {
		scrollBottom := pf.lastH - n
		vtui.WritePassthrough([]byte(fmt.Sprintf("\x1b[1;%dr", scrollBottom)))
		pf.drawHostConsoleOverlay()
	}
}

// leaveHostConsole restores the alternate screen buffer and returns visual control to f4 panels.
func (pf *PanelsFrame) leaveHostConsole() {
	if pf.shellMode != ShellModeHost {
		return
	}
	pf.hostConsoleMu.Lock()
	if !pf.hostConsoleActive {
		pf.hostConsoleMu.Unlock()
		return
	}
	pf.hostConsoleActive = false
	pf.hostConsoleMu.Unlock()
	pf.syncAutoCompleteSuppression()

	// A host-console application can receive a mouse-down record without a
	// matching release reaching f4 (notably through native Windows console
	// input). FrameManager keeps routing all later mouse events to the frame
	// that handled that down event until it sees a release. Queue a neutral
	// release while returning control to the panels so a stale capture cannot
	// block the menu bar or modal dialogs.
	if vtui.FrameManager != nil {
		vtui.FrameManager.PostEvent(vtinput.InputEvent{
			Type:        vtinput.MouseEventType,
			MouseX:      -1,
			MouseY:      -1,
			KeyDown:     false,
			ButtonState: 0,
		})
	}

	// Protective reset sequence to clean up any terminal modes left by child applications
	var resetSeq strings.Builder
	if pf.termView != nil && pf.termView.UseAltScreen {
		resetSeq.WriteString("\x1b[?1049l")
	}
	resetSeq.WriteString("\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?2004l\x1b[r\x1b[0m\x1b[?25h")
	vtui.WritePassthrough([]byte(resetSeq.String()))

	vtui.SetAltScreen(true)
	if vtui.FrameManager != nil && vtui.FrameManager.Screen() != nil {
		vtui.FrameManager.Screen().HardReset()
	}
	pf.SetBusy(false)
	if vtui.FrameManager != nil {
		vtui.FrameManager.Redraw()
	}
}
