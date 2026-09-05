package main

import (
	"strings"
	"testing"
)

// The receiving half of full colour over sixel.
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
	sb.WriteString(itoa(w))
	sb.WriteByte(';')
	sb.WriteString(itoa(h))
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
		sb.WriteString("!" + itoa(w/2) + painted)
		sb.WriteString("!" + itoa(w-w/2) + blank)
	}
	return sb.String()
}

func TestSixelLayersStackAtTheSameCell(t *testing.T) {
	e := newSixelEnv(t)

	// What the layered encoder writes: the cursor is restated before each
	// image, because a sixel dump leaves it at the sixel active position.
	e.tv.SetCursor(4, 6)
	e.send("0;1;0", sixelHalfBody(30, 40, ";2;100;0;0", true))
	e.tv.SetCursor(4, 6)
	e.send("0;1;0", sixelHalfBody(30, 40, ";2;0;100;0", false))

	if len(e.tv.images) != 2 {
		t.Fatalf("got %d placement(s), want both layers kept", len(e.tv.images))
	}
	for i, p := range e.tv.images {
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

	if len(e.tv.images) != 1 {
		t.Fatalf("got %d placement(s), want one", len(e.tv.images))
	}
	surf := e.tv.images[0].Surface
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

// The composing itself: source over destination, so a transparent pixel keeps
// what is under it and an opaque one replaces it.
func TestBlitIntoComposes(t *testing.T) {
	const w, h = 2, 1
	dst := []byte{
		0, 0, 255, 255, // blue, opaque
		0, 0, 255, 255,
	}
	src := []byte{
		255, 0, 0, 0, // red, fully transparent
		255, 0, 0, 255, // red, opaque
	}
	blitInto(dst, w, h, src, w, h, w*4, 0, 0)

	if dst[0] != 0 || dst[2] != 255 {
		t.Errorf("a transparent pixel overwrote the one under it: %v", dst[0:4])
	}
	if dst[4] != 255 || dst[6] != 0 {
		t.Errorf("an opaque pixel did not land: %v", dst[4:8])
	}
}

func TestBlitIntoBlendsHalfTransparency(t *testing.T) {
	dst := []byte{0, 0, 0, 255}
	src := []byte{255, 255, 255, 128}
	blitInto(dst, 1, 1, src, 1, 1, 4, 0, 0)

	// 255*128/255 rounded is 128, and a stack of these must not drift: the
	// rounding is what keeps ten layers of white from arriving grey.
	if dst[0] != 128 || dst[1] != 128 || dst[2] != 128 {
		t.Errorf("got %v, want a half blend", dst[0:3])
	}
	if dst[3] != 255 {
		t.Errorf("alpha came out %d, want the destination to stay opaque", dst[3])
	}
}
