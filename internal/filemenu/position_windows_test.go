package filemenu

import "testing"

func TestGridPointUsesRenderedViewport(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		width, height, wantX, wantY int
	}{
		{"nominal", 1600, 960, 800, 192},
		{"zoomed", 2800, 1800, 1400, 360},
		{"fractional", 2831, 1817, 1415, 363},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := gridPoint(100, 12, 200, 60, tc.width, tc.height)
			if !got.Valid || got.X != tc.wantX || got.Y != tc.wantY {
				t.Fatalf("position = %+v, want %d,%d", got, tc.wantX, tc.wantY)
			}
		})
	}
	if gridPoint(1, 1, 0, 60, 1600, 960).Valid {
		t.Fatal("accepted unknown grid")
	}
}
