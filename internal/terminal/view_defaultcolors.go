package terminal

import "github.com/unxed/vtui"

// hostDefaultColors turns the colours a terminal program leaves at their
// defaults (palette index 7 on 0, what SGR 0, 39 and 49 select) into the host
// terminal's own default colours, so that a mirror of the host console drawn
// by f4 looks like the console itself -- translucent or themed -- and not like
// a black rectangle (#1675). Anything the program coloured explicitly is kept.
func hostDefaultColors(attr uint64) uint64 {
	if attr&vtui.IsFgRGB == 0 && vtui.GetIndexFore(attr) == vtui.GetIndexFore(DefaultTermAttr) {
		attr = vtui.SetDefaultFore(attr)
	}
	if attr&vtui.IsBgRGB == 0 && vtui.GetIndexBack(attr) == vtui.GetIndexBack(DefaultTermAttr) {
		attr = vtui.SetDefaultBack(attr)
	}
	return attr
}
