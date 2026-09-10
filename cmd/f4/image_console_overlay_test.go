package main

import (
	"testing"

	"github.com/unxed/f4/internal/wincon"
	"github.com/unxed/vtui"
)

func TestConsoleFrameKey(t *testing.T) {
	surf := vtui.NewImageSurface(4, 4)
	surf.SetPixel(0, 0, 1, 2, 3, 255)
	other := vtui.NewImageSurface(4, 4)
	other.SetPixel(0, 0, 9, 9, 9, 255)

	base := []consolePiece{{
		rect: wincon.Rect{X: 1, Y: 2, W: 30, H: 40},
		p:    vtui.ImagePlacement{Surface: surf, Col: 1, Row: 2, Cols: 3, Rows: 4},
	}}
	frame := wincon.Rect{X: 1, Y: 2, W: 30, H: 40}
	key := consoleFrameKey(base, frame)
	if key != consoleFrameKey(base, frame) {
		t.Fatal("the same console frame must keep the same key")
	}

	variants := map[string][]consolePiece{
		"moved": {{
			rect: wincon.Rect{X: 5, Y: 2, W: 30, H: 40},
			p:    vtui.ImagePlacement{Surface: surf, Col: 5, Row: 2, Cols: 3, Rows: 4},
		}},
		"different picture": {{
			rect: base[0].rect,
			p:    vtui.ImagePlacement{Surface: other, Col: 1, Row: 2, Cols: 3, Rows: 4},
		}},
		"different source crop": {{
			rect: base[0].rect,
			p:    vtui.ImagePlacement{Surface: surf, Col: 1, Row: 2, Cols: 3, Rows: 4, SrcW: 2, SrcH: 2},
		}},
		"one more picture": append(append([]consolePiece(nil), base...), base[0]),
	}
	for name, variant := range variants {
		if consoleFrameKey(variant, frame) == key {
			t.Errorf("%s must key differently", name)
		}
	}
	if consoleFrameKey(base, wincon.Rect{X: 9, Y: 2, W: 30, H: 40}) == key {
		t.Error("a frame somewhere else must key differently")
	}
}
