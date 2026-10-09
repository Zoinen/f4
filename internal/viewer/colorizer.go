package viewer

// TextColorizer colours the lines of a text read from its first line — the
// head of a file a quick view shows — off the UI thread.
type TextColorizer interface {
	// LineAttrs returns the attribute of each rune of line idx, or nil while
	// that line is not coloured yet or will not be. The slice is shared and
	// must not be modified.
	LineAttrs(idx int) []uint64
	// Close stops the work and releases what it holds.
	Close()
}

// NewTextColorizer, when set, returns a colorizer for lines, the text of
// path, drawn over base; or nil when the text is not to be coloured. redraw
// runs on the UI thread whenever more lines are coloured. quickView says the
// text is a quick view's, which FarColorer's viewer colouring setting treats
// apart from a viewer.
//
// It is a seam: the colours come from Colorer, which the editor package owns
// and which this package cannot import. The application sets it.
var NewTextColorizer func(path string, lines []string, base uint64, quickView bool, redraw func()) TextColorizer
