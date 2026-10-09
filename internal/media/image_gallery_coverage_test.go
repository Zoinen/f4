package media

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// A window too small to hold even one tile must still produce a grid: one
// tile per axis, not zero.
func TestImageGalleryLayoutTooSmallForATile(t *testing.T) {
	g := &ImageGallery{}
	g.layout(imageTileCols-1, imageTileRows-1)
	if g.cols != 1 || g.rows != 1 {
		t.Fatalf("a window smaller than one tile must still give a 1x1 grid, got %dx%d", g.cols, g.rows)
	}
	if got := g.step(); got != 1 {
		t.Errorf("step() with a single column must be 1, got %d", got)
	}
	if got := g.page(); got != 1 {
		t.Errorf("page() with a single row must be 1, got %d", got)
	}
}

// step and page fall back to the window's own layout once it holds more than
// one tile per axis.
func TestImageGalleryStepAndPageFollowLayout(t *testing.T) {
	g := &ImageGallery{}
	g.layout(imageTileCols*4, imageTileRows*2)
	if got := g.step(); got != 4 {
		t.Errorf("step() should match the column count, got %d", got)
	}
	if got := g.page(); got != 2 {
		t.Errorf("page() should match the row count, got %d", got)
	}
}

// galleryKey is only reached with a live grid; without one it must decline,
// same as with a modifier held that the grid does not claim for itself.
func TestImageGalleryKeyDeclinesWithoutAGridOrWithAModifier(t *testing.T) {
	iv := newTestImageView(t, 40, 20)
	iv.siblings = []string{"a.png", "b.png"}

	if iv.galleryKey(&vtinput.InputEvent{KeyDown: true, Char: 'd'}) {
		t.Error("without an open grid there is nothing to move")
	}

	iv.ToggleGallery()
	iv.Gal.Cursor = 0
	if iv.galleryKey(&vtinput.InputEvent{
		KeyDown:         true,
		Char:            'd',
		ControlKeyState: vtinput.LeftCtrlPressed,
	}) {
		t.Error("a held Ctrl must fall through to the rest of the UI")
	}
	if iv.Gal.Cursor != 0 {
		t.Errorf("a declined key must not move the Cursor, got %d", iv.Gal.Cursor)
	}
}

// Every key the grid claims for movement, tried from the middle of a 30-tile
// directory laid out as 4 columns by 2 rows on screen.
func TestImageGalleryKeyMovesTheCursor(t *testing.T) {
	const total = 30
	siblings := make([]string, total)
	for i := range siblings {
		siblings[i] = "p.png"
	}

	cases := []struct {
		name string
		ev   vtinput.InputEvent
		want int
	}{
		{"a", vtinput.InputEvent{KeyDown: true, Char: 'a'}, 9},
		{"A", vtinput.InputEvent{KeyDown: true, Char: 'A'}, 9},
		{"d", vtinput.InputEvent{KeyDown: true, Char: 'd'}, 11},
		{"D", vtinput.InputEvent{KeyDown: true, Char: 'D'}, 11},
		{"space", vtinput.InputEvent{KeyDown: true, Char: ' '}, 11},
		{"w", vtinput.InputEvent{KeyDown: true, Char: 'w'}, 6},
		{"W", vtinput.InputEvent{KeyDown: true, Char: 'W'}, 6},
		{"s", vtinput.InputEvent{KeyDown: true, Char: 's'}, 14},
		{"S", vtinput.InputEvent{KeyDown: true, Char: 'S'}, 14},
		{"left", vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_LEFT}, 9},
		{"right", vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT}, 11},
		{"up", vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_UP}, 6},
		{"down", vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN}, 14},
		{"pgup", vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_PRIOR}, 2},
		{"pgdn", vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_NEXT}, 18},
		{"home", vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_HOME}, 0},
		{"end", vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_END}, total - 1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			iv := newTestImageView(t, 40, 20)
			iv.siblings = siblings
			iv.ToggleGallery()
			iv.Gal.layout(imageTileCols*4, imageTileRows*2)
			iv.Gal.Cursor = 10

			ev := c.ev
			if !iv.galleryKey(&ev) {
				t.Fatal("a claimed key must report itself handled")
			}
			if iv.Gal.Cursor != c.want {
				t.Errorf("Cursor ended at %d, want %d", iv.Gal.Cursor, c.want)
			}
		})
	}
}

func TestImageViewGalleryPathEdgeCases(t *testing.T) {
	iv := newTestImageView(t, 40, 20)
	iv.siblings = []string{"a.png", "b.png"}

	if got := iv.GalleryPath(); got != "" {
		t.Errorf("without a grid there is no picture under the Cursor, got %q", got)
	}

	iv.Gal = &ImageGallery{Cursor: -1}
	if got := iv.GalleryPath(); got != "" {
		t.Errorf("a negative Cursor must not be indexed, got %q", got)
	}

	iv.Gal.Cursor = len(iv.siblings)
	if got := iv.GalleryPath(); got != "" {
		t.Errorf("a Cursor past the end must not be indexed, got %q", got)
	}

	iv.Gal.Cursor = 1
	if got := iv.GalleryPath(); got != "b.png" {
		t.Errorf("the picture under the Cursor is %q, want b.png", got)
	}
}

// showGallery is painted only for a live grid over a real screen, and an
// empty directory leaves it with nothing to lay out.
func TestImageViewShowGalleryDeclinesWithoutTheRightState(t *testing.T) {
	iv := newTestImageView(t, 40, 20)
	scr := newImageTestScreen(t)

	// Neither a grid nor a screen: both must be quiet no-ops.
	iv.showGallery(nil)
	iv.showGallery(scr)

	iv.ToggleGallery()
	iv.showGallery(nil)

	// A grid with nothing in it: layout runs, but there is nothing to place.
	iv.siblings = nil
	scr.Graphics().BeginFrame()
	iv.showGallery(scr)
	scr.Graphics().EndFrame()
	if n := scr.Graphics().Len(); n != 0 {
		t.Errorf("an empty directory must place no tiles, got %d", n)
	}
}

// A backend without graphics support still captions every tile; it just
// never asks the graphics layer to place a picture.
func TestImageViewShowTileSkipsPlacementWithoutGraphicsSupport(t *testing.T) {
	withStubPipeline(t, 8, 8)

	scr := newImageTestScreen(t)
	scr.Graphics().SetProtocol(vtui.GraphicsNone)

	iv := newTestImageView(t, 40, 20)
	iv.siblings = []string{"a.png"}
	iv.ToggleGallery()
	iv.Gal.thumbs["a.png"] = vtui.NewImageSurface(8, 8)

	scr.Graphics().BeginFrame()
	iv.Show(scr)
	scr.Graphics().EndFrame()

	if n := scr.Graphics().Len(); n != 0 {
		t.Errorf("a backend without graphics support must place nothing, got %d tiles", n)
	}
}

// requestThumb is the one path none of the drawing tests reach for real: a
// tile with no preview inside the file falls back to decoding it whole, and
// the result lands on the UI thread rather than the worker that decoded it.
func TestImageViewRequestThumbFallsBackToFullDecode(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	asked := withStubPipeline(t, 6, 6)

	iv := newTestImageView(t, 40, 20)
	iv.siblings = []string{"a.png"}
	iv.ToggleGallery()

	iv.requestThumb("a.png")

	select {
	case got := <-asked:
		if got != "a.png" {
			t.Fatalf("the fallback decode asked for %q, want a.png", got)
		}
	case <-time.After(time.Second):
		t.Fatal("the fallback decode never started")
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && !iv.Gal.thumbs["a.png"].Valid() {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		default:
			time.Sleep(2 * time.Millisecond)
		}
	}
	surf := iv.Gal.thumbs["a.png"]
	if !surf.Valid() || surf.Width != 6 || surf.Height != 6 {
		t.Fatalf("the decoded picture never reached the tile, got %+v", surf)
	}

	// A tile already asked for must not start a second decode.
	iv.requestThumb("a.png")
	select {
	case got := <-asked:
		t.Fatalf("a picture already asked for was decoded again: %q", got)
	default:
	}
}

// A picture that fails both as a preview and as a whole decode must leave
// the tile empty rather than crash or post a stale result.
func TestImageViewRequestThumbGivesUpWhenDecodeFails(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	old := ImagePipe
	t.Cleanup(func() { ImagePipe = old })

	done := make(chan struct{})
	ImagePipe = newTestPipeline(func(ctx context.Context, v vfs.VFS, Path string) (*vtui.ImageSurface, string, error) {
		defer close(done)
		return nil, "", errors.New("broken file")
	})
	ImagePipe.preview = func(ctx context.Context, v vfs.VFS, Path string) (*vtui.ImageSurface, string, error) {
		return nil, "", errors.New("no thumbnail")
	}

	iv := newTestImageView(t, 40, 20)
	iv.siblings = []string{"bad.png"}
	iv.ToggleGallery()

	iv.requestThumb("bad.png")

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("the decode attempt never ran")
	}
	// requestThumb's own goroutine has nothing left to do but notice the
	// error and return; give it a moment before checking it stayed quiet.
	time.Sleep(20 * time.Millisecond)
	select {
	case <-vtui.FrameManager.TaskChan:
		t.Fatal("a failed decode must not post a tile to the UI thread")
	default:
	}
	if iv.Gal.thumbs["bad.png"].Valid() {
		t.Error("a failed decode left a thumbnail behind")
	}
}
