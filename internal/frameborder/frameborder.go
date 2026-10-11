// Package frameborder reads back the colour a frame drew its own border
// with.
//
// A view that decorates a frame it does not own -- a key hint, an empty ring
// around a dialog, a control row painted over somebody else's border -- has
// to reuse that frame's colour instead of hard-coding a palette index, and so
// has to read it off the screen. A frame dragged past the left or the top
// edge has its corners outside the buffer, and ScreenBuf answers an
// off-screen coordinate with a zero cell, which paints the decoration black.
// The probe therefore has to land on a border cell that is still on screen.
package frameborder

import "github.com/unxed/vtui"

// Attr reports the attribute the frame with the corners (x1,y1)-(x2,y2) drew
// its border with. It probes the four edges and answers with the first probe
// that lies on the frame and inside the buffer, so a frame dragged partly off
// an edge still yields its border colour.
//
// It answers false when no probe survives: the border lies wholly off the
// screen -- which includes a frame larger than the screen, whose border runs
// past every edge -- and there is no border colour left to read. The caller
// should then paint nothing.
func Attr(scr *vtui.ScreenBuf, x1, y1, x2, y2 int) (uint64, bool) {
	w, h := scr.Width(), scr.Height()
	probes := [][2]int{
		{max(x1, 0), y1}, // top edge, moved right onto the screen
		{x1, max(y1, 0)}, // left edge, moved down onto the screen
		{max(x1, 0), y2}, // bottom edge, moved right onto the screen
		{x2, max(y1, 0)}, // right edge, moved down onto the screen
	}
	for _, probe := range probes {
		// A probe clamped onto the screen can land on a cell the frame does
		// not occupy: clamping the top edge rightwards steps off the frame
		// once the whole frame sits left of the buffer. Skip those, they
		// would answer with somebody else's colour.
		if probe[0] < x1 || probe[0] > x2 || probe[1] < y1 || probe[1] > y2 {
			continue
		}
		if probe[0] < 0 || probe[0] >= w || probe[1] < 0 || probe[1] >= h {
			continue
		}
		return scr.GetCell(probe[0], probe[1]).Attributes, true
	}
	return 0, false
}
