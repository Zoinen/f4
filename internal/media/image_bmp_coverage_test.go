package media

// More of decodeBMP's own branches: the ones TestDecodeBMP24 and its
// neighbours in image_formats_test.go do not reach — the geometry and
// compression guards, the palette that has to be filled in by hand, the
// palette that is not actually there, and the 16, 4 and 1 bit pixel formats,
// which each unpack differently from the 8/24/32 bit cases already covered.

import (
	"encoding/binary"
	"testing"
)

// A declared header shorter than BITMAPINFOHEADER is not a BMP this decoder
// understands, even though the file itself is long enough to hold one.
func TestDecodeBMPRejectsShortInfoHeader(t *testing.T) {
	data := make([]byte, bmpFileHeaderSize+bmpInfoHeaderSize)
	data[0], data[1] = 'B', 'M'
	binary.LittleEndian.PutUint32(data[14:18], bmpInfoHeaderSize-1)
	if _, err := decodeBMP(data); err == nil {
		t.Error("a header shorter than BITMAPINFOHEADER must be rejected")
	}
}

// Zero in either dimension is not a picture, whichever one it is.
func TestDecodeBMPRejectsEmptyGeometry(t *testing.T) {
	if _, err := decodeBMP(bmpFile(0, 2, 24, nil, nil)); err == nil {
		t.Error("zero width must be rejected")
	}
	if _, err := decodeBMP(bmpFile(2, 0, 24, nil, nil)); err == nil {
		t.Error("zero height must be rejected")
	}
}

// A geometry over the shared pixel-count ceiling is refused before any pixel
// buffer is allocated for it.
func TestDecodeBMPRejectsImageLargerThanTheLimit(t *testing.T) {
	if _, err := decodeBMP(bmpFile(imageMaxPixels+1, 1, 24, nil, nil)); err == nil {
		t.Error("an image over the shared pixel limit must be rejected")
	}
}

// Only uncompressed BMPs are supported; RLE and the other compression codes
// are refused rather than misread as raw pixels.
func TestDecodeBMPRejectsCompression(t *testing.T) {
	row := make([]byte, 8) // 2 pixels * 3 bytes, padded to the 4 byte stride
	data := bmpFile(2, 2, 24, nil, [][]byte{row, row})
	binary.LittleEndian.PutUint32(data[30:34], 1) // any non-zero compression code
	if _, err := decodeBMP(data); err == nil {
		t.Error("a compressed BMP must be rejected")
	}
}

// A colour depth outside the six this decoder knows is refused by name
// rather than silently misread.
func TestDecodeBMPRejectsUnsupportedBitDepth(t *testing.T) {
	if _, err := decodeBMP(bmpFile(2, 2, 7, nil, nil)); err == nil {
		t.Error("an unsupported colour depth must be rejected")
	}
}

// An indexed image that declares no palette length at all must still get
// one: the format implies a full 256 entry table for 8 bits per pixel. A
// file that then turns out too short to actually hold those entries is a
// truncated file, not a valid image with a short palette.
func TestDecodeBMPRejectsTruncatedDefaultPalette(t *testing.T) {
	data := bmpFile(2, -1, 8, nil, [][]byte{{0, 1, 0, 0}})
	if _, err := decodeBMP(data); err == nil {
		t.Error("a defaulted palette the file has no room for must be rejected")
	}
}

// 16 bit BMPs (the untested case among the pixel formats) use 5 bits per
// channel, most significant bits first, with no alpha.
func TestDecodeBMP16BitFivePerChannel(t *testing.T) {
	// 0x7C00 is pure red (R=31,G=0,B=0) and 0x03E0 is pure green (R=0,G=31,
	// B=0) in that layout, both stored little endian.
	row := []byte{0x00, 0x7C, 0xE0, 0x03}
	surf, err := decodeBMP(bmpFile(2, -1, 16, nil, [][]byte{row}))
	if err != nil {
		t.Fatalf("decoding failed: %v", err)
	}
	if r, g, b, a := surf.PixelAt(0, 0); r != 255 || g != 0 || b != 0 || a != 255 {
		t.Errorf("first pixel: got %d %d %d %d", r, g, b, a)
	}
	if r, g, b, _ := surf.PixelAt(1, 0); r != 0 || g != 255 || b != 0 {
		t.Errorf("second pixel: got %d %d %d", r, g, b)
	}
}

// 4 bit BMPs pack two palette indices per byte, high nibble first.
func TestDecodeBMP4BitPalette(t *testing.T) {
	palette := [][3]byte{{10, 20, 30}, {40, 50, 60}, {70, 80, 90}}
	// Byte 0 packs pixel 0 (high nibble, index 1) and pixel 1 (low nibble,
	// index 2); byte 1 holds pixel 2 alone (high nibble, index 0), and the
	// row is padded out to its 4 byte stride.
	row := []byte{0x12, 0x00, 0x00, 0x00}
	surf, err := decodeBMP(bmpFile(3, -1, 4, palette, [][]byte{row}))
	if err != nil {
		t.Fatalf("decoding failed: %v", err)
	}
	if r, g, b, _ := surf.PixelAt(0, 0); r != 40 || g != 50 || b != 60 {
		t.Errorf("first pixel (index 1): got %d %d %d", r, g, b)
	}
	if r, g, b, _ := surf.PixelAt(1, 0); r != 70 || g != 80 || b != 90 {
		t.Errorf("second pixel (index 2): got %d %d %d", r, g, b)
	}
	if r, g, b, _ := surf.PixelAt(2, 0); r != 10 || g != 20 || b != 30 {
		t.Errorf("third pixel (index 0): got %d %d %d", r, g, b)
	}
}

// 1 bit BMPs pack eight palette indices per byte, most significant bit
// first.
func TestDecodeBMP1BitPalette(t *testing.T) {
	palette := [][3]byte{{0, 0, 0}, {255, 255, 255}}
	// 0xB2 is 1011 0010: white, black, white, white, black, black, white,
	// black, reading the byte from its most significant bit.
	row := []byte{0xB2, 0x00, 0x00, 0x00}
	surf, err := decodeBMP(bmpFile(8, -1, 1, palette, [][]byte{row}))
	if err != nil {
		t.Fatalf("decoding failed: %v", err)
	}
	want := []bool{true, false, true, true, false, false, true, false} // true = white
	for x, white := range want {
		r, g, b, _ := surf.PixelAt(x, 0)
		gotWhite := r == 255 && g == 255 && b == 255
		if gotWhite != white {
			t.Errorf("pixel %d: got %d %d %d, want white=%v", x, r, g, b, white)
		}
	}
}

// bmpScale5 stretches a five bit channel over a whole byte, and clamps
// anything wider than five bits rather than overflowing past it.
func TestBmpScale5(t *testing.T) {
	cases := []struct {
		in   uint16
		want byte
	}{
		{0, 0},
		{1, 8},
		{15, 123},
		{16, 132},
		{31, 255},
		{40, 255}, // out of range, clamped to the maximum five bit value
	}
	for _, c := range cases {
		if got := bmpScale5(c.in); got != c.want {
			t.Errorf("bmpScale5(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

// An index outside the palette is treated as opaque black instead of
// panicking: a truncated or corrupt palette must not crash the reader.
func TestBmpPaletteAtOutOfRange(t *testing.T) {
	palette := []qoiPixel{{r: 1, g: 2, b: 3, a: 4}}
	opaqueBlack := qoiPixel{a: 0xFF}
	if got := bmpPaletteAt(palette, -1); got != opaqueBlack {
		t.Errorf("negative index: got %+v, want %+v", got, opaqueBlack)
	}
	if got := bmpPaletteAt(palette, 1); got != opaqueBlack {
		t.Errorf("index past the end: got %+v, want %+v", got, opaqueBlack)
	}
	if got := bmpPaletteAt(palette, 0); got != palette[0] {
		t.Errorf("in range index: got %+v, want %+v", got, palette[0])
	}
}
