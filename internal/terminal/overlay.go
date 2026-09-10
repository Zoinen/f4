package terminal

// ConsoleOverlayContent is the backend-independent description of the overlay:
// what to put on the command line row, where the cursor belongs, and the keybar
// labels. The ANSI and Win32 Console emitters below render the same struct.
type ConsoleOverlayContent struct {
	Lines     int
	Cmd       string
	CursorCol int
	Keys      []OverlayKeySlot
	Popup     *OverlayPopupContent
}

type OverlayPopupContent struct {
	X         int
	Y         int
	Width     int
	Height    int
	SelectPos int
	Items     []string
}

// OverlayKeySlot is one F-key cell of the overlay keybar: the number, its
// label, and the column the number starts at.
type OverlayKeySlot struct {
	Col   int
	Num   string
	Label string
}
