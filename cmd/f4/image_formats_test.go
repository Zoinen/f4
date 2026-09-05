package main

import (
	"encoding/binary"
	"testing"
)

// qoiFile builds the stream by hand, one chunk of each kind that matters.
func qoiFile() []byte {
	out := []byte("qoif")
	out = binary.BigEndian.AppendUint32(out, 2)
	out = binary.BigEndian.AppendUint32(out, 2)
	out = append(out, 4, 0)

	out = append(out, qoiOpRGBA, 10, 20, 30, 255) // the first pixel
	out = append(out, qoiOpRun)                   // and the same one again
	out = append(out, qoiOpDiff|(3<<4)|(2<<2)|1)  // +1, 0, -1
	out = append(out, qoiOpIndex|9)               // back to the first colour

	out = append(out, 0, 0, 0, 0, 0, 0, 0, 1)
	return out
}

func TestDecodeQOI(t *testing.T) {
	surf, err := decodeQOI(qoiFile())
	if err != nil {
		t.Fatalf("decoding failed: %v", err)
	}
	if surf.Width != 2 || surf.Height != 2 {
		t.Fatalf("geometry: %dx%d", surf.Width, surf.Height)
	}

	want := [][4]byte{
		{10, 20, 30, 255},
		{10, 20, 30, 255},
		{11, 20, 29, 255},
		{10, 20, 30, 255},
	}
	for i, w := range want {
		r, g, b, a := surf.PixelAt(i%2, i/2)
		if r != w[0] || g != w[1] || b != w[2] || a != w[3] {
			t.Errorf("pixel %d: got %d %d %d %d, want %v", i, r, g, b, a, w)
		}
	}
}

func TestDecodeQOIRejectsRubbish(t *testing.T) {
	if _, err := decodeQOI([]byte("not an image")); err == nil {
		t.Error("a file that is not QOI must be reported as such")
	}

	// A stream that promises four pixels and delivers one.
	short := qoiFile()[:qoiHeaderSize+5]
	if _, err := decodeQOI(short); err == nil {
		t.Error("a truncated stream must be reported as such")
	}
}

// bmpFile assembles a bottom-up image out of already packed rows.
func bmpFile(width, height, bits int, palette [][3]byte, rows [][]byte) []byte {
	info := make([]byte, bmpInfoHeaderSize)
	le := binary.LittleEndian
	le.PutUint32(info[0:4], bmpInfoHeaderSize)
	le.PutUint32(info[4:8], testUint32Int32Bits(width))
	le.PutUint32(info[8:12], testUint32Int32Bits(height))
	le.PutUint16(info[12:14], 1)
	le.PutUint16(info[14:16], testUint16(bits))
	le.PutUint32(info[32:36], testUint32(len(palette)))

	var pal []byte
	for _, c := range palette {
		pal = append(pal, c[2], c[1], c[0], 0)
	}
	offset := bmpFileHeaderSize + bmpInfoHeaderSize + len(pal)

	var pixels []byte
	for _, row := range rows {
		pixels = append(pixels, row...)
	}

	out := []byte{'B', 'M'}
	out = le.AppendUint32(out, testUint32(offset+len(pixels)))
	out = le.AppendUint32(out, 0)
	out = le.AppendUint32(out, testUint32(offset))
	out = append(out, info...)
	out = append(out, pal...)
	return append(out, pixels...)
}

func TestDecodeBMP24(t *testing.T) {
	// Two rows, stored bottom first: the file starts with the lower one.
	lower := []byte{0, 0, 255, 255, 255, 255, 0, 0}
	upper := []byte{255, 0, 0, 0, 255, 0, 0, 0}

	surf, err := decodeBMP(bmpFile(2, 2, 24, nil, [][]byte{lower, upper}))
	if err != nil {
		t.Fatalf("decoding failed: %v", err)
	}
	if r, g, b, a := surf.PixelAt(0, 0); r != 0 || g != 0 || b != 255 || a != 255 {
		t.Errorf("top left: got %d %d %d %d", r, g, b, a)
	}
	if r, g, b, _ := surf.PixelAt(1, 0); r != 0 || g != 255 || b != 0 {
		t.Errorf("top right: got %d %d %d", r, g, b)
	}
	if r, g, b, _ := surf.PixelAt(0, 1); r != 255 || g != 0 || b != 0 {
		t.Errorf("bottom left: got %d %d %d", r, g, b)
	}
	if r, g, b, _ := surf.PixelAt(1, 1); r != 255 || g != 255 || b != 255 {
		t.Errorf("bottom right: got %d %d %d", r, g, b)
	}
}

func TestDecodeBMPTopDownAndPalette(t *testing.T) {
	palette := [][3]byte{{10, 20, 30}, {40, 50, 60}}
	// One row of two pixels, padded to four bytes, stored top down.
	row := []byte{0x00, 0x01, 0, 0}

	surf, err := decodeBMP(bmpFile(2, -1, 8, palette, [][]byte{row}))
	if err != nil {
		t.Fatalf("decoding failed: %v", err)
	}
	if r, g, b, a := surf.PixelAt(0, 0); r != 10 || g != 20 || b != 30 || a != 255 {
		t.Errorf("first pixel: got %d %d %d %d", r, g, b, a)
	}
	if r, g, b, _ := surf.PixelAt(1, 0); r != 40 || g != 50 || b != 60 {
		t.Errorf("second pixel: got %d %d %d", r, g, b)
	}
}

func TestDecodeBMP32TreatsEmptyAlphaAsOpaque(t *testing.T) {
	row := []byte{1, 2, 3, 0, 4, 5, 6, 0}

	surf, err := decodeBMP(bmpFile(2, -1, 32, nil, [][]byte{row}))
	if err != nil {
		t.Fatalf("decoding failed: %v", err)
	}
	if r, g, b, a := surf.PixelAt(0, 0); r != 3 || g != 2 || b != 1 || a != 255 {
		t.Errorf("first pixel: got %d %d %d %d", r, g, b, a)
	}

	// One pixel with an alpha channel is enough to take the file at its word.
	row = []byte{1, 2, 3, 0, 4, 5, 6, 128}
	surf, err = decodeBMP(bmpFile(2, -1, 32, nil, [][]byte{row}))
	if err != nil {
		t.Fatalf("decoding failed: %v", err)
	}
	if _, _, _, a := surf.PixelAt(0, 0); a != 0 {
		t.Errorf("first pixel alpha: got %d, want 0", a)
	}
}

func TestDecodeBMPRejectsRubbish(t *testing.T) {
	if _, err := decodeBMP([]byte("not an image at all, really")); err == nil {
		t.Error("a file that is not BMP must be reported as such")
	}

	truncated := bmpFile(4, -4, 24, nil, [][]byte{{1, 2, 3}})
	if _, err := decodeBMP(truncated); err == nil {
		t.Error("a file promising more rows than it has must be reported as such")
	}
}

func TestDecodeImageFallsBackWhenTheExtensionLies(t *testing.T) {
	// A QOI image saved under a name claiming it is a PNG.
	surf, name, err := DecodeImage("misnamed.png", qoiFile())
	if err != nil {
		t.Fatalf("decoding failed: %v", err)
	}
	if name != "go-qoi" {
		t.Errorf("decoded by %q, expected the QOI decoder", name)
	}
	if surf.Width != 2 {
		t.Errorf("geometry: %dx%d", surf.Width, surf.Height)
	}

	if _, _, err := DecodeImage("whatever.png", []byte("no picture in here")); err == nil {
		t.Error("a file no decoder can read must be reported as such")
	}
}
