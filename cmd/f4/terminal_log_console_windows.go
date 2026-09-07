//go:build windows

package main

import (
	"strings"
	"unsafe"
)

// readHostConsoleFullText reads the entire host console screen buffer --
// not just the visible window, but info.Size, which under a console that
// actually has scrollback (e.g. wineconsole: dwSize taller than srWindow,
// see WINE.md §2g) covers everything scrolled off the top too -- and
// returns it as text, one line per buffer row with trailing blanks trimmed.
//
// This is the F3/F4 (Terminal.ViewLog/EditLog) data source for
// ShellModeSimpleInline: there is no PTY in that mode, so pf.termView is
// never fed by anything the user actually ran. runSimpleInlineCommand()
// execs the child with inherited stdio straight into this same console
// buffer, bypassing the ANSI parser and termView entirely -- so termView's
// own log is always empty here, and the console buffer itself is the only
// real record of command history to show. See simple_exec.go and
// TerminalLogVFS.
//
// overlayLines excludes the trailing rows the console-view overlay itself
// owns (the command line and, if shown, the keybar -- see
// PanelsFrame.overlayLines()): those are f4's own UI chrome, redrawn fresh
// on every keystroke, not history. Without excluding them, F3/F4 would
// include a snapshot of whatever the command line/keybar happened to say at
// the moment the log was opened, right in the middle of the text.
func readHostConsoleFullText(overlayLines int) []byte {
	hOut, ok := winOverlayHandle()
	if !ok {
		return nil
	}
	info, ok := winOverlayInfo(hOut)
	if !ok {
		return nil
	}
	w := int(info.Size.X)
	h := int(info.Size.Y)
	if w <= 0 || h <= 0 {
		return nil
	}

	// The overlay is pinned to the bottom of the *visible window*
	// (info.Window.Bottom), not necessarily the bottom of the full buffer --
	// same arithmetic as winDrawConsoleOverlay's cmdRow. Exclude that same
	// range from the log text.
	excludeFrom := h
	if overlayLines > 0 {
		cmdRow := int(info.Window.Bottom) - overlayLines + 1
		if cmdRow >= 0 && cmdRow < h {
			excludeFrom = cmdRow
		}
	}

	// Some Console API hosts choke on very large single reads; read in
	// horizontal strips well under any such limit rather than assume one
	// call for the whole (possibly tall) scrollback buffer is safe.
	const maxRowsPerCall = 200
	cells := make([]simpleCharInfo, w*h)
	for top := 0; top < h; top += maxRowsPerCall {
		bottom := top + maxRowsPerCall - 1
		if bottom > h-1 {
			bottom = h - 1
		}
		rows := bottom - top + 1
		chunk := make([]simpleCharInfo, w*rows)
		bufSize := uintptr(uint32(uint16(w)) | (uint32(uint16(rows)) << 16))
		region := simpleSmallRect{Left: 0, Top: int16(top), Right: int16(w - 1), Bottom: int16(bottom)}
		procReadConsoleOutputW.Call(
			uintptr(hOut),
			uintptr(unsafe.Pointer(&chunk[0])),
			bufSize,
			0,
			uintptr(unsafe.Pointer(&region)),
		)
		copy(cells[top*w:(top+rows)*w], chunk)
	}

	var sb strings.Builder
	for row := 0; row < excludeFrom; row++ {
		line := cells[row*w : (row+1)*w]
		last := w - 1
		for last >= 0 && line[last].UnicodeChar == ' ' {
			last--
		}
		for i := 0; i <= last; i++ {
			sb.WriteRune(rune(line[i].UnicodeChar))
		}
		sb.WriteByte('\n')
	}
	return []byte(sb.String())
}
