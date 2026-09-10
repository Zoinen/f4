package editor

import (
	config "github.com/unxed/f4/internal/config"
	testing "testing"
)

func useColorerCross(t *testing.T, mode int, crosshair bool) {
	t.Helper()

	oldMode, oldCrosshair := config.App.EditorCrossMode, config.App.EditorCrosshair
	config.App.EditorCrossMode = mode
	config.App.EditorCrosshair = crosshair

	t.Cleanup(func() {
		config.App.EditorCrossMode = oldMode
		config.App.EditorCrosshair = oldCrosshair
	})
}

func TestColorerCross_ModeAxes(t *testing.T) {
	cases := []struct {
		mode int
		horz bool
		vert bool
	}{
		{config.ColorerCrossOff, false, false},
		{config.ColorerCrossVertical, false, true},
		{config.ColorerCrossHorizontal, true, false},
		{config.ColorerCrossBoth, true, true},
	}
	for _, tc := range cases {
		horz, vert := CrossModeAxes(tc.mode)
		if horz != tc.horz || vert != tc.vert {
			t.Errorf("Mode %d gave horz=%v vert=%v, expected horz=%v vert=%v", tc.mode, horz, vert, tc.horz, tc.vert)
		}
	}
}

func TestColorerCross_DisabledByTheCrosshairSwitch(t *testing.T) {
	useColorerCross(t, config.ColorerCrossBoth, false)

	if horz, vert, _, _ := EditorCrossAttrs(); horz || vert {
		t.Errorf("Expected no cross with the crosshair off, got horz=%v vert=%v", horz, vert)
	}
}
