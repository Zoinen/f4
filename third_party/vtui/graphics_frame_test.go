package vtui

import (
	"strings"
	"testing"
)

func TestImmediateModeKeepsAndDrops(t *testing.T) {
	var g GraphicsLayer
	surf := NewImageSurface(2, 2)

	g.BeginFrame()
	idA := g.DrawImage("a", ImagePlacement{Surface: surf, Cols: 1, Rows: 1})
	idB := g.DrawImage("b", ImagePlacement{Surface: surf, Col: 5, Cols: 1, Rows: 1})
	g.EndFrame()
	if g.Len() != 2 || idA == idB {
		t.Fatalf("expected two distinct placements, got %d", g.Len())
	}

	g.BeginFrame()
	if got := g.DrawImage("a", ImagePlacement{Surface: surf, Cols: 1, Rows: 1}); got != idA {
		t.Errorf("the same key must keep its placement id, got %d want %d", got, idA)
	}
	g.EndFrame()

	if g.Len() != 1 {
		t.Fatalf("the undeclared placement must disappear, %d left", g.Len())
	}
	list, _ := g.Snapshot(nil)
	if list[0].ID != idA {
		t.Error("the wrong placement survived")
	}
}

func TestImmediateModeIsQuietWhenNothingChanges(t *testing.T) {
	var g GraphicsLayer
	surf := NewImageSurface(2, 2)
	p := ImagePlacement{Surface: surf, Col: 1, Row: 1, Cols: 2, Rows: 2}

	g.BeginFrame()
	g.DrawImage("v", p)
	g.EndFrame()

	gen := g.Generation()
	g.TakeRepaintRequest()

	g.BeginFrame()
	g.DrawImage("v", p)
	g.EndFrame()
	if g.Generation() != gen {
		t.Error("redeclaring the same placement must not bump the generation")
	}
	if g.TakeRepaintRequest() {
		t.Error("an unchanged frame must not ask for a repaint")
	}

	g.BeginFrame()
	p.Col = 4
	g.DrawImage("v", p)
	g.EndFrame()
	if g.Generation() == gen {
		t.Error("moving a placement must bump the generation")
	}
	if !g.TakeRepaintRequest() {
		t.Error("moving a placement must ask for a repaint")
	}
}

func TestEndFrameLeavesManualPlacementsAlone(t *testing.T) {
	var g GraphicsLayer
	surf := NewImageSurface(2, 2)
	manual := g.Add(ImagePlacement{Surface: surf, Cols: 1, Rows: 1})

	g.BeginFrame()
	g.DrawImage("k", ImagePlacement{Surface: surf, Col: 2, Cols: 1, Rows: 1})
	g.EndFrame()

	g.BeginFrame()
	g.EndFrame()

	list, _ := g.Snapshot(nil)
	if len(list) != 1 || list[0].ID != manual {
		t.Errorf("only the keyed placement should have been dropped, got %v", list)
	}
}

func TestEndFrameWithoutBeginIsANoOp(t *testing.T) {
	var g GraphicsLayer
	g.BeginFrame()
	g.DrawImage("k", ImagePlacement{Surface: NewImageSurface(2, 2), Cols: 1, Rows: 1})
	g.EndFrame()

	g.EndFrame()
	if g.Len() != 1 {
		t.Error("a stray EndFrame must not remove anything")
	}
}

func TestFlushDoesNotForceAnExtraFrame(t *testing.T) {
	scr, out := newGraphicsTestScreen(t)
	surf := NewImageSurface(4, 4)
	scr.Graphics().Add(ImagePlacement{Surface: surf, Col: 1, Row: 1, Cols: 4, Rows: 2})

	scr.Flush()
	out.Reset()
	scr.Flush()
	if out.Len() != 0 {
		t.Errorf("a settled screen must produce no output at all: %q", out.String())
	}

	// Removing the image asks for a repaint, and that repaint must happen in
	// the very next frame, not the one after it.
	scr.Graphics().Clear()
	out.Reset()
	scr.Flush()
	if !strings.Contains(out.String(), "\x1b_Ga=d,") {
		t.Errorf("removing an image must delete its placement: %q", out.String())
	}

	out.Reset()
	scr.Flush()
	if out.Len() != 0 {
		t.Errorf("the repaint must not spill into a third frame: %q", out.String())
	}
}

func TestScreenAccessorIsWired(t *testing.T) {
	saved := FrameManager.scr
	defer func() { FrameManager.scr = saved }()

	scr := NewSilentScreenBuf()
	FrameManager.scr = scr
	if FrameManager.Screen() != scr {
		t.Error("Screen must return the ScreenBuf the frames are painted into")
	}
}
