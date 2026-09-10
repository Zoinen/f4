package terminal

import (
	"strconv"
	"strings"
	"testing"
)

// The receiving half of Full colour over sixel.
//
// A picture with more colours than 256 registers reaches a terminal one of two
// ways. Either the sender redefines a register between bands, which
// TestSixelRegisterRedefinitionIsImmediate covers, or it sends the picture
// several times at the same cell with P2=1 and a palette each, and the
// terminal composes them. f4 has to take both: the second is what vtui sends
// to Windows Terminal, so f4 running inside f4 there is a stack of layers
// arriving at its own built-in terminal.

// sixelHalfBody paints the left half of a w by h rectangle in one colour and
// leaves the right half untouched, which under P2=1 is transparent.
func sixelHalfBody(w, h int, colour string, left bool) string {
	var sb strings.Builder
	sb.WriteString(`"1;1;`)
	sb.WriteString(strconv.Itoa(w))
	sb.WriteByte(';')
	sb.WriteString(strconv.Itoa(h))
	sb.WriteString("#0")
	sb.WriteString(colour)
	for band := 0; band < (h+5)/6; band++ {
		if band > 0 {
			sb.WriteByte('-')
		}
		sb.WriteString("#0")
		painted, blank := "~", "?"
		if !left {
			painted, blank = "?", "~"
		}
		sb.WriteString("!" + strconv.Itoa(w/2) + painted)
		sb.WriteString("!" + strconv.Itoa(w-w/2) + blank)
	}
	return sb.String()
}

func TestSixelLayersStackAtTheSameCell(t *testing.T) {
	e := newSixelEnv(t)

	// What the layered encoder writes: the Cursor is restated before each
	// image, because a sixel dump leaves it at the sixel active position.
	e.tv.SetCursor(4, 6)
	e.send("0;1;0", sixelHalfBody(30, 40, ";2;100;0;0", true))
	e.tv.SetCursor(4, 6)
	e.send("0;1;0", sixelHalfBody(30, 40, ";2;0;100;0", false))

	if len(e.tv.Images) != 2 {
		t.Fatalf("got %d placement(s), want both layers kept", len(e.tv.Images))
	}
	for i, p := range e.tv.Images {
		if p.Col != 4 || p.Row != 6 {
			t.Errorf("layer %d at %d,%d, want both at 4,6", i, p.Col, p.Row)
		}
		if !p.Sixel {
			t.Errorf("layer %d is not marked as a sixel placement", i)
		}
		if p.Surface.Opaque {
			t.Errorf("layer %d lost its transparency on the way to the placement", i)
		}
	}
}

// A layer must not erase the one under it. The decoder leaves the pixels an
// image does not paint at zero alpha, and everything that composes placements
// has to honour that rather than copy the bytes across.
func TestSixelLayerLeavesUnpaintedPixelsAlone(t *testing.T) {
	e := newSixelEnv(t)
	e.tv.SetCursor(0, 0)
	e.send("0;1;0", sixelHalfBody(30, 6, ";2;0;100;0", false))

	if len(e.tv.Images) != 1 {
		t.Fatalf("got %d placement(s), want one", len(e.tv.Images))
	}
	surf := e.tv.Images[0].Surface
	if surf.Opaque {
		t.Fatal("a P2=1 image came out opaque")
	}
	if _, _, _, a := surf.PixelAt(0, 0); a != 0 {
		t.Errorf("the unpainted half has alpha %d, want 0", a)
	}
	if _, g, _, a := surf.PixelAt(surf.Width-1, 0); a != 0xFF || g == 0 {
		t.Errorf("the painted half came out as alpha=%d g=%d", a, g)
	}
}
