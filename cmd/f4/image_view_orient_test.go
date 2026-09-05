package main

import (
	"context"
	"testing"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestImageViewRotationKeys(t *testing.T) {
	iv := newTestImageView(t, 40, 20)

	turn := func(char rune) {
		t.Helper()
		if !iv.ProcessKey(&vtinput.InputEvent{KeyDown: true, Char: char}) {
			t.Fatalf("%q was not handled", char)
		}
	}
	mirror := func(char rune) {
		t.Helper()
		e := &vtinput.InputEvent{KeyDown: true, Char: char}
		e.ControlKeyState |= vtinput.LeftAltPressed
		if !iv.ProcessKey(e) {
			t.Fatalf("Alt+%q was not handled", char)
		}
	}

	turn('>')
	if iv.rotation != 90 {
		t.Fatalf("one turn forward gave %d", iv.rotation)
	}
	if iv.display().Width != 20 || iv.display().Height != 40 {
		t.Errorf("the turned picture is %dx%d", iv.display().Width, iv.display().Height)
	}

	turn('.')
	if iv.rotation != 180 {
		t.Errorf("the dot must turn like the angle bracket, got %d", iv.rotation)
	}
	turn('<')
	turn(',')
	if iv.rotation != 0 {
		t.Errorf("turning back twice gave %d", iv.rotation)
	}
	if iv.shown != nil || iv.display() != iv.surface {
		t.Error("an unturned picture must be shown as it was decoded, without a copy")
	}

	mirror('>')
	if !iv.flipH || iv.flipV {
		t.Errorf("Alt+> mirrors across the vertical axis, got %v %v", iv.flipH, iv.flipV)
	}
	mirror('<')
	if !iv.flipV {
		t.Error("Alt+< mirrors across the horizontal axis")
	}
	if iv.rotation != 0 {
		t.Errorf("mirroring must not turn the picture, got %d", iv.rotation)
	}
	if iv.display().Width != 40 || iv.display().Height != 20 {
		t.Errorf("mirroring must keep the size, got %dx%d", iv.display().Width, iv.display().Height)
	}
}

func TestImageViewTurnFollowsTheMirror(t *testing.T) {
	iv := newTestImageView(t, 40, 20)

	// One mirrored axis reverses the direction of a turn, so the stored
	// angle has to move backwards for the picture on screen to move
	// forwards.
	iv.Flip(true, false)
	iv.Rotate(90)
	if iv.rotation != 270 {
		t.Errorf("with one axis mirrored a clockwise key must store 270, got %d", iv.rotation)
	}

	// Both axes together are a half turn, which commutes, so the angle goes
	// forward again.
	iv.Flip(false, true)
	iv.Rotate(90)
	if iv.rotation != 0 {
		t.Errorf("with both axes mirrored the angle must go forward, got %d", iv.rotation)
	}
}

func TestImageViewRotationChangesThePlacement(t *testing.T) {
	scr := newImageTestScreen(t)
	iv := newTestImageView(t, 200, 100)

	wide, ok := iv.placementFor(scr)
	if !ok {
		t.Fatal("layout failed")
	}
	if wide.Surface != iv.surface {
		t.Error("an unturned picture is sent as it was decoded")
	}

	iv.Rotate(90)
	tall, ok := iv.placementFor(scr)
	if !ok {
		t.Fatal("layout failed")
	}
	if tall.Surface != iv.shown {
		t.Error("the turned copy is what has to reach the terminal")
	}
	if tall.Cols >= wide.Cols || tall.Rows <= wide.Rows {
		t.Errorf("a turned landscape picture must stand up: %dx%d cells against %dx%d",
			tall.Cols, tall.Rows, wide.Cols, wide.Rows)
	}
}

func TestImageViewRotationSurvivesTheSharperPicture(t *testing.T) {
	iv := newTestImageView(t, 40, 20)
	iv.Rotate(90)

	// The full resolution decode replaces the thumbnail of the same file,
	// which must not undo what the reader has done to the orientation.
	iv.SetImage(ImageResult{Surface: vtui.NewImageSurface(80, 40), Decoder: "stub"})
	if iv.rotation != 90 {
		t.Fatalf("the thumbnail and the picture share an orientation, got %d", iv.rotation)
	}
	if iv.display().Width != 40 || iv.display().Height != 80 {
		t.Errorf("the sharper picture arrived unturned: %dx%d", iv.display().Width, iv.display().Height)
	}
}

func TestImageViewOrientationResetsOnTheNextPicture(t *testing.T) {
	withStubPipeline(t, 20, 10)

	iv := newTestImageView(t, 100, 100)
	iv.path = "a.png"
	iv.SetSiblings([]string{"a.png", "b.png"}, 0)
	if res := ImagePipe.LoadSync(context.Background(), nil, "b.png"); res.Err != nil {
		t.Fatalf("b.png: %v", res.Err)
	}

	iv.Rotate(90)
	iv.Flip(true, false)
	iv.Step(1)

	if iv.path != "b.png" {
		t.Fatalf("the step went to %q", iv.path)
	}
	if iv.rotation != 0 || iv.flipH || iv.flipV || iv.shown != nil {
		t.Errorf("the next picture must arrive as it was decoded: %d %v %v",
			iv.rotation, iv.flipH, iv.flipV)
	}
	if iv.display().Width != 20 || iv.display().Height != 10 {
		t.Errorf("the picture on screen is %dx%d", iv.display().Width, iv.display().Height)
	}
}
