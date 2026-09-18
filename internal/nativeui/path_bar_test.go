package nativeui

import "testing"

func TestPathBarVisibilitySurvivesShellNormalization(t *testing.T) {
	for _, hidden := range []bool{false, true} {
		shell := appShellFromLegacy(map[string]any{"hidePanelPathBar": hidden})
		if shell.ToMap()["hidePanelPathBar"] != hidden {
			t.Fatal("lost path bar preference")
		}
	}
}
