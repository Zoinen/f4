package main

import "testing"

var sixelBlack = [3]byte{0, 0, 0}

// sixelPix reads one pixel of a decoded image as r, g, b, a.
func sixelPix(t *testing.T, img *sixelImage, x, y int) (byte, byte, byte, byte) {
	t.Helper()
	return img.Surface.PixelAt(x, y)
}

func decodeOrFail(t *testing.T, params []int, body string) *sixelImage {
	t.Helper()
	img, err := decodeSixel(params, body, sixelBlack)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return img
}

func TestSixelRasterAttributesSizeTheImage(t *testing.T) {
	// One band of two red columns inside a declared 4x6 raster.
	img := decodeOrFail(t, []int{0, 1, 0}, `"1;1;4;6#0;2;100;0;0@@`)
	if img.Surface.Width != 4 || img.Surface.Height != 6 {
		t.Fatalf("size: got %dx%d, want 4x6", img.Surface.Width, img.Surface.Height)
	}
	if r, g, b, a := sixelPix(t, img, 0, 0); r != 255 || g != 0 || b != 0 || a != 255 {
		t.Errorf("first pixel: got %d,%d,%d,%d", r, g, b, a)
	}
	if _, _, _, a := sixelPix(t, img, 2, 0); a != 0 {
		t.Error("a pixel the image never painted must stay transparent under P2=1")
	}
	// Only the low bit was set, so nothing below the first row.
	if _, _, _, a := sixelPix(t, img, 0, 1); a != 0 {
		t.Error("only the first sixel row was set")
	}
}

func TestSixelGrowsWithoutRasterAttributes(t *testing.T) {
	// No raster attributes: the extent comes from the data. Two bands of
	// three columns, and the aspect ratio of P1=7 is one to one.
	img := decodeOrFail(t, []int{7, 1, 0}, `#0;2;0;100;0~~~-~~~`)
	if img.Surface.Width != 3 || img.Surface.Height != 12 {
		t.Fatalf("size: got %dx%d, want 3x12", img.Surface.Width, img.Surface.Height)
	}
	if _, g, _, a := sixelPix(t, img, 2, 11); g != 255 || a != 255 {
		t.Error("the last pixel of the second band must be painted")
	}
}

func TestSixelDefaultAspectRatioIsTwoToOne(t *testing.T) {
	// P1 defaults to a two to one pixel aspect ratio, so one band of six
	// sixel rows covers twelve pixel rows.
	img := decodeOrFail(t, nil, `#0;2;0;0;100~`)
	if img.Surface.Height != 12 {
		t.Fatalf("height: got %d, want 12", img.Surface.Height)
	}
	if _, _, b, a := sixelPix(t, img, 0, 1); b != 255 || a != 255 {
		t.Error("each sixel row covers two pixel rows")
	}
}

func TestSixelRepeatAndCarriageReturn(t *testing.T) {
	// !5? advances five columns without painting, then $ returns to the
	// left margin so the second colour overwrites the first.
	img := decodeOrFail(t, []int{7, 1, 0}, `"1;1;5;6#1;2;100;0;0!5@$#2;2;0;0;100@@`)
	if img.Surface.Width != 5 {
		t.Fatalf("width: got %d, want 5", img.Surface.Width)
	}
	if r, _, b, _ := sixelPix(t, img, 0, 0); r != 0 || b != 255 {
		t.Errorf("the second pass must overwrite the first: got r=%d b=%d", r, b)
	}
	if r, _, b, _ := sixelPix(t, img, 4, 0); r != 255 || b != 0 {
		t.Errorf("the tail of the first run must survive: got r=%d b=%d", r, b)
	}
}

func TestSixelOpaqueBackgroundFill(t *testing.T) {
	// P2 of 0 means the pixels the image does not paint take the current
	// background colour, which the terminal passes in.
	img, err := decodeSixel([]int{7, 0, 0}, `"1;1;2;6#0;2;100;100;100@`, [3]byte{10, 20, 30})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if r, g, b, a := sixelPix(t, img, 1, 0); r != 10 || g != 20 || b != 30 || a != 255 {
		t.Errorf("background: got %d,%d,%d,%d", r, g, b, a)
	}
	if !img.Surface.Opaque {
		t.Error("an image with no transparency must be flagged opaque")
	}
}

// A register redefined between bands paints the bands in different colours.
// This is what lets an encoder with 256 registers draw an unlimited number of
// colours, and it is the whole of what "full colour sixel" means.
func TestSixelRegisterRedefinitionIsImmediate(t *testing.T) {
	img := decodeOrFail(t, []int{7, 1, 0}, `"1;1;1;12#9;2;100;0;0~-#9;2;0;0;100~`)
	if r, _, b, _ := sixelPix(t, img, 0, 0); r != 255 || b != 0 {
		t.Errorf("first band: got r=%d b=%d", r, b)
	}
	if r, _, b, _ := sixelPix(t, img, 0, 6); r != 0 || b != 255 {
		t.Errorf("second band: got r=%d b=%d", r, b)
	}
}

func TestSixelHighRegisterNumber(t *testing.T) {
	img := decodeOrFail(t, []int{7, 1, 0}, `"1;1;1;6#60000;2;0;100;0~`)
	if _, g, _, _ := sixelPix(t, img, 0, 0); g != 255 {
		t.Error("a register far above the sixteen of the hardware must work")
	}
}

func TestSixelDefaultPalette(t *testing.T) {
	// Register 1 of the default map is the VT340 blue, 20/20/80 percent.
	img := decodeOrFail(t, []int{7, 1, 0}, `"1;1;1;6#1~`)
	r, g, b, _ := sixelPix(t, img, 0, 0)
	if r != 51 || g != 51 || b != 204 {
		t.Errorf("default register 1: got %d,%d,%d, want 51,51,204", r, g, b)
	}
}

func TestSixelHLSColour(t *testing.T) {
	// In DEC's HLS space hue 120 is red and hue 0 is blue.
	img := decodeOrFail(t, []int{7, 1, 0}, `"1;1;2;6#0;1;120;50;100@#1;1;0;50;100@`)
	if r, g, b, _ := sixelPix(t, img, 0, 0); r != 255 || g != 0 || b != 0 {
		t.Errorf("hue 120 should be red: got %d,%d,%d", r, g, b)
	}
	if r, g, b, _ := sixelPix(t, img, 1, 0); r != 0 || g != 0 || b != 255 {
		t.Errorf("hue 0 should be blue: got %d,%d,%d", r, g, b)
	}
}

// The final sixel position drives where the text cursor lands, so it has to
// count the graphics new lines that follow the data.
func TestSixelEndPositionCountsTrailingNewLines(t *testing.T) {
	img := decodeOrFail(t, []int{7, 1, 0}, `"1;1;1;6#0;2;100;100;100~`)
	if img.EndY != 0 {
		t.Errorf("data alone must leave the position in the first band: got %d", img.EndY)
	}
	img = decodeOrFail(t, []int{7, 1, 0}, `"1;1;1;6#0;2;100;100;100~--`)
	if img.EndY != 12 {
		t.Errorf("two graphics new lines: got %d, want 12", img.EndY)
	}
}

func TestSixelWhitespaceIsIgnored(t *testing.T) {
	img := decodeOrFail(t, []int{7, 1, 0}, "\"1;1;2;6#0;2;100;100;100@\r\n@")
	if img.Surface.Width != 2 {
		t.Fatalf("width: got %d, want 2", img.Surface.Width)
	}
	if _, _, _, a := sixelPix(t, img, 1, 0); a != 255 {
		t.Error("a line break inside the data is not data")
	}
}

func TestSixelEmptyBodyIsRejected(t *testing.T) {
	if _, err := decodeSixel(nil, "", sixelBlack); err == nil {
		t.Error("an empty sequence has no picture in it")
	}
}

func TestSixelOversizedRasterIsRejected(t *testing.T) {
	if _, err := decodeSixel(nil, `"1;1;60000;60000~`, sixelBlack); err == nil {
		t.Error("a raster past the pixel budget must be refused")
	}
}

func TestSixelTruncatesToDeclaredRaster(t *testing.T) {
	// The client declared two columns and then wrote four. The extra data
	// is dropped rather than growing the picture, which is what keeps a
	// padded last band from making the image taller than advertised.
	img := decodeOrFail(t, []int{7, 1, 0}, `"1;1;2;6#0;2;100;100;100~~~~`)
	if img.Surface.Width != 2 {
		t.Fatalf("width: got %d, want 2", img.Surface.Width)
	}
}
