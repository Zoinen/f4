package vtui

// wheelNotchLines is how many text lines one wheel notch scrolls in vtui
// widgets. It defaults to 1: terminal, X11 and Wayland backends deliver one
// wheel event per notch and historically scrolled a single line. GUI hosts
// that used to expand one notch into several events (gogpu, ebiten) set it
// to the system setting on startup, preserving their previous behavior now
// that the expansion moved from hosts to consumers.
var wheelNotchLines = 1

// WheelLinesPerNotch returns how many text lines one wheel notch scrolls.
// Applications embedding vtui widgets should use this value for their own
// wheel handling so the behavior stays consistent with the widgets.
func WheelLinesPerNotch() int {
	return wheelNotchLines
}

// setWheelNotchLines sets the per-notch scroll amount. Values below 1 are
// ignored. Called by GUI hosts on startup.
func setWheelNotchLines(n int) {
	if n >= 1 {
		wheelNotchLines = n
	}
}

// GetWheelScrollLines returns the operating system's "lines per wheel notch"
// setting, or 3 on platforms that have no such setting.
func GetWheelScrollLines() int {
	return getSystemScrollLines()
}

// WheelArea identifies a class of widgets whose wheel scroll speed can be
// overridden independently by the embedding application.
type WheelArea int

const (
	// WheelAreaList covers tables, list boxes and generic scroll views.
	// It is the zero value, so an untouched ScrollView lands here.
	WheelAreaList WheelArea = iota
	// WheelAreaMenu covers vertical menus (VMenu, including ComboBox
	// dropdowns).
	WheelAreaMenu
	wheelAreaCount
)

// wheelAreaLines holds per-area overrides: [area][0] = up, [area][1] = down.
// A zero entry means "follow WheelLinesPerNotch".
var wheelAreaLines [wheelAreaCount][2]int

// SetWheelAreaLines overrides the wheel scroll speed (lines per notch) for a
// widget area, separately for the up and down directions. A value of 0
// restores the default behavior (follow WheelLinesPerNotch).
func SetWheelAreaLines(area WheelArea, up, down int) {
	if area < 0 || area >= wheelAreaCount {
		return
	}
	if up < 0 {
		up = 0
	}
	if down < 0 {
		down = 0
	}
	wheelAreaLines[area][0] = up
	wheelAreaLines[area][1] = down
}

// wheelLinesFor resolves the lines to scroll for a wheel event in the given
// area. direction > 0 means wheel up (scroll back), < 0 means wheel down.
func wheelLinesFor(area WheelArea, direction int) int {
	if area >= 0 && area < wheelAreaCount {
		v := wheelAreaLines[area][1]
		if direction > 0 {
			v = wheelAreaLines[area][0]
		}
		if v > 0 {
			return v
		}
	}
	return wheelNotchLines
}
