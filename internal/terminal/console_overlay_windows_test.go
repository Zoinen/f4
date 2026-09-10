//go:build windows

package terminal

import (
	"github.com/unxed/vtui"
	"syscall"
	"testing"
)

func TestWinConsoleOverlayHelpers(t *testing.T) {
	if !WinConsoleOverlayAvailable() {
		t.Fatal("WinConsoleOverlayAvailable() = false, want true")
	}

	for _, tc := range []struct {
		name string
		x, y int16
		want uintptr
	}{
		{name: "zero", want: 0},
		{name: "positive", x: 0x1234, y: 0x5678, want: 0x56781234},
		{name: "negative", x: -1, y: -2, want: 0xfffeffff},
	} {
		t.Run("coordinate/"+tc.name, func(t *testing.T) {
			if got := winOverlayCoordArg(tc.x, tc.y); got != tc.want {
				t.Fatalf("winOverlayCoordArg(%d, %d) = %#x, want %#x", tc.x, tc.y, got, tc.want)
			}
		})
	}

	t.Run("row", func(t *testing.T) {
		if got := newOverlayRow(0); len(got) != 0 {
			t.Fatalf("newOverlayRow(0) length = %d, want 0", len(got))
		}
		got := newOverlayRow(3)
		for i, cell := range got {
			if cell.UnicodeChar != ' ' || cell.Attributes != overlayAttrNum {
				t.Errorf("cell %d = %#v, want space with overlayAttrNum", i, cell)
			}
		}
	})

	t.Run("text", func(t *testing.T) {
		row := newOverlayRow(5)
		attr := uint16(0x1234)
		if got := fillOverlayText(row, 1, "A😀Б?", attr); got != 5 {
			t.Fatalf("fillOverlayText next column = %d, want 5", got)
		}
		want := []simpleCharInfo{
			{UnicodeChar: ' ', Attributes: overlayAttrNum},
			{UnicodeChar: 'A', Attributes: attr},
			{UnicodeChar: '?', Attributes: attr},
			{UnicodeChar: 'Б', Attributes: attr},
			{UnicodeChar: '?', Attributes: attr},
		}
		for i := range row {
			if row[i] != want[i] {
				t.Errorf("cell %d = %#v, want %#v", i, row[i], want[i])
			}
		}

		if got := fillOverlayText(row[:3], 2, "xy", attr); got != 3 {
			t.Fatalf("clipped fill next column = %d, want 3", got)
		}
		if got := fillOverlayText(row[:3], 4, "x", attr); got != 4 {
			t.Fatalf("out-of-range fill next column = %d, want 4", got)
		}
	})

	t.Run("popup colors", func(t *testing.T) {
		frame, text, selected := popupColors()
		wantFrame := vtui.AttrToWin32Attr(vtui.Palette[vtui.ColDialogBox], nil)
		wantText := vtui.AttrToWin32Attr(vtui.Palette[vtui.ColDialogText], nil)
		wantSelected := vtui.AttrToWin32Attr(vtui.Palette[vtui.ColDialogSelectedButton], nil)
		if frame != wantFrame || text != wantText || selected != wantSelected {
			t.Fatalf("popupColors() = %#x, %#x, %#x; want %#x, %#x, %#x", frame, text, selected, wantFrame, wantText, wantSelected)
		}
	})

	t.Run("invalid handle calls", func(t *testing.T) {
		if _, ok := winOverlayInfo(syscall.InvalidHandle); ok {
			t.Fatal("winOverlayInfo(InvalidHandle) = ok, want false")
		}
		winWriteOverlayRow(syscall.InvalidHandle, 0, 0, 0, nil)
		rect := simpleSmallRect{Left: 1, Top: 2, Right: 3, Bottom: 4}
		pinOverlayWindow(syscall.InvalidHandle, &rect)
	})

	t.Run("empty overlay calls", func(t *testing.T) {
		WinDrawConsoleOverlay(ConsoleOverlayContent{})
		WinClearConsoleOverlay(0)
	})
}
