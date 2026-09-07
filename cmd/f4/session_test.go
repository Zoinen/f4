package main

import "testing"

func TestShouldPersistGUIWindowSize(t *testing.T) {
	if shouldPersistGUIWindowSize("") {
		t.Error("terminal mode must not persist its geometry as a GUI window size")
	}
	if !shouldPersistGUIWindowSize("wayland") {
		t.Error("native GUI mode must persist its window size")
	}
}
