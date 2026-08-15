package vtui

import "testing"

func TestWheelLinesPerNotchDefault(t *testing.T) {
	if got := WheelLinesPerNotch(); got != 1 {
		t.Errorf("Expected default of 1 line per notch, got %d", got)
	}
}

func TestSetWheelNotchLines(t *testing.T) {
	old := WheelLinesPerNotch()
	defer setWheelNotchLines(old)

	setWheelNotchLines(5)
	if got := WheelLinesPerNotch(); got != 5 {
		t.Errorf("Expected 5 lines per notch, got %d", got)
	}

	// Values below 1 are ignored
	setWheelNotchLines(0)
	setWheelNotchLines(-3)
	if got := WheelLinesPerNotch(); got != 5 {
		t.Errorf("Expected values below 1 to be ignored, got %d", got)
	}
}

func TestGetWheelScrollLines(t *testing.T) {
	if got := GetWheelScrollLines(); got <= 0 {
		t.Errorf("Expected positive system scroll lines, got %d", got)
	}
}

func TestWheelAreaOverrides(t *testing.T) {
	defer SetWheelAreaLines(WheelAreaMenu, 0, 0)
	defer SetWheelAreaLines(WheelAreaList, 0, 0)

	// Defaults: fall back to the per-notch value
	if got := wheelLinesFor(WheelAreaMenu, 1); got != WheelLinesPerNotch() {
		t.Errorf("Expected default up to be %d, got %d", WheelLinesPerNotch(), got)
	}
	if got := wheelLinesFor(WheelAreaList, -1); got != WheelLinesPerNotch() {
		t.Errorf("Expected default down to be %d, got %d", WheelLinesPerNotch(), got)
	}

	SetWheelAreaLines(WheelAreaMenu, 2, 5)
	if got := wheelLinesFor(WheelAreaMenu, 1); got != 2 {
		t.Errorf("Expected menu up 2, got %d", got)
	}
	if got := wheelLinesFor(WheelAreaMenu, -1); got != 5 {
		t.Errorf("Expected menu down 5, got %d", got)
	}
	// Other area is unaffected
	if got := wheelLinesFor(WheelAreaList, -1); got != WheelLinesPerNotch() {
		t.Errorf("Expected list down to stay %d, got %d", WheelLinesPerNotch(), got)
	}

	// Zero restores the default; negative clamps to zero; bad area is ignored
	SetWheelAreaLines(WheelAreaMenu, -1, 0)
	if got := wheelLinesFor(WheelAreaMenu, 1); got != WheelLinesPerNotch() {
		t.Errorf("Expected menu up restored to %d, got %d", WheelLinesPerNotch(), got)
	}
	SetWheelAreaLines(wheelAreaCount, 9, 9) // must not panic
}

func TestVMenuUsesMenuWheelArea(t *testing.T) {
	m := NewVMenu("Test")
	if m.WheelArea != WheelAreaMenu {
		t.Errorf("Expected VMenu.WheelArea to be WheelAreaMenu, got %d", m.WheelArea)
	}
}
