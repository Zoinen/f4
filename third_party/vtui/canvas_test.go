package vtui

import "testing"

func TestCanvasPlacesAndRemovesSurface(t *testing.T) {
	scr := &ScreenBuf{}
	canvas := NewCanvas()
	canvas.SetPosition(2, 3, 5, 6)
	canvas.SetSurface(NewImageSurface(12, 8))
	canvas.Show(scr)
	placements, _ := scr.Graphics().Snapshot(nil)
	if len(placements) != 1 {
		t.Fatalf("placements = %d, want 1", len(placements))
	}
	p := placements[0]
	if p.Col != 2 || p.Row != 3 || p.Cols != 4 || p.Rows != 4 || p.Surface != canvas.Surface {
		t.Fatalf("placement = %+v", p)
	}
	canvas.SetSurface(nil)
	if got := scr.Graphics().Len(); got != 0 {
		t.Fatalf("placements after clearing surface = %d, want 0", got)
	}
	canvas.Hide(scr)
	if got := scr.Graphics().Len(); got != 0 {
		t.Fatalf("placements after Hide = %d, want 0", got)
	}
}

func TestCanvasFactoryImplementsPropertyAccess(t *testing.T) {
	element, err := NewByType("Canvas")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := element.(PropertyAccess); !ok {
		t.Fatalf("Canvas %T does not implement PropertyAccess", element)
	}
}
