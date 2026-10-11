package media

import (
	"testing"

	"github.com/unxed/vtui"
)

func TestImageViewSmallHelpersCoverage(t *testing.T) {
	var nilView *ImageView
	if gal, cursor, Path := nilView.GalleryState(); gal != nil || cursor != 0 || Path != "" {
		t.Fatalf("nil GalleryState = %v, %d, %q", gal, cursor, Path)
	}

	iv := NewGalleryView("second.png", []string{"first.png", "second.png"}, 1, 1)
	if gal, cursor, Path := iv.GalleryState(); gal != iv.Gal || cursor != 1 || Path != "second.png" {
		t.Fatalf("GalleryState = %p, %d, %q", gal, cursor, Path)
	}
	if iv.barHeight() != 1 {
		t.Fatal("normal view must reserve the title bar")
	}
	iv.Full = true
	if iv.barHeight() != 0 {
		t.Fatal("fullscreen view must not reserve the title bar")
	}

	for _, tc := range []struct {
		pixels, cell, limit, want int
	}{
		{0, 8, 20, 1},
		{9, 8, 20, 2},
		{100, 8, 5, 5},
	} {
		if got := CellsFor(tc.pixels, tc.cell, tc.limit); got != tc.want {
			t.Errorf("CellsFor(%d,%d,%d) = %d, want %d", tc.pixels, tc.cell, tc.limit, got, tc.want)
		}
	}
	for _, tc := range []struct {
		rotation     int
		flipH, flipV bool
		want         string
	}{
		{0, false, false, ""},
		{90, false, false, "90°"},
		{180, true, false, "180°, mirror H"},
		{270, false, true, "270°, mirror V"},
		{0, true, true, "mirror H, mirror V"},
	} {
		if got := imageOrientationLabel(tc.rotation, tc.flipH, tc.flipV); got != tc.want {
			t.Errorf("orientation label = %q, want %q", got, tc.want)
		}
	}
}

func TestImageViewGuardsAndTogglesCoverage(t *testing.T) {
	iv := &ImageView{}
	iv.Pan(1, 1)
	iv.SetImage(ImageResult{})
	if iv.Zoom != 0 || iv.panX != 0 || iv.panY != 0 {
		t.Fatalf("invalid image changed state: zoom=%v pan=%v,%v", iv.Zoom, iv.panX, iv.panY)
	}

	iv = newTestImageView(t, 20, 10)
	iv.Zoom = 4
	iv.panX, iv.panY = 10, 20
	iv.ToggleActualSize()
	if !iv.actual || iv.Zoom != 1 || iv.panX != 0 || iv.panY != 0 {
		t.Fatalf("actual-size toggle = actual %v zoom %v pan %v,%v", iv.actual, iv.Zoom, iv.panX, iv.panY)
	}
	iv.ToggleActualSize()
	if iv.actual {
		t.Fatal("second actual-size toggle must restore fitting mode")
	}
	iv.ToggleOverlay()
	if !iv.Overlay {
		t.Fatal("overlay toggle did not enable the panel")
	}
	iv.ToggleOverlay()
	if iv.Overlay {
		t.Fatal("overlay toggle did not disable the panel")
	}

	old := vtui.FrameManager.HideBars
	t.Cleanup(func() { vtui.FrameManager.HideBars = old })
	iv.ResizeConsole(80, 25)
	iv.SetFullScreen(true)
	iv.SetFullScreen(true)
	if !iv.Full || !vtui.FrameManager.HideBars || iv.Y2 != 24 {
		t.Fatalf("fullscreen layout = full %v hide %v y2 %d", iv.Full, vtui.FrameManager.HideBars, iv.Y2)
	}
	iv.SetFullScreen(false)
	iv.SetFullScreen(false)
	if iv.Full || vtui.FrameManager.HideBars || iv.Y2 != 23 {
		t.Fatalf("windowed layout = full %v hide %v y2 %d", iv.Full, vtui.FrameManager.HideBars, iv.Y2)
	}
}
