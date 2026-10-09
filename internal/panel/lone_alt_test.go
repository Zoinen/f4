package panel

import (
	"sync"
	"testing"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtinput"
)

func altKey(down bool, vk uint16, state vtinput.ControlKeyState) *vtinput.InputEvent {
	return &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: down, VirtualKeyCode: vk, ControlKeyState: state}
}

// f4 #1131: only Alt pressed and released on its own, and quickly, is a tap.
// Every chord that merely starts with Alt must not be one.
func TestLoneAltTapRecognisesOnlyALoneQuickPress(t *testing.T) {
	t0 := time.Unix(1000, 0)
	step := 50 * time.Millisecond
	altDown := altKey(true, vtinput.VK_MENU, vtinput.LeftAltPressed)
	altUp := altKey(false, vtinput.VK_MENU, 0)

	cases := []struct {
		name   string
		events []*vtinput.InputEvent
		want   bool
	}{
		{"lone Alt", []*vtinput.InputEvent{altDown, altUp}, true},
		{"left Alt by its own code", []*vtinput.InputEvent{altKey(true, vtinput.VK_LMENU, 0), altKey(false, vtinput.VK_LMENU, 0)}, true},
		{"autorepeat while held", []*vtinput.InputEvent{altDown, altDown, altDown, altUp}, true},
		{"Alt+letter", []*vtinput.InputEvent{altDown,
			{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_A, Char: 'a', ControlKeyState: vtinput.LeftAltPressed},
			altKey(false, vtinput.VK_A, vtinput.LeftAltPressed), altUp}, false},
		{"Alt+F7 consumed by a hotkey", []*vtinput.InputEvent{altDown,
			altKey(true, vtinput.VK_F7, vtinput.LeftAltPressed), altUp}, false},
		{"Ctrl+Alt (AltGr)", []*vtinput.InputEvent{altKey(true, vtinput.VK_CONTROL, vtinput.LeftCtrlPressed),
			altKey(true, vtinput.VK_MENU, vtinput.LeftCtrlPressed|vtinput.RightAltPressed), altUp}, false},
		{"Alt+Shift layout switch", []*vtinput.InputEvent{altDown,
			altKey(true, vtinput.VK_SHIFT, vtinput.LeftAltPressed|vtinput.ShiftPressed), altUp}, false},
		{"Alt+click", []*vtinput.InputEvent{altDown,
			{Type: vtinput.MouseEventType, KeyDown: true, ButtonState: vtinput.FromLeft1stButtonPressed, ControlKeyState: vtinput.LeftAltPressed}, altUp}, false},
		{"pointer moved while Alt held", []*vtinput.InputEvent{altDown,
			{Type: vtinput.MouseEventType, MouseEventFlags: vtinput.MouseMoved}, altUp}, true},
		{"focus lost (Alt+Tab to another window)", []*vtinput.InputEvent{altDown,
			{Type: vtinput.FocusEventType}, altUp}, false},
		{"release without a press", []*vtinput.InputEvent{altUp}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var tr loneAltTracker
			now := t0
			got := false
			for i, e := range tc.events {
				tapped := tr.observe(e, now)
				now = now.Add(step)
				if tapped && i != len(tc.events)-1 {
					t.Fatalf("event %d completed a tap before the end", i)
				}
				got = tapped
			}
			if got != tc.want {
				t.Fatalf("tap = %v, want %v", got, tc.want)
			}
		})
	}

	// Holding Alt to read the Alt row of the key bar and letting go is not a tap.
	var tr loneAltTracker
	tr.observe(altDown, t0)
	if tr.observe(altUp, t0.Add(loneAltMaxHold+time.Millisecond)) {
		t.Error("a long Alt hold counted as a tap")
	}
	// ...and does not leave a stale press behind for the next one.
	tr.observe(altDown, t0.Add(2*time.Second))
	if !tr.observe(altUp, t0.Add(2*time.Second+step)) {
		t.Error("a tap after a long hold was not recognised")
	}
}

// The tap opens and closes the filter only with the option on; the
// Panel.AutoFilter action (PanelsFrame.ToggleAutoFilter) works either way.
func TestPanelsFrameLoneAltTogglesFilterWhenEnabled(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	before := config.App
	defer func() { config.App = before }()
	config.App.NavigationMode = config.NavigationClassic
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	waitForDirectoryLoads(t)
	fsp := pf.GetActivePanel()
	if fsp == nil {
		t.Fatal("no active file panel")
	}

	config.App.PanelAutoFilter = false
	if pf.HandleLoneAlt() || fsp.AutoFilterActive() {
		t.Fatal("a lone Alt opened the filter with the option off")
	}

	config.App.PanelAutoFilter = true
	if !pf.HandleLoneAlt() || !fsp.AutoFilterActive() {
		t.Fatal("a lone Alt did not open the filter")
	}
	if !pf.HandleLoneAlt() || fsp.AutoFilterActive() || fsp.FastFindMode {
		t.Fatal("a second lone Alt did not close the filter")
	}

	config.App.PanelAutoFilter = false
	if !pf.ToggleAutoFilter() || !fsp.AutoFilterActive() {
		t.Fatal("the Panel.AutoFilter action did not open the filter")
	}
	escape := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE}
	pf.ProcessKey(escape)
	if fsp.AutoFilterActive() {
		t.Fatal("Esc did not close the filter")
	}
	if !pf.ShowPanels {
		t.Fatal("Esc closing the filter also hid the panels")
	}
}

// f4 #1131: with AltTabSettings=1 (classic Alt+Tab) and with Alt+Enter, the
// Alt release reaches f4 before the focus loss or the size change that shows
// it was no tap. A completed tap acts only if nothing of the kind follows
// within loneAltGrace.
func TestScheduleLoneAltActsOnlyIfNothingFollows(t *testing.T) {
	oldGrace, oldSize, oldPost := loneAltGrace, loneAltTerminalSize, loneAltPost
	t.Cleanup(func() { loneAltGrace, loneAltTerminalSize, loneAltPost = oldGrace, oldSize, oldPost })
	loneAltGrace = 30 * time.Millisecond
	loneAltPost = func(f func()) { f() }
	var mu sync.Mutex
	w, h := 80, 25
	loneAltTerminalSize = func() (int, int) { mu.Lock(); defer mu.Unlock(); return w, h }

	altDown := altKey(true, vtinput.VK_MENU, vtinput.LeftAltPressed)
	altUp := altKey(false, vtinput.VK_MENU, 0)
	tap := func(between ...func()) chan struct{} {
		fired := make(chan struct{}, 1)
		loneAlt.mu.Lock()
		loneAlt.armed = false
		loneAlt.mu.Unlock()
		LoneAltTap(altDown)
		if !LoneAltTap(altUp) {
			t.Fatal("the release did not complete a tap")
		}
		ScheduleLoneAlt(func() { fired <- struct{}{} })
		for _, f := range between {
			f()
		}
		return fired
	}
	acted := func(c chan struct{}) bool {
		select {
		case <-c:
			return true
		case <-time.After(300 * time.Millisecond):
			return false
		}
	}

	if !acted(tap()) {
		t.Error("an undisturbed tap did not act")
	}
	if acted(tap(func() { LoneAltTap(&vtinput.InputEvent{Type: vtinput.FocusEventType}) })) {
		t.Error("a focus loss right after the release (Alt+Tab) did not cancel the tap")
	}
	if acted(tap(func() { mu.Lock(); w = 120; mu.Unlock() })) {
		t.Error("a size change after the release (Alt+Enter) did not cancel the tap")
	}
	if !acted(tap(func() {
		LoneAltTap(&vtinput.InputEvent{Type: vtinput.MouseEventType, MouseEventFlags: vtinput.MouseMoved})
	})) {
		t.Error("pointer motion cancelled the tap")
	}
}

// Without a grace period the tap acts on the release itself, with no timer:
// that is how it is everywhere but Windows, where the author of #1131 felt
// the old quarter second as a lag.
func TestScheduleLoneAltWithoutGraceActsAtOnce(t *testing.T) {
	oldGrace, oldPost := loneAltGrace, loneAltPost
	t.Cleanup(func() { loneAltGrace, loneAltPost = oldGrace, oldPost })
	loneAltGrace = 0
	fired := false
	loneAltPost = func(f func()) { f() }
	ScheduleLoneAlt(func() { fired = true })
	if !fired {
		t.Fatal("the tap did not act at once")
	}
	if defaultLoneAltGrace() > 100*time.Millisecond {
		t.Fatalf("the default grace is %v, want a delay nobody feels", defaultLoneAltGrace())
	}
}
