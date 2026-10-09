package app

import (
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/vtui"
)

// TestRenderDialogOuterBorderLeavesEmptyRingWhenEnabled covers f4#1399: with
// the setting on, the ring outside a modal dialog's own border is blank
// space -- no second box is drawn over the frame the dialog already has --
// it carries the same colour the dialog's border was just drawn with, and
// the frame's shadow is re-laid outside the ring instead of being wiped
// out along with the panel content the ring blanks.
func TestRenderDialogOuterBorderLeavesEmptyRingWhenEnabled(t *testing.T) {
	orig := config.App.DialogOuterBorder
	t.Cleanup(func() { config.App.DialogOuterBorder = orig })
	config.App.DialogOuterBorder = true

	scr := vtui.NewScreenBuf()
	scr.AllocBuf(40, 20)
	// Panel content sitting behind the dialog, so the test can tell blanked
	// cells, untouched cells and shadowed cells apart.
	panelAttr := vtui.SetRGBBoth(0, 0x00FFFF, 0x0000A0)
	scr.FillRect(0, 0, 39, 19, '.', panelAttr)
	bare := scr.GetCell(0, 0)

	dlg := vtui.NewDialog(5, 5, 20, 12, "Test")
	dlg.Show(scr)
	RenderDialogOuterBorder(scr, dlg)

	x1, y1, x2, y2 := dlg.GetPosition()
	midY := (y1 + y2) / 2
	midX := (x1 + x2) / 2
	// Three cells wide on the sides, one cell tall above and below.
	ring := [][2]int{
		{x1 - 3, y1 - 1}, {x1 - 2, y1 - 1}, {x1 - 1, y1 - 1},
		{x2 + 1, y1 - 1}, {x2 + 2, y1 - 1}, {x2 + 3, y1 - 1},
		{x1 - 3, y2 + 1}, {x1 - 2, y2 + 1}, {x1 - 1, y2 + 1},
		{x2 + 1, y2 + 1}, {x2 + 2, y2 + 1}, {x2 + 3, y2 + 1},
		{midX, y1 - 1}, {midX, y2 + 1},
		{x1 - 3, midY}, {x1 - 2, midY}, {x1 - 1, midY},
		{x2 + 1, midY}, {x2 + 2, midY}, {x2 + 3, midY},
	}
	for _, c := range ring {
		if cell := scr.GetCell(c[0], c[1]); cell.Char != ' ' {
			t.Fatalf("expected empty space at (%d,%d) outside the dialog, got %+v", c[0], c[1], cell)
		}
	}

	// The frame the dialog already owns must survive: only the ring outside
	// it is touched.
	for _, c := range [][2]int{{x1, y1}, {x2, y1}, {x1, y2}, {x2, y2}} {
		if cell := scr.GetCell(c[0], c[1]); cell.Char == ' ' || cell.Char == 0 {
			t.Fatalf("the dialog's own border was erased at (%d,%d): %+v", c[0], c[1], cell)
		}
	}

	// The ring must reuse the colour the dialog's own border was drawn
	// with, not some unrelated palette entry.
	wantAttr := scr.GetCell(x1, y1).Attributes
	if got := scr.GetCell(x1-3, y1-1).Attributes; got != wantAttr {
		t.Fatalf("outer border attr = %x, want %x (dialog's own border colour)", got, wantAttr)
	}

	// It stops at that ring: the panel just outside it keeps its content
	// and its colour, and the top/bottom stay one cell tall.
	for _, c := range [][2]int{{x1 - 4, midY}, {midX, y1 - 2}, {x1 - 3, y1 - 2}} {
		if cell := scr.GetCell(c[0], c[1]); cell != bare {
			t.Fatalf("outer border painted past its ring at (%d,%d): %+v, want %+v", c[0], c[1], cell, bare)
		}
	}

	// The shadow the frame manager laid before Show fell inside the ring and
	// was blanked with it, so it has to be re-laid just outside: same text,
	// darkened colour, one row below and two columns right of the ring.
	for _, c := range [][2]int{{x2 + 4, midY}, {x2 + 5, midY}, {midX, y2 + 2}} {
		cell := scr.GetCell(c[0], c[1])
		if cell.Char != '.' {
			t.Fatalf("shadow erased the panel content at (%d,%d): %+v", c[0], c[1], cell)
		}
		if cell.Attributes == bare.Attributes {
			t.Fatalf("no shadow outside the ring at (%d,%d): %+v", c[0], c[1], cell)
		}
	}
}

// TestRenderDialogOuterBorderKeepsColourWhenDraggedOffScreen covers the
// mouse-drag case: MoveRelative does not clamp, so a dialog dragged past
// the left or the top edge of the window has its top-left corner off the
// screen, GetCell answers that with a zero cell, and the ring used to come
// out in the wrong colour.
func TestRenderDialogOuterBorderKeepsColourWhenDraggedOffScreen(t *testing.T) {
	orig := config.App.DialogOuterBorder
	t.Cleanup(func() { config.App.DialogOuterBorder = orig })
	config.App.DialogOuterBorder = true

	for _, tc := range []struct {
		name   string
		dx, dy int
		// border is a cell of the dialog's own border that stayed on
		// screen; ring sits just outside it.
		border [2]int
		ring   [2]int
	}{
		{name: "left", dx: -8, dy: 0, border: [2]int{0, 5}, ring: [2]int{0, 4}},
		{name: "top", dx: 0, dy: -6, border: [2]int{5, 0}, ring: [2]int{4, 0}},
		{name: "corner", dx: -8, dy: -6, border: [2]int{0, 6}, ring: [2]int{0, 7}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scr := vtui.NewScreenBuf()
			scr.AllocBuf(40, 20)

			dlg := vtui.NewDialog(5, 5, 20, 12, "Test")
			dlg.MoveRelative(tc.dx, tc.dy)
			dlg.Show(scr)
			RenderDialogOuterBorder(scr, dlg)

			frame := scr.GetCell(tc.border[0], tc.border[1])
			if frame.Char == ' ' || frame.Char == 0 {
				t.Fatalf("no dialog border at (%d,%d): %+v", tc.border[0], tc.border[1], frame)
			}
			if frame.Attributes == 0 {
				t.Fatalf("dialog border colour at (%d,%d) is zero, the ring check would be vacuous", tc.border[0], tc.border[1])
			}

			cell := scr.GetCell(tc.ring[0], tc.ring[1])
			if cell.Char != ' ' {
				t.Fatalf("expected empty ring at (%d,%d), got %+v", tc.ring[0], tc.ring[1], cell)
			}
			if cell.Attributes != frame.Attributes {
				t.Fatalf("ring colour at (%d,%d) = %x, want the dialog border colour %x",
					tc.ring[0], tc.ring[1], cell.Attributes, frame.Attributes)
			}
		})
	}
}

// TestRenderDialogOuterBorderDefaultOff covers the "must not change default
// behaviour" requirement: with the setting left at its zero value (as
// config.Default has it), nothing is painted outside the dialog's own
// border.
func TestRenderDialogOuterBorderDefaultOff(t *testing.T) {
	orig := config.App.DialogOuterBorder
	t.Cleanup(func() { config.App.DialogOuterBorder = orig })
	config.App.DialogOuterBorder = false

	scr := vtui.NewScreenBuf()
	scr.AllocBuf(40, 20)

	dlg := vtui.NewDialog(5, 5, 20, 12, "Test")
	dlg.Show(scr)
	RenderDialogOuterBorder(scr, dlg)

	x1, y1, x2, y2 := dlg.GetPosition()
	for _, c := range [][2]int{{x1 - 1, y1 - 1}, {x2 + 1, y2 + 1}} {
		if cell := scr.GetCell(c[0], c[1]); cell.Char != 0 {
			t.Fatalf("outer border drawn while DialogOuterBorder is off: %+v at (%d,%d)", cell, c[0], c[1])
		}
	}
}

// TestRenderDialogOuterBorderUserMenuOnlyNotPlainMenus covers the other half
// of f4#1399: the empty ring also wraps the user menu, but not an ordinary
// dropdown/context menu built from the same vtui.VMenu.
func TestRenderDialogOuterBorderUserMenuOnlyNotPlainMenus(t *testing.T) {
	orig := config.App.DialogOuterBorder
	t.Cleanup(func() { config.App.DialogOuterBorder = orig })
	config.App.DialogOuterBorder = true

	scr := vtui.NewScreenBuf()
	scr.AllocBuf(40, 20)
	menu := vtui.NewVMenu("User menu")
	menu.SetPosition(5, 5, 20, 12)
	frame := &panel.UserMenuFrame{VMenu: menu}
	frame.Show(scr)
	RenderDialogOuterBorder(scr, frame)
	if cell := scr.GetCell(4, 4); cell.Char != ' ' {
		t.Fatalf("expected an empty outer ring around the user menu, got %+v", cell)
	}

	scr2 := vtui.NewScreenBuf()
	scr2.AllocBuf(40, 20)
	plain := vtui.NewVMenu("Commands")
	plain.SetPosition(5, 5, 20, 12)
	plain.Show(scr2)
	RenderDialogOuterBorder(scr2, plain)
	if cell := scr2.GetCell(4, 4); cell.Char != 0 {
		t.Fatalf("outer border drawn around an ordinary menu: %+v", cell)
	}
}
