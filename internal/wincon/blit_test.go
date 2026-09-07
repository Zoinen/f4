package wincon

import "testing"

// The frame buffer is bottom-up with its channels the other way round, and it
// is also where a picture that arrives in overlapping pieces is composed. Both
// mistakes -- the flip and the channel order -- look like a picture rather
// than an error, and the composing one looks like the top layer alone.

func TestBlitIntoWritesBottomUpBGRA(t *testing.T) {
	const w, h = 1, 2
	dst := make([]byte, w*h*4)
	// One red pixel at the top row of a two row buffer.
	src := []byte{255, 0, 0, 255}
	blitInto(dst, w, h, src, 1, 1, 4, 0, 0)

	// Top row of the picture is the last row of the buffer.
	bottom := dst[4:8]
	if bottom[0] != 0 || bottom[1] != 0 || bottom[2] != 255 || bottom[3] != 255 {
		t.Errorf("got %v, want red as BGRA in the last row", bottom)
	}
	if dst[0] != 0 || dst[1] != 0 || dst[2] != 0 {
		t.Errorf("the other row was written: %v", dst[0:4])
	}
}

func TestBlitIntoComposesLayers(t *testing.T) {
	const w, h = 2, 1
	dst := make([]byte, w*h*4)
	// The layer underneath: both pixels blue.
	blitInto(dst, w, h, []byte{
		0, 0, 255, 255,
		0, 0, 255, 255,
	}, w, h, w*4, 0, 0)
	// The layer on top repaints one pixel and leaves the other alone.
	blitInto(dst, w, h, []byte{
		0, 255, 0, 255,
		0, 0, 0, 0,
	}, w, h, w*4, 0, 0)

	// BGRA, and the row is the only row.
	if dst[0] != 0 || dst[1] != 255 || dst[2] != 0 {
		t.Errorf("the top layer did not land: %v", dst[0:4])
	}
	if dst[4] != 255 || dst[6] != 0 {
		t.Errorf("the top layer erased what it left transparent: %v", dst[4:8])
	}
}
