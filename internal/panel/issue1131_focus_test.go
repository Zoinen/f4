package panel

import (
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtinput"
)

// f4 #1131 (follow-up): an application focus change -- Alt+Tab to another
// window, or a mouse click into one -- must not close an open autofilter
// window. The filter is a window the user opened on purpose, so only a
// repeat lone Alt or Esc is allowed to close it; losing and regaining the
// application's focus is neither.
func TestPanelsFrameFocusLossKeepsAutoFilterOpen(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	before := config.App
	defer func() { config.App = before }()
	config.App.NavigationMode = config.NavigationClassic
	config.App.PanelAutoFilter = true
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	waitForDirectoryLoads(t)
	fsp := pf.GetActivePanel()
	if fsp == nil {
		t.Fatal("no active file panel")
	}

	if !pf.HandleLoneAlt() || !fsp.AutoFilterActive() {
		t.Fatal("a lone Alt did not open the filter")
	}
	typeIntoFilter(t, fsp, "a")
	if !fsp.AutoFilterActive() {
		t.Fatal("typing into the open filter closed it")
	}

	focusLost := &vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: false}
	if !pf.ProcessKey(focusLost) {
		t.Fatal("PanelsFrame did not consume the focus-lost event")
	}
	if !fsp.AutoFilterActive() || !fsp.FastFindMode {
		t.Fatal("losing application focus closed the autofilter window")
	}

	focusGained := &vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: true}
	pf.ProcessKey(focusGained)
	if !fsp.AutoFilterActive() {
		t.Fatal("regaining application focus should not touch an open filter either")
	}

	// A repeat lone Alt still closes it.
	if !pf.HandleLoneAlt() || fsp.AutoFilterActive() || fsp.FastFindMode {
		t.Fatal("a second lone Alt did not close the filter after a focus change")
	}

	// Esc closes it too, the same as before this change.
	if !pf.HandleLoneAlt() || !fsp.AutoFilterActive() {
		t.Fatal("a lone Alt did not reopen the filter")
	}
	escape := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE}
	pf.ProcessKey(escape)
	if fsp.AutoFilterActive() {
		t.Fatal("Esc did not close the filter")
	}
}

// The plain cursor-moving quick search (autofilter not engaged) is
// unaffected by this change: it stays the transient UI it always was, and a
// focus change still clears it -- only the filter window gained the new
// immunity above.
func TestPanelsFrameFocusLossStillClosesPlainQuickSearch(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	before := config.App
	defer func() { config.App = before }()
	config.App.NavigationMode = config.NavigationClassic
	config.App.PanelAutoFilter = false
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	waitForDirectoryLoads(t)
	fsp := pf.GetActivePanel()
	if fsp == nil {
		t.Fatal("no active file panel")
	}

	typeIntoPanel(t, fsp, "a")
	if !fsp.FastFindMode || fsp.AutoFilterActive() {
		t.Fatal("Alt+letter did not start the plain quick search")
	}

	focusLost := &vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: false}
	pf.ProcessKey(focusLost)
	if fsp.FastFindMode {
		t.Fatal("losing application focus should still close the plain quick search")
	}
}
