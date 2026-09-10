package media

import (
	"testing"
)

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
