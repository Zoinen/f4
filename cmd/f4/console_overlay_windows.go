//go:build windows

package main

import (
	"syscall"
	"unsafe"

	"github.com/unxed/vtui"
)

// Painting the overlay on Windows cannot go through ANSI: with the winapi
// renderer f4 draws into its own screen buffer while the console the user sees
// after Ctrl+O is the original one, and that buffer is not a VT stream.
// Everything here therefore writes cells with the classic Console API.
var (
	procGetConsoleScreenBufferInfoOverlay = kernel32SimpleExec.NewProc("GetConsoleScreenBufferInfo")
	procSetConsoleCursorPositionOverlay   = kernel32SimpleExec.NewProc("SetConsoleCursorPosition")
	procSetConsoleCursorInfoOverlay       = kernel32SimpleExec.NewProc("SetConsoleCursorInfo")
	procSetConsoleWindowInfoOverlay       = kernel32SimpleExec.NewProc("SetConsoleWindowInfo")
)

type overlayCoord struct {
	X int16
	Y int16
}

type overlayCursorInfo struct {
	Size    uint32
	Visible int32
}

type overlayBufferInfo struct {
	Size              overlayCoord
	CursorPosition    overlayCoord
	Attributes        uint16
	Window            simpleSmallRect
	MaximumWindowSize overlayCoord
}

const (
	// Matches vtui's real KeyBar palette (palette.go: ColKeyBarNum /
	// ColKeyBarText, "LightGray on DarkGray / DarkGray on Teal"). The overlay
	// used to have these two swapped — light text on cyan for the number,
	// plain gray for the label — which is why it read as a different, off
	// keybar next to the real one instead of a seamless continuation of it.
	overlayAttrNum  = uint16(0x07) // light gray on dark gray — the "N" digit
	overlayAttrText = uint16(0x30) // dark text on teal — the label
)

type overlayPopupState struct {
	left      int16
	top       int16
	width     int16
	height    int16
	selectPos int
	count     int
	firstItem string
}

// The console cursor as the child process left it. The overlay moves the cursor
// into the command line, so the original position has to come back before the
// next command starts printing.
var (
	overlaySavedCursor      overlayCoord
	overlaySavedCursorValid bool

	overlaySavedPopupRect  simpleSmallRect
	overlaySavedPopupBuf   []simpleCharInfo
	overlaySavedPopupValid bool
	overlayLastPopupState  overlayPopupState
)

// popupColors reads the actual colors AutoCompleteMenu uses when drawn
// normally (Palette[ColDialogBox]/[ColDialogText]/[ColDialogSelectedButton],
// see vtui's autocomplete.go NewAutoCompleteMenu) and converts them to Win32
// Console attributes. Used instead of hardcoded bytes so the console-view
// popup matches the current theme instead of a fixed "classic Far" blue --
// previously frame and text were guessed wrong (white/yellow on blue instead
// of the dialog's actual black on light gray); only the selection color
// happened to match by coincidence.
func popupColors() (frame, text, sel uint16) {
	frame = vtui.AttrToWin32Attr(vtui.Palette[vtui.ColDialogBox], nil)
	text = vtui.AttrToWin32Attr(vtui.Palette[vtui.ColDialogText], nil)
	sel = vtui.AttrToWin32Attr(vtui.Palette[vtui.ColDialogSelectedButton], nil)
	return
}

func winConsoleOverlayAvailable() bool { return true }

// clearConsoleViewBackground paints the visible console window before the
// overlay goes on top of it.
//
// Entering the console view assumes SetConsoleActiveScreenBuffer switches the
// display to a buffer that already holds the shell's output, so f4 draws only
// its two overlay rows and nothing else. Under Wine that switch does not
// produce a visually distinct buffer -- the panels f4 drew are still the
// pixels on screen, and nothing ever writes over them, so they stay until
// some unrelated full repaint happens (WINE.md §2k.1).
//
// An earlier attempt read the window and wrote the same cells straight back,
// hoping to force a repaint. That could never work: rewriting the panels with
// the panels changes nothing. What is actually missing is content -- so write
// content: the saved console snapshot when one of the right size exists (that
// is the previous commands' output, which the far-style console view is
// supposed to keep), otherwise blanks.
//
// No-op (and safe to call) if the console handle or geometry can't be read.
func clearConsoleViewBackground(w, h int) {
	if hostConsoleBufferMatches(w, h) {
		restoreHostConsoleBufferImpl()
		return
	}
	// Blanking is a Wine-only fallback. On a real Windows console the buffer
	// switch works, whatever the shell printed is genuinely there, and
	// erasing it would throw away exactly what the far-style console view
	// exists to show.
	if !vtui.IsWine() {
		return
	}
	h2, ok := winOverlayHandle()
	if !ok {
		return
	}
	info, ok := winOverlayInfo(h2)
	if !ok {
		return
	}
	cols := int(info.Window.Right) - int(info.Window.Left) + 1
	rows := int(info.Window.Bottom) - int(info.Window.Top) + 1
	if cols <= 0 || rows <= 0 {
		return
	}
	buf := make([]simpleCharInfo, cols*rows)
	for i := range buf {
		buf[i].UnicodeChar = ' '
		buf[i].Attributes = overlayAttrNum
	}
	region := simpleSmallRect{
		Left:   info.Window.Left,
		Top:    info.Window.Top,
		Right:  info.Window.Right,
		Bottom: info.Window.Bottom,
	}
	bufSize := uintptr(uint32(uint16(cols)) | (uint32(uint16(rows)) << 16))
	procWriteConsoleOutputW.Call(
		uintptr(h2),
		uintptr(unsafe.Pointer(&buf[0])),
		bufSize,
		0,
		uintptr(unsafe.Pointer(&region)),
	)
}

func winOverlayHandle() (syscall.Handle, bool) {
	h, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil || h == 0 || h == syscall.InvalidHandle {
		return 0, false
	}
	return h, true
}

func winOverlayInfo(h syscall.Handle) (overlayBufferInfo, bool) {
	var info overlayBufferInfo
	r1, _, _ := procGetConsoleScreenBufferInfoOverlay.Call(uintptr(h), uintptr(unsafe.Pointer(&info)))
	return info, r1 != 0
}

func winOverlayCoordArg(x, y int16) uintptr {
	return uintptr(uint32(uint16(x)) | uint32(uint16(y))<<16)
}

// pinOverlayWindow re-asserts the console window's rectangle before every
// draw, exactly like vtui's own resetConsoleWindowPos() does on every
// Flush() of f4's own screen buffer (win32_console_windows.go) — the code
// path that, per every screenshot so far, renders correctly under Wine.
// The overlay never did this for the host buffer: it only ever *read*
// GetConsoleScreenBufferInfo and trusted the answer. Under a real Wine
// console the buffer legitimately has scrollback (dwSize taller than
// srWindow — confirmed via wineconsole f4.exe: dwSize=80x150,
// srWindow=25 rows), and if the window rect conhost/wineconsole is tracking
// internally ever drifts from what GetConsoleScreenBufferInfo reports back
// (a caching or repaint-ordering gap, not something visible from outside),
// passively trusting the read is exactly the class of bug that produces
// "the numbers say bottom, the pixels say top." Pinning the window to the
// same rectangle we just read is a no-op if Wine's internal state already
// agrees with it, and forces a resync if it does not.
func pinOverlayWindow(h syscall.Handle, w *simpleSmallRect) {
	rect := *w
	procSetConsoleWindowInfoOverlay.Call(uintptr(h), uintptr(1), uintptr(unsafe.Pointer(&rect)))
}

func newOverlayRow(width int) []simpleCharInfo {
	row := make([]simpleCharInfo, width)
	for i := range row {
		// Plain background: same light-gray-on-dark as the keybar's own
		// number cells, never the teal reserved for keybar labels (see
		// fillOverlayText(cmdCells, ...) below for why this matters).
		row[i] = simpleCharInfo{UnicodeChar: ' ', Attributes: overlayAttrNum}
	}
	return row
}

// fillOverlayText writes s into row starting at col and returns the next column.
func fillOverlayText(row []simpleCharInfo, col int, s string, attr uint16) int {
	for _, r := range s {
		if col >= len(row) {
			break
		}
		ch := uint16('?')
		if r < 0x10000 {
			ch = uint16(r)
		}
		row[col] = simpleCharInfo{UnicodeChar: ch, Attributes: attr}
		col++
	}
	return col
}

func winWriteOverlayRow(h syscall.Handle, left, right, top int16, cells []simpleCharInfo) {
	if len(cells) == 0 {
		return
	}
	region := simpleSmallRect{Left: left, Top: top, Right: right, Bottom: top}
	procWriteConsoleOutputW.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&cells[0])),
		winOverlayCoordArg(int16(len(cells)), 1),
		winOverlayCoordArg(0, 0),
		uintptr(unsafe.Pointer(&region)),
	)
}

// winDrawConsoleOverlay paints the overlay onto the bottom rows of the visible
// console window. Rows are counted from srWindow.Bottom rather than from the
// buffer height: a console buffer is routinely taller than its window, and
// using the buffer would put the command line somewhere off screen.
func winDrawConsoleOverlay(ov consoleOverlayContent) {
	if ov.Lines <= 0 {
		return
	}
	h, ok := winOverlayHandle()
	if !ok {
		return
	}
	info, ok := winOverlayInfo(h)
	if !ok {
		return
	}
	// A child that just finished writing can leave CursorPosition ahead of
	// srWindow: GetConsoleScreenBufferInfo's cursor field already reflects
	// the line it printed, but under Wine the window rect that follows the
	// cursor can lag behind by one repaint tick if this read races the
	// child's last write. A single-line command like "echo 123" returns
	// fast enough for that race to matter; a multi-line command like "dir"
	// spans enough console writes that the window has caught up by the time
	// we get here -- which is exactly the "dir shows, echo doesn't" split.
	// pinOverlayWindow() below re-asserts whatever rect we hand it, so
	// pinning a stale one *locks in* the race instead of letting the next
	// repaint self-correct. Extend the rect down to include the cursor row
	// first, so the pin never excludes the output the child just produced.
	if info.CursorPosition.Y > info.Window.Bottom {
		shift := info.CursorPosition.Y - info.Window.Bottom
		info.Window.Top += shift
		info.Window.Bottom += shift
	}
	pinOverlayWindow(h, &info.Window)
	// Re-read after pinning: if the pin forced Wine to reconcile a stale
	// window rect, this is the corrected geometry; if the pin was a no-op,
	// this is identical to what was already read above.
	if info2, ok2 := winOverlayInfo(h); ok2 {
		info = info2
	}
	left, right := info.Window.Left, info.Window.Right
	width := int(right-left) + 1
	if width <= 0 {
		return
	}
	cmdRow := info.Window.Bottom - int16(ov.Lines) + 1
	if cmdRow < info.Window.Top {
		return
	}
	// This is the geometry actually driving the write below, queried by this
	// function itself rather than a separate probe call — if it ever
	// disagrees with the OVERLAY: line's own probe, that gap is the bug.
	// After the pin-and-reread above, so a nonempty diff between this line
	// and the OVERLAY: line one above it in the log means the pin actually
	// changed what Wine reports.
	vtui.DebugLog("OVERLAY_WIN: dwSize=%dx%d srWindow=L%dT%dR%dB%d cmdRow=%d cursor=%d,%d",
		info.Size.X, info.Size.Y, info.Window.Left, info.Window.Top, info.Window.Right, info.Window.Bottom,
		cmdRow, info.CursorPosition.X, info.CursorPosition.Y)

	if !overlaySavedCursorValid {
		overlaySavedCursor = info.CursorPosition
		overlaySavedCursorValid = true
	}

	// 1. Determine popup geometry and changes
	hasPopup := ov.Popup != nil && len(ov.Popup.Items) > 0 && ov.Popup.Width > 0 && ov.Popup.Height > 0
	var popupFrameAttr, popupTextAttr, popupSelAttr uint16
	if hasPopup {
		popupFrameAttr, popupTextAttr, popupSelAttr = popupColors()
	}
	var newPopupState overlayPopupState
	if hasPopup {
		popW := int16(ov.Popup.Width)
		popH := int16(ov.Popup.Height)
		popLeft := left + int16(ov.Popup.X)
		if popLeft+popW > right+1 {
			popLeft = right + 1 - popW
		}
		if popLeft < left {
			popLeft = left
		}
		popTop := cmdRow - popH
		if popTop < info.Window.Top {
			popTop = info.Window.Top
		}
		newPopupState = overlayPopupState{
			left:      popLeft,
			top:       popTop,
			width:     popW,
			height:    popH,
			selectPos: ov.Popup.SelectPos,
			count:     len(ov.Popup.Items),
			firstItem: ov.Popup.Items[0],
		}
	}

	popupNeedsFullRedraw := false
	if overlaySavedPopupValid {
		if !hasPopup || newPopupState.left != overlayLastPopupState.left ||
			newPopupState.top != overlayLastPopupState.top ||
			newPopupState.width != overlayLastPopupState.width ||
			newPopupState.height != overlayLastPopupState.height ||
			newPopupState.count != overlayLastPopupState.count ||
			newPopupState.firstItem != overlayLastPopupState.firstItem {
			// Geometry or items changed, or popup closed: restore old background
			pW := int(overlaySavedPopupRect.Right-overlaySavedPopupRect.Left) + 1
			pH := int(overlaySavedPopupRect.Bottom-overlaySavedPopupRect.Top) + 1
			if pW > 0 && pH > 0 && len(overlaySavedPopupBuf) == pW*pH {
				pBufSize := uintptr(uint32(uint16(pW)) | (uint32(uint16(pH)) << 16))
				r := overlaySavedPopupRect
				procWriteConsoleOutputW.Call(
					uintptr(h),
					uintptr(unsafe.Pointer(&overlaySavedPopupBuf[0])),
					pBufSize,
					0,
					uintptr(unsafe.Pointer(&r)),
				)
			}
			overlaySavedPopupValid = false
			popupNeedsFullRedraw = true
		}
	} else if hasPopup {
		popupNeedsFullRedraw = true
	}

	if hasPopup && popupNeedsFullRedraw {
		popLeft := newPopupState.left
		popTop := newPopupState.top
		popRight := popLeft + newPopupState.width - 1
		popBottom := popTop + newPopupState.height - 1
		pW := int(popRight-popLeft) + 1
		pH := int(popBottom-popTop) + 1

		if pW > 0 && pH > 0 {
			overlaySavedPopupRect = simpleSmallRect{
				Left:   popLeft,
				Top:    popTop,
				Right:  popRight,
				Bottom: popBottom,
			}
			overlaySavedPopupBuf = make([]simpleCharInfo, pW*pH)
			pBufSize := uintptr(uint32(uint16(pW)) | (uint32(uint16(pH)) << 16))
			readR := overlaySavedPopupRect
			procReadConsoleOutputW.Call(
				uintptr(h),
				uintptr(unsafe.Pointer(&overlaySavedPopupBuf[0])),
				pBufSize,
				0,
				uintptr(unsafe.Pointer(&readR)),
			)
			overlaySavedPopupValid = true
			overlayLastPopupState = newPopupState

			// Draw popup box
			popCells := make([]simpleCharInfo, pW*pH)
			for i := range popCells {
				popCells[i] = simpleCharInfo{UnicodeChar: ' ', Attributes: popupTextAttr}
			}
			// Border
			for cx := 0; cx < pW; cx++ {
				popCells[cx] = simpleCharInfo{UnicodeChar: 0x2500, Attributes: popupFrameAttr}
				popCells[(pH-1)*pW+cx] = simpleCharInfo{UnicodeChar: 0x2500, Attributes: popupFrameAttr}
			}
			for cy := 0; cy < pH; cy++ {
				popCells[cy*pW] = simpleCharInfo{UnicodeChar: 0x2502, Attributes: popupFrameAttr}
				popCells[cy*pW+pW-1] = simpleCharInfo{UnicodeChar: 0x2502, Attributes: popupFrameAttr}
			}
			popCells[0] = simpleCharInfo{UnicodeChar: 0x250C, Attributes: popupFrameAttr}
			popCells[pW-1] = simpleCharInfo{UnicodeChar: 0x2510, Attributes: popupFrameAttr}
			popCells[(pH-1)*pW] = simpleCharInfo{UnicodeChar: 0x2514, Attributes: popupFrameAttr}
			popCells[(pH-1)*pW+pW-1] = simpleCharInfo{UnicodeChar: 0x2518, Attributes: popupFrameAttr}

			// Items
			for idx, itemText := range ov.Popup.Items {
				rowIdx := idx + 1
				if rowIdx >= pH-1 {
					break
				}
				attr := popupTextAttr
				if idx == ov.Popup.SelectPos {
					attr = popupSelAttr
				}
				for col := 1; col < pW-1; col++ {
					popCells[rowIdx*pW+col] = simpleCharInfo{UnicodeChar: ' ', Attributes: attr}
				}
				fillOverlayText(popCells[rowIdx*pW+1:], 0, itemText, attr)
			}

			writeR := overlaySavedPopupRect
			procWriteConsoleOutputW.Call(
				uintptr(h),
				uintptr(unsafe.Pointer(&popCells[0])),
				pBufSize,
				0,
				uintptr(unsafe.Pointer(&writeR)),
			)
		}
	} else if hasPopup && overlayLastPopupState.selectPos != newPopupState.selectPos {
		// Only selection position changed: update item rows without re-reading background
		popLeft := newPopupState.left
		popTop := newPopupState.top
		popRight := popLeft + newPopupState.width - 1
		popBottom := popTop + newPopupState.height - 1
		pW := int(popRight-popLeft) + 1
		pH := int(popBottom-popTop) + 1

		popCells := make([]simpleCharInfo, pW*pH)
		for i := range popCells {
			popCells[i] = simpleCharInfo{UnicodeChar: ' ', Attributes: popupTextAttr}
		}
		for cx := 0; cx < pW; cx++ {
			popCells[cx] = simpleCharInfo{UnicodeChar: 0x2500, Attributes: popupFrameAttr}
			popCells[(pH-1)*pW+cx] = simpleCharInfo{UnicodeChar: 0x2500, Attributes: popupFrameAttr}
		}
		for cy := 0; cy < pH; cy++ {
			popCells[cy*pW] = simpleCharInfo{UnicodeChar: 0x2502, Attributes: popupFrameAttr}
			popCells[cy*pW+pW-1] = simpleCharInfo{UnicodeChar: 0x2502, Attributes: popupFrameAttr}
		}
		popCells[0] = simpleCharInfo{UnicodeChar: 0x250C, Attributes: popupFrameAttr}
		popCells[pW-1] = simpleCharInfo{UnicodeChar: 0x2510, Attributes: popupFrameAttr}
		popCells[(pH-1)*pW] = simpleCharInfo{UnicodeChar: 0x2514, Attributes: popupFrameAttr}
		popCells[(pH-1)*pW+pW-1] = simpleCharInfo{UnicodeChar: 0x2518, Attributes: popupFrameAttr}

		for idx, itemText := range ov.Popup.Items {
			rowIdx := idx + 1
			if rowIdx >= pH-1 {
				break
			}
			attr := popupTextAttr
			if idx == ov.Popup.SelectPos {
				attr = popupSelAttr
			}
			for col := 1; col < pW-1; col++ {
				popCells[rowIdx*pW+col] = simpleCharInfo{UnicodeChar: ' ', Attributes: attr}
			}
			fillOverlayText(popCells[rowIdx*pW+1:], 0, itemText, attr)
		}

		pBufSize := uintptr(uint32(uint16(pW)) | (uint32(uint16(pH)) << 16))
		writeR := overlaySavedPopupRect
		procWriteConsoleOutputW.Call(
			uintptr(h),
			uintptr(unsafe.Pointer(&popCells[0])),
			pBufSize,
			0,
			uintptr(unsafe.Pointer(&writeR)),
		)
		overlayLastPopupState.selectPos = newPopupState.selectPos
	}

	cmdCells := newOverlayRow(width)
	fillOverlayText(cmdCells, 0, ov.Cmd, overlayAttrNum)
	winWriteOverlayRow(h, left, right, cmdRow, cmdCells)

	if len(ov.Keys) > 0 {
		keyCells := newOverlayRow(width)
		for _, k := range ov.Keys {
			// Each slot knows its own column: slot widths are uneven once the
			// width does not divide by 12, so appending sequentially drifts.
			col := fillOverlayText(keyCells, k.Col, k.Num, overlayAttrNum)
			fillOverlayText(keyCells, col, k.Label, overlayAttrText)
		}
		winWriteOverlayRow(h, left, right, info.Window.Bottom, keyCells)
	}

	cursorX := left + int16(ov.CursorCol)
	if cursorX > right {
		cursorX = right
	}
	procSetConsoleCursorPositionOverlay.Call(uintptr(h), winOverlayCoordArg(cursorX, cmdRow))
	ci := overlayCursorInfo{Size: 25, Visible: 1}
	procSetConsoleCursorInfoOverlay.Call(uintptr(h), uintptr(unsafe.Pointer(&ci)))
}

// winClearConsoleOverlay blanks the reserved rows and restores the cursor the
// child process is expected to continue from.
func winClearConsoleOverlay(n int) {
	if n <= 0 {
		return
	}
	h, ok := winOverlayHandle()
	if !ok {
		return
	}
	info, ok := winOverlayInfo(h)
	if !ok {
		return
	}
	pinOverlayWindow(h, &info.Window)
	if info2, ok2 := winOverlayInfo(h); ok2 {
		info = info2
	}
	left, right := info.Window.Left, info.Window.Right
	width := int(right-left) + 1
	if width <= 0 {
		return
	}
	blank := newOverlayRow(width)
	for i := 0; i < n; i++ {
		row := info.Window.Bottom - int16(n) + 1 + int16(i)
		if row < info.Window.Top {
			continue
		}
		winWriteOverlayRow(h, left, right, row, blank)
	}
	if overlaySavedPopupValid {
		pW := int(overlaySavedPopupRect.Right-overlaySavedPopupRect.Left) + 1
		pH := int(overlaySavedPopupRect.Bottom-overlaySavedPopupRect.Top) + 1
		if pW > 0 && pH > 0 && len(overlaySavedPopupBuf) == pW*pH {
			pBufSize := uintptr(uint32(uint16(pW)) | (uint32(uint16(pH)) << 16))
			r := overlaySavedPopupRect
			procWriteConsoleOutputW.Call(
				uintptr(h),
				uintptr(unsafe.Pointer(&overlaySavedPopupBuf[0])),
				pBufSize,
				0,
				uintptr(unsafe.Pointer(&r)),
			)
		}
		overlaySavedPopupValid = false
	}

	if overlaySavedCursorValid {
		procSetConsoleCursorPositionOverlay.Call(uintptr(h), winOverlayCoordArg(overlaySavedCursor.X, overlaySavedCursor.Y))
		overlaySavedCursorValid = false
	}
}
