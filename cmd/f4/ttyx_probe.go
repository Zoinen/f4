package main

// Where the character grid actually is inside the terminal's window.
//
// The X server knows where the window is. It does not know that the top of
// that window is a menu bar and the right of it a scroll bar, and dividing the
// window by the number of cells in it therefore gives a cell that is slightly
// too large and an origin that is slightly too high — which is exactly what
// put a picture a row and a bit above the space meant for it in gnome-terminal.
//
// The terminal knows. It carries the pixel size of its text area in the window
// size ioctl, and answers for it over the wire as well, and that is the missing
// measurement: the grid is that size and what is left over is furniture.
//
// The questions asked over the wire have to be asked before the input reader
// starts, because afterwards the answer is just another escape sequence
// arriving on standard input and the reader eats it. So all of it happens
// once, at startup, and is remembered.

import (
	"io"
	"os"
	"strings"
	"sync"

	"github.com/unxed/f4/internal/ttyx"
	"github.com/unxed/vtui"
)

var (
	hostTextMu    sync.Mutex
	hostTextW     int
	hostTextH     int
	hostTextKnown bool
	hostCellW     int
	hostCellH     int
	hostCellKnown bool
)

// ProbeHostTextArea works out how large the terminal's text area is. It must
// be called before anything else starts reading standard input.
//
// Three ways, cheapest first.
//
// The window size ioctl carries the text area in pixels alongside the size in
// characters, and every terminal worth the name fills it in. It costs a system
// call, it cannot be lost, and it cannot eat a keystroke — which the other two
// can, being questions asked over the wire and answered on standard input.
//
// Failing that, CSI 16 t gives the size of one cell, which multiplied by the
// grid is the text area exactly and owes nothing to padding, and CSI 14 t
// gives the text area directly.
//
// Failing all three the caller treats the window as the grid, which is right
// for a terminal with no furniture and wrong by the height of a menu bar for
// one with.
func ProbeHostTextArea() {
	if w, h, ok := hostPixelsFromIoctl(os.Stdin); ok {
		vtui.DebugLog("TTYX: TIOCGWINSZ -> text area %dx%d", w, h)
		hostTextMu.Lock()
		hostTextW, hostTextH, hostTextKnown = w, h, true
		hostCellKnown = false
		hostTextMu.Unlock()
		publishCellSize()
		return
	}

	cw, ch, cellOK := queryPixels("\x1b[16t", "\x1b[6;")
	tw, th, areaOK := queryPixels("\x1b[14t", "\x1b[4;")
	vtui.DebugLog("TTYX: no TIOCGWINSZ pixels; CSI 16 t -> cell %dx%d (%v), CSI 14 t -> text area %dx%d (%v)",
		cw, ch, cellOK, tw, th, areaOK)

	hostTextMu.Lock()
	hostCellW, hostCellH, hostCellKnown = cw, ch, cellOK
	hostTextW, hostTextH, hostTextKnown = tw, th, areaOK
	hostTextMu.Unlock()
	restoreFar2lAfterProbe(os.Stdout)
	publishCellSize()
}

// What the questions above read and did not use.
//
// Reading standard input is destructive, and the terminal has usually put
// something there already: the protocols are announced just before this runs,
// and a far2l terminal answers the announcement straight away. That answer
// arrives in the same buffer as the answer being waited for, is not the one
// with the prefix, and is dropped -- and with it the only notice that the
// extensions are live. The clipboard then has no far2l to use in either
// direction, which is what #922 turned out to be.
var (
	swallowedMu sync.Mutex
	swallowed   strings.Builder
)

// noteProbeInput records everything a question read, answer and all.
func noteProbeInput(s string) {
	if s == "" {
		return
	}
	swallowedMu.Lock()
	swallowed.WriteString(s)
	swallowedMu.Unlock()
}

func takeProbeInput() string {
	swallowedMu.Lock()
	defer swallowedMu.Unlock()
	s := swallowed.String()
	swallowed.Reset()
	return s
}

// far2lAck is what a far2l terminal sends when it accepts the announcement,
// and far2lAnnounce is the announcement itself -- the same sequence vtinput
// writes when it enables the protocols.
const (
	far2lAck      = "\x1b_far2lok"
	far2lAnnounce = "\x1b_far2l1\x1b\\"
)

// restoreFar2lAfterProbe asks again when the acknowledgement was among what
// the questions ate, so that the reader gets one it can see. Asking is cheap
// and the terminal answers every time; a terminal that never acknowledged is
// not asked again, so nothing is sent to one that would not understand it.
func restoreFar2lAfterProbe(out io.Writer) bool {
	if !strings.Contains(takeProbeInput(), far2lAck) {
		return false
	}
	vtui.DebugLog("TTYX: the probe ate the far2l acknowledgement; announcing again")
	_, err := io.WriteString(out, far2lAnnounce)
	return err == nil
}

// publishCellSize tells vtui the real size of a character cell. Without it the
// graphics layer answers zero, everything that lays a picture out falls back
// to a guessed eight by sixteen, and a picture ends up the wrong shape as well
// as in the wrong place — the cell of a terminal is nothing like square.
func publishCellSize() {
	scr := vtui.FrameManager.Screen()
	if scr == nil {
		return
	}
	cols, rows := scr.Width(), scr.Height()
	w, h, ok := hostTextSize(cols, rows)
	if !ok || cols <= 0 || rows <= 0 {
		return
	}
	// In device pixels, which is what everything downstream measures in.
	// The session may not be up yet, in which case the scale cannot be
	// worked out and the cell is published in whatever units the terminal
	// answered in; the shape of it is right either way, and the shape is
	// what the layout depends on.
	scale := 1
	if sess := sharedTTYXSession(); sess != nil {
		if win, err := sess.Geometry(); err == nil {
			scale = hostScale(win, w, h)
		}
	}
	cw, ch := w*scale/cols, h*scale/rows
	if cw <= 0 || ch <= 0 {
		return
	}
	vtui.DebugLog("TTYX: the cell is %dx%d pixels (scale %d)", cw, ch, scale)
	scr.Graphics().SetCellSize(cw, ch)
}

// hostCellSize is the pixel size of one cell as the terminal reported it.
func hostCellSize() (int, int, bool) {
	hostTextMu.Lock()
	defer hostTextMu.Unlock()
	return hostCellW, hostCellH, hostCellKnown
}

func hostTextArea() (w, h int, ok bool) {
	hostTextMu.Lock()
	defer hostTextMu.Unlock()
	return hostTextW, hostTextH, hostTextKnown
}

// hostTextSize is the size of the character grid in pixels, worked out from
// whichever question the terminal answered. The cell is preferred because it
// owes nothing to padding: multiplied by the grid it is the text area exactly,
// while the reported text area is whatever the terminal chooses to call one.
func hostTextSize(cols, rows int) (int, int, bool) {
	if cw, ch, ok := hostCellSize(); ok && cols > 0 && rows > 0 {
		return cols * cw, rows * ch, true
	}
	return hostTextArea()
}

// hostScale finds the factor between a text area the terminal reported and the
// window the X server reported.
//
// They are not in the same units on a scaled desktop. GTK answers CSI 14 t in
// logical pixels, X reports the window in device pixels, and on a display at
// double scale the terminal says its text area is 640x408 while its window is
// 1312x868 — half of it, and the picture came out half size in the bottom left
// corner. Nothing in either answer says which units it is in, so the factor is
// worked out: the largest whole number of text areas that still fits inside
// the window.
//
// It cannot be fooled by a terminal with a lot of furniture, because choosing
// two would need the menu bar and the scroll bar together to be as large as
// the text between them.
func hostScale(win ttyx.Rect, textW, textH int) int {
	if textW <= 0 || textH <= 0 {
		return 1
	}
	best := 1
	for s := 2; s <= 4; s++ {
		if textW*s <= win.W && textH*s <= win.H {
			best = s
		}
	}
	return best
}

// hostGridRect works out where the character grid sits inside the terminal
// window, given where the window is.
//
// With a measured text area the grid is put against the left and the bottom of
// the window. That is where every terminal that has furniture keeps it: a menu
// bar is at the top, a scroll bar is on the right, and neither is ever at the
// bottom left. A terminal with a symmetric border is out by the width of that
// border, which is a pixel or two.
//
// Without a measurement the grid is the whole window, which is what this did
// before it could measure anything, and is right for a terminal with no
// furniture at all.
func hostGridRect(win ttyx.Rect, textW, textH int, known bool) ttyx.Rect {
	if !known || textW <= 0 || textH <= 0 {
		return win
	}
	if s := hostScale(win, textW, textH); s > 1 {
		textW, textH = textW*s, textH*s
	}
	if textW > win.W {
		textW = win.W
	}
	if textH > win.H {
		textH = win.H
	}
	return ttyx.Rect{
		X: win.X,
		Y: win.Y + (win.H - textH),
		W: textW,
		H: textH,
	}
}
