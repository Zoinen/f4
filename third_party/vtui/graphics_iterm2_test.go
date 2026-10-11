package vtui

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func iterm2Solid(w, h int, r, g, b byte) *ImageSurface {
	s := NewImageSurface(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			s.SetPixel(x, y, r, g, b, 255)
		}
	}
	return s
}

var iterm2Seq = regexp.MustCompile(`\x1b\]1337;File=inline=1;width=(\d+);height=(\d+);preserveAspectRatio=0;size=(\d+):([A-Za-z0-9+/=]*)\x07`)

// decodeITerm2 pulls the PNG out of the first File sequence in out.
func decodeITerm2(t *testing.T, out string) (cols, rows int, img image.Image) {
	t.Helper()
	m := iterm2Seq.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("no iTerm2 File sequence in %q", out)
	}
	raw, err := base64.StdEncoding.DecodeString(m[4])
	if err != nil {
		t.Fatal(err)
	}
	if want := len(raw); m[3] != itoa(want) {
		t.Fatalf("size=%s, but the payload is %d bytes", m[3], want)
	}
	img, err = png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("payload is not a PNG: %v", err)
	}
	return atoi(m[1]), atoi(m[2]), img
}

func itoa(n int) string { return strconv.Itoa(n) }

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

func TestITerm2PlacesAPictureOverItsCells(t *testing.T) {
	scr := NewScreenBuf()
	var out bytes.Buffer
	scr.Writer = &out
	scr.AllocBuf(80, 25)
	scr.Graphics().SetProtocol(GraphicsITerm2)
	scr.Graphics().SetCellSize(8, 16)

	scr.Graphics().BeginFrame()
	scr.Graphics().DrawImage("img", ImagePlacement{Surface: iterm2Solid(40, 32, 200, 30, 30), Col: 4, Row: 2, Cols: 10, Rows: 3})
	scr.Graphics().EndFrame()
	scr.Flush()

	raw := out.String()
	at := strings.Index(raw, "\x1b[3;5H\x1b]1337;File=")
	if at < 0 {
		t.Fatalf("no cursor move to row 3, column 5 right before the picture:\n%q", raw)
	}
	cols, rows, img := decodeITerm2(t, raw)
	if cols != 10 || rows != 3 {
		t.Errorf("picture asked for %dx%d cells, want 10x3", cols, rows)
	}
	if b := img.Bounds(); b.Dx() != 40 || b.Dy() != 32 {
		t.Errorf("a small source is sent at its own size, got %dx%d", b.Dx(), b.Dy())
	}
	if r, g, bl, _ := img.At(5, 5).RGBA(); r>>8 != 200 || g>>8 != 30 || bl>>8 != 30 {
		t.Errorf("colour changed on the way: %d %d %d", r>>8, g>>8, bl>>8)
	}
}

func TestITerm2SendsOnlyTheSourceRectangleAndNoMoreThanTheCellsHold(t *testing.T) {
	surf := iterm2Solid(400, 300, 10, 200, 10)
	surf.SetPixel(0, 0, 255, 0, 0, 255)

	var out bytes.Buffer
	e := newITerm2Encoder()
	list := []ImagePlacement{{Surface: surf, Col: 0, Row: 0, Cols: 5, Rows: 2, SrcX: 100, SrcY: 100, SrcW: 200, SrcH: 100}}
	e.Render(&out, list, 8, 16) // the cells hold 40x32 pixels
	_, _, img := decodeITerm2(t, out.String())
	if b := img.Bounds(); b.Dx() > 40 || b.Dy() > 32 {
		t.Errorf("a big source was sent at %dx%d, more than the 40x32 the cells hold", b.Dx(), b.Dy())
	}
	if r, g, _, _ := img.At(0, 0).RGBA(); r>>8 > 100 || g>>8 < 100 {
		t.Errorf("the red pixel outside the source rectangle is in the picture: %d %d", r>>8, g>>8)
	}

	// With no cell size known the crop goes as it is.
	out.Reset()
	newITerm2Encoder().Render(&out, list, 0, 0)
	if _, _, img := decodeITerm2(t, out.String()); img.Bounds().Dx() != 200 || img.Bounds().Dy() != 100 {
		t.Errorf("without a cell size the crop should be sent as it is, got %v", img.Bounds())
	}
}

func TestITerm2CachesAndSkipsWhatCannotBeDrawn(t *testing.T) {
	e := newITerm2Encoder()
	surf := iterm2Solid(16, 16, 1, 2, 3)
	list := []ImagePlacement{
		{Surface: surf, Col: 1, Row: 1, Cols: 2, Rows: 1},
		{Surface: nil, Col: 1, Row: 1, Cols: 2, Rows: 1},
		{Surface: surf, Col: 1, Row: 1, Cols: 0, Rows: 1},
		{Surface: surf, Col: 1, Row: 1, Cols: 2, Rows: 1, SrcX: 100, SrcY: 100, SrcW: 5, SrcH: 5},
	}
	var first, second bytes.Buffer
	e.Render(&first, list, 8, 16)
	if n := strings.Count(first.String(), "\x1b]1337;File="); n != 1 {
		t.Fatalf("drew %d pictures, want only the valid one", n)
	}
	e.Render(&second, list, 8, 16)
	if first.String() != second.String() {
		t.Error("the second frame of the same picture differs from the first")
	}
	if len(e.cache) != 1 {
		t.Errorf("cache holds %d entries, want 1", len(e.cache))
	}
	e.Reset()
	if len(e.cache) != 0 || len(e.order) != 0 {
		t.Error("Reset left encoded pictures behind")
	}

	// The cache is bounded.
	for i := 0; i < iterm2CacheLimit+10; i++ {
		s := iterm2Solid(4, 4, byte(i), 0, 0)
		var sink bytes.Buffer
		e.Render(&sink, []ImagePlacement{{Surface: s, Cols: 1, Rows: 1}}, 8, 16)
	}
	if len(e.cache) > iterm2CacheLimit {
		t.Errorf("cache grew to %d, limit %d", len(e.cache), iterm2CacheLimit)
	}
}

func TestITerm2ForcedRedrawStartsFromNothing(t *testing.T) {
	scr := NewScreenBuf()
	var out bytes.Buffer
	scr.Writer = &out
	scr.AllocBuf(40, 10)
	scr.Graphics().SetProtocol(GraphicsITerm2)
	scr.Graphics().SetCellSize(8, 16)
	scr.Graphics().BeginFrame()
	scr.Graphics().DrawImage("img", ImagePlacement{Surface: iterm2Solid(8, 16, 9, 9, 9), Col: 0, Row: 0, Cols: 1, Rows: 1})
	scr.Graphics().EndFrame()
	scr.Flush()
	ansi, ok := scr.Renderer.(*AnsiRenderer)
	if !ok || ansi.gfxITerm2 == nil || len(ansi.gfxITerm2.cache) == 0 {
		t.Fatal("the picture was not encoded")
	}
	scr.HardReset()
	scr.Flush()
	if strings.Count(out.String(), "\x1b]1337;File=") < 2 {
		t.Error("a forced redraw did not send the picture again")
	}
}

// A source larger than the cells in one direction only is sent as it is: the
// terminal stretches it, and shrinking one side alone would distort it.
func TestITerm2ShrinksOnlyWhenBothSidesAreLarger(t *testing.T) {
	var out bytes.Buffer
	wide := iterm2Solid(400, 10, 1, 2, 3) // wider than 5 cells of 8 px, but shorter than 2 rows of 16
	newITerm2Encoder().Render(&out, []ImagePlacement{{Surface: wide, Cols: 5, Rows: 2}}, 8, 16)
	if _, _, img := decodeITerm2(t, out.String()); img.Bounds().Dx() != 400 || img.Bounds().Dy() != 10 {
		t.Errorf("a source larger in one direction only was resized to %v", img.Bounds())
	}
	out.Reset()
	tall := iterm2Solid(10, 400, 1, 2, 3)
	newITerm2Encoder().Render(&out, []ImagePlacement{{Surface: tall, Cols: 5, Rows: 2}}, 8, 16)
	if _, _, img := decodeITerm2(t, out.String()); img.Bounds().Dx() != 10 || img.Bounds().Dy() != 400 {
		t.Errorf("a source taller only was resized to %v", img.Bounds())
	}
}
