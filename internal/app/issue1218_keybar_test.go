package app

import "testing"

func TestAppQuitUsesTheLocalizedF10KeyBarLabel(t *testing.T) {
	a, ok := GetAction("App.Quit")
	if !ok {
		t.Fatal("App.Quit is not registered")
	}
	if a.LabelKey != "KeyBar.F10" {
		t.Fatalf("App.Quit label key = %q, want %q", a.LabelKey, "KeyBar.F10")
	}
}
