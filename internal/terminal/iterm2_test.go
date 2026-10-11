package terminal

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func iterm2PNG(t *testing.T, w, h int) []byte {
	t.Helper()
	src := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			src.Set(x, y, color.NRGBA{R: 200, G: 40, B: 40, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// send feeds one OSC 1337 File sequence to the parser.
func (e *sixelEnv) sendITerm2(args string, data []byte) {
	e.p.Process([]byte("\x1b]1337;File=" + args + ":" + base64.StdEncoding.EncodeToString(data) + "\x07"))
}

func TestITerm2InlinePictureIsPlacedAtTheCursor(t *testing.T) {
	e := newSixelEnv(t) // 10x20 pixel cells
	e.tv.CursorX, e.tv.CursorY = 3, 2
	e.sendITerm2("inline=1", iterm2PNG(t, 40, 40))

	if len(e.tv.Images) != 1 {
		t.Fatalf("got %d placements, want 1", len(e.tv.Images))
	}
	img := e.tv.Images[0]
	if img.Col != 3 || img.Row != 2 {
		t.Errorf("placed at %d,%d, want 3,2", img.Col, img.Row)
	}
	if img.Cols != 4 || img.Rows != 2 {
		t.Errorf("spans %dx%d cells, want 4x2 for 40x40 pixels", img.Cols, img.Rows)
	}
	if !img.Sixel {
		t.Error("the picture must be kept out of the kitty delete-by-id commands")
	}
	if e.tv.CursorY != 3 || e.tv.CursorX != 3 {
		t.Errorf("cursor at %d,%d, want the picture's last row (3,3)", e.tv.CursorX, e.tv.CursorY)
	}
}

func TestITerm2SizeArguments(t *testing.T) {
	cases := []struct {
		args       string
		cols, rows int
	}{
		{"inline=1;width=10;height=5;preserveAspectRatio=0", 10, 5},
		{"inline=1;width=10", 10, 5},                 // 40x40 picture: height follows the aspect ratio
		{"inline=1;height=3", 6, 3},                  // width follows
		{"inline=1;width=100px;height=100px", 10, 5}, // pixels to cells at 10x20
		{"inline=1;width=50%;height=50%", 40, 12},    // percent of an 80x24 terminal, fitted below
		{"inline=1;width=auto;height=auto", 4, 2},    // natural size
		{"inline=1;width=20;height=2", 4, 2},         // aspect ratio kept: the limiting side is the height
		{"inline=1;width=20;height=2;preserveAspectRatio=0", 20, 2},
	}
	for _, tc := range cases {
		e := newSixelEnv(t)
		e.sendITerm2(tc.args, iterm2PNG(t, 40, 40))
		if len(e.tv.Images) != 1 {
			t.Fatalf("%s: got %d placements", tc.args, len(e.tv.Images))
		}
		img := e.tv.Images[0]
		if tc.args == "inline=1;width=50%;height=50%" {
			// 40 x 12 cells is 400 x 240 pixels; the square picture fits the height.
			if img.Rows != 12 || img.Cols != 24 {
				t.Errorf("%s: %dx%d, want 24x12 (fitted)", tc.args, img.Cols, img.Rows)
			}
			continue
		}
		if img.Cols != tc.cols || img.Rows != tc.rows {
			t.Errorf("%s: %dx%d cells, want %dx%d", tc.args, img.Cols, img.Rows, tc.cols, tc.rows)
		}
	}
}

func TestITerm2IgnoresWhatItShouldNotDraw(t *testing.T) {
	e := newSixelEnv(t)
	data := iterm2PNG(t, 10, 10)
	e.sendITerm2("size=123", data) // not inline: a download request
	e.sendITerm2("inline=0", data) // explicitly not inline
	e.sendITerm2("inline=1", []byte("not a picture"))
	e.sendITerm2("inline=1", nil)
	e.p.Process([]byte("\x1b]1337;File=inline=1\x07"))           // no payload at all
	e.p.Process([]byte("\x1b]1337;File=inline=1:!!!notb64\x07")) // payload that is not base64
	e.p.Process([]byte("\x1b]1337;CursorShape=1\x07"))           // another 1337 command
	if len(e.tv.Images) != 0 {
		t.Fatalf("drew %d pictures from sequences that must not draw", len(e.tv.Images))
	}
}

func TestITerm2AcceptsUnpaddedBase64AndParsesSizes(t *testing.T) {
	e := newSixelEnv(t)
	enc := base64.RawStdEncoding.EncodeToString(iterm2PNG(t, 10, 20))
	e.p.Process([]byte("\x1b]1337;File=inline=1:" + enc + "\x07"))
	if len(e.tv.Images) != 1 {
		t.Fatalf("an unpadded payload was not drawn")
	}
	for in, want := range map[string]iterm2Size{
		"": {}, "auto": {}, "0": {}, "-3": {}, "x": {},
		"12": {'c', 12}, "30px": {'p', 30}, "50%": {'%', 50}, " 7 ": {'c', 7},
	} {
		if got := parseITerm2Size(in); got != want {
			t.Errorf("parseITerm2Size(%q) = %+v, want %+v", in, got, want)
		}
	}
}

// A picture taller than what is left of the screen scrolls the text up, like a
// sixel, and the cursor ends on the picture's last row.
func TestITerm2ScrollsWhenThePictureDoesNotFit(t *testing.T) {
	e := newSixelEnv(t)
	e.tv.CursorX, e.tv.CursorY = 0, 22
	e.sendITerm2("inline=1;width=10;height=6;preserveAspectRatio=0", iterm2PNG(t, 10, 60))
	if len(e.tv.Images) != 1 {
		t.Fatal("no placement")
	}
	if img := e.tv.Images[0]; img.Row+img.Rows-1 != e.tv.ScrollBottom {
		t.Errorf("picture spans rows %d..%d, want it to end on the bottom row %d", img.Row, img.Row+img.Rows-1, e.tv.ScrollBottom)
	}
	if e.tv.CursorY != e.tv.ScrollBottom {
		t.Errorf("cursor row %d, want %d", e.tv.CursorY, e.tv.ScrollBottom)
	}
}
