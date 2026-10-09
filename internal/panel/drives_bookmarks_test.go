package panel

import "testing"

func TestMoveDriveBookmark(t *testing.T) {
	list := []DriveBookmark{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	names := func(l []DriveBookmark) string {
		out := ""
		for _, b := range l {
			out += b.Name
		}
		return out
	}
	if got, ok := moveDriveBookmark(list, 1, -1); !ok || names(got) != "bac" {
		t.Fatalf("b up: %q %v", names(got), ok)
	}
	if got, ok := moveDriveBookmark(list, 1, 1); !ok || names(got) != "acb" {
		t.Fatalf("b down: %q %v", names(got), ok)
	}
	if names(list) != "abc" {
		t.Fatalf("the original list was changed: %q", names(list))
	}
	for _, c := range []struct{ index, delta int }{{0, -1}, {2, 1}, {-1, 1}, {3, -1}, {1, 0}} {
		if got, ok := moveDriveBookmark(list, c.index, c.delta); ok || names(got) != "abc" {
			t.Errorf("index %d delta %d: %q %v, want no move", c.index, c.delta, names(got), ok)
		}
	}
}
