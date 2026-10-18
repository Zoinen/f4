package media

import (
	"reflect"
	"testing"

	"github.com/unxed/vtinput"
)

func pressF8(t *testing.T, iv *ImageView) {
	t.Helper()
	if !iv.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_F8}) {
		t.Fatal("F8 was not handled")
	}
}

// F8 asks for the picture on screen to be deleted; once it is gone the next
// picture takes its place, and at the end of the list the previous one does
// (f4#1991).
func TestImageViewF8DeletesAndShowsTheNext(t *testing.T) {
	withStubPipeline(t, 8, 8)

	iv := newTestImageView(t, 40, 20)
	iv.Path = "b.png"
	iv.SetSiblings([]string{"a.png", "b.png", "c.png"}, 1)
	iv.SetSelection(map[string]bool{"b.png": true})

	var asked []string
	confirm := true
	iv.OnDelete = func(Path string, removed func()) {
		asked = append(asked, Path)
		if confirm {
			removed()
		}
	}

	confirm = false
	pressF8(t, iv)
	if iv.Path != "b.png" || len(iv.siblings) != 3 {
		t.Fatalf("a refused delete changed the viewer: %s %v", iv.Path, iv.siblings)
	}

	confirm = true
	pressF8(t, iv)
	if !reflect.DeepEqual(asked, []string{"b.png", "b.png"}) {
		t.Fatalf("asked to delete %v", asked)
	}
	if iv.Path != "c.png" || iv.Index != 1 || !reflect.DeepEqual(iv.siblings, []string{"a.png", "c.png"}) {
		t.Fatalf("after the delete: %s at %d of %v", iv.Path, iv.Index, iv.siblings)
	}
	if iv.Selected["b.png"] {
		t.Error("a deleted picture stays picked")
	}

	pressF8(t, iv)
	if iv.Path != "a.png" || iv.Index != 0 || !reflect.DeepEqual(iv.siblings, []string{"a.png"}) {
		t.Fatalf("deleting the last one: %s at %d of %v", iv.Path, iv.Index, iv.siblings)
	}

	closed := false
	iv.OnClose = func() { closed = true }
	pressF8(t, iv)
	if !closed {
		t.Error("deleting the only picture left must close the viewer")
	}
}

// In the gallery F8 deletes the picture under the cursor, not the one that
// was on screen before the grid opened.
func TestImageViewF8InTheGalleryDeletesTheCursor(t *testing.T) {
	withStubPipeline(t, 8, 8)

	iv := newTestImageView(t, 40, 20)
	iv.Path = "a.png"
	iv.SetSiblings([]string{"a.png", "b.png", "c.png"}, 0)
	iv.ToggleGallery()
	iv.Gal.move(2, 3)

	var asked string
	iv.OnDelete = func(Path string, removed func()) {
		asked = Path
		removed()
	}
	pressF8(t, iv)

	if asked != "c.png" {
		t.Fatalf("deleted %q, the cursor was on c.png", asked)
	}
	if iv.Path != "a.png" || iv.Gal == nil || iv.Gal.Cursor != 1 {
		t.Fatalf("after the delete: %s, gallery %+v", iv.Path, iv.Gal)
	}
	if !reflect.DeepEqual(iv.siblings, []string{"a.png", "b.png"}) {
		t.Fatalf("siblings %v", iv.siblings)
	}
}

// Without anybody to delete files F8 is still taken, so that it does not fall
// through to the panel underneath, but nothing happens.
func TestImageViewF8WithoutDeleter(t *testing.T) {
	withStubPipeline(t, 8, 8)

	iv := newTestImageView(t, 40, 20)
	iv.Path = "a.png"
	iv.SetSiblings([]string{"a.png", "b.png"}, 0)
	pressF8(t, iv)
	if iv.Path != "a.png" || len(iv.siblings) != 2 {
		t.Fatalf("F8 changed the viewer: %s %v", iv.Path, iv.siblings)
	}
	if got := iv.GetKeyLabels().Normal[7]; got != "" {
		t.Errorf("F8 label %q without a deleter", got)
	}
}
