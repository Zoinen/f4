package media

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestImageGalleryLayoutAndScrolling(t *testing.T) {
	g := &ImageGallery{}
	g.layout(80, 23)
	if g.cols != 4 || g.rows != 2 {
		t.Fatalf("an 80x23 window holds a 4x2 grid of tiles, got %dx%d", g.cols, g.rows)
	}

	// Twenty pictures are five rows of four, of which two fit on screen.
	g.scrollTo(19, 20)
	if g.top != 3 {
		t.Errorf("the last row has to come into view, got top %d", g.top)
	}
	g.scrollTo(0, 20)
	if g.top != 0 {
		t.Errorf("the first row has to come into view, got top %d", g.top)
	}

	// A grid that is not even Full has nothing to scroll.
	g.scrollTo(2, 3)
	if g.top != 0 {
		t.Errorf("a grid with three pictures must not scroll, got top %d", g.top)
	}

	g.move(-100, 20)
	if g.Cursor != 0 {
		t.Errorf("the Cursor stops at the first picture, got %d", g.Cursor)
	}
	g.move(100, 20)
	if g.Cursor != 19 {
		t.Errorf("the Cursor stops at the last picture, got %d", g.Cursor)
	}
}

func TestImageViewGallerySelectionReachesThePanel(t *testing.T) {
	withStubPipeline(t, 8, 8)

	iv := newTestImageView(t, 40, 20)
	iv.Path = "b.png"
	iv.SetSiblings([]string{"a.png", "b.png", "c.png"}, 1)

	var reported []string
	iv.OnSelect = func(Path string, on bool) {
		reported = append(reported, fmt.Sprintf("%s=%v", Path, on))
	}

	press := func(vk uint16) {
		t.Helper()
		if !iv.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vk}) {
			t.Fatalf("key %d was not handled", vk)
		}
	}

	press(vtinput.VK_F12)
	if iv.Gal == nil || iv.Gal.Cursor != 1 {
		t.Fatal("the grid opens on the picture that was on screen")
	}

	press(vtinput.VK_INSERT)
	if !iv.Selected["b.png"] {
		t.Error("Ins did not pick the picture under the Cursor")
	}
	if iv.Gal.Cursor != 2 {
		t.Errorf("Ins moves on, the Cursor is at %d", iv.Gal.Cursor)
	}

	press(vtinput.VK_INSERT)
	press(vtinput.VK_DELETE)
	if iv.Selected["c.png"] {
		t.Error("Del did not unpick the picture under the Cursor")
	}

	want := []string{"b.png=true", "c.png=true", "c.png=false"}
	if strings.Join(reported, " ") != strings.Join(want, " ") {
		t.Errorf("the panel was told %v, expected %v", reported, want)
	}

	// Escape leaves the grid without leaving the viewer.
	press(vtinput.VK_ESCAPE)
	if iv.Gal != nil {
		t.Error("Escape did not close the grid")
	}
	if iv.IsDone() {
		t.Error("Escape closed the whole viewer instead of the grid")
	}
}

func TestImageViewGalleryDrawsATileForEachPicture(t *testing.T) {
	withStubPipeline(t, 8, 8)

	scr := newImageTestScreen(t)
	iv := newTestImageView(t, 40, 20)
	iv.Path = "a.png"
	iv.SetSiblings([]string{"a.png", "b.png", "c.png"}, 0)
	iv.ToggleGallery()

	// Two thumbnails have arrived; fetching the third is a background job
	// that has no business running on the drawing Path, and this test is not
	// about it.
	iv.Gal.thumbs["a.png"] = vtui.NewImageSurface(8, 8)
	iv.Gal.thumbs["b.png"] = vtui.NewImageSurface(8, 8)
	iv.Gal.asked["c.png"] = true

	scr.Graphics().BeginFrame()
	iv.Show(scr)
	scr.Graphics().EndFrame()

	if n := scr.Graphics().Len(); n != 2 {
		t.Errorf("two thumbnails are ready, %d were placed", n)
	}

	// Every tile is captioned, whether its thumbnail has arrived or not.
	row := testutil.ScreenRow(scr, imageTileRows, 0, 79)
	for _, name := range []string{"a.png", "b.png", "c.png"} {
		if !strings.Contains(row, name) {
			t.Errorf("the caption row is %q, without %s", row, name)
		}
	}
}

func TestImageViewGalleryEnterOpensTheCursor(t *testing.T) {
	withStubPipeline(t, 20, 10)

	iv := newTestImageView(t, 100, 100)
	iv.Path = "a.png"
	iv.SetSiblings([]string{"a.png", "b.png"}, 0)
	if res := ImagePipe.LoadSync(context.Background(), nil, "b.png"); res.Err != nil {
		t.Fatalf("b.png: %v", res.Err)
	}

	iv.ToggleGallery()
	iv.Gal.move(1, 2)
	iv.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})

	if iv.Gal != nil {
		t.Error("Enter has to leave the grid")
	}
	if iv.Path != "b.png" || iv.Index != 1 {
		t.Errorf("Enter opened %q at %d", iv.Path, iv.Index)
	}
}
