//go:build windows

package vtui

import "testing"

func TestRestoreViewport_SavedSizeForTheSameWindow(t *testing.T) {
	// 120x30 maximized and restored: conhost shows the restored window with
	// a scroll bar, 118 columns wide, but the window is the one the 120x30
	// came from.
	saved := consoleNormalWindow{cols: 120, rows: 30, winW: 1000, winH: 520, set: true}
	w, h, fromSaved := restoreViewport(saved, 1000, 520, 118, 30)
	if w != 120 || h != 30 || !fromSaved {
		t.Fatalf("got %dx%d fromSaved=%v, want 120x30 from the saved size", w, h, fromSaved)
	}
}

func TestRestoreViewport_OtherWindowKeepsTheRestoredViewport(t *testing.T) {
	saved := consoleNormalWindow{cols: 120, rows: 30, winW: 1000, winH: 520, set: true}
	for _, tc := range []struct {
		name       string
		winW, winH int32
	}{
		{"narrower", 840, 520},
		{"taller", 1000, 700},
		{"unknown", 0, 0},
	} {
		w, h, fromSaved := restoreViewport(saved, tc.winW, tc.winH, 100, 40)
		if w != 100 || h != 40 || fromSaved {
			t.Errorf("%s: got %dx%d fromSaved=%v, want the restored 100x40", tc.name, w, h, fromSaved)
		}
	}
}

func TestRestoreViewport_NothingSaved(t *testing.T) {
	w, h, fromSaved := restoreViewport(consoleNormalWindow{}, 0, 0, 80, 25)
	if w != 80 || h != 25 || fromSaved {
		t.Fatalf("got %dx%d fromSaved=%v, want the restored 80x25", w, h, fromSaved)
	}
}
