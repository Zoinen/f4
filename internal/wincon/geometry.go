package wincon

// The console overlay, minus the system calls.
//
// Everything here is arithmetic and judgement, so it compiles and is tested
// everywhere, including on the machine this was written on, which has no
// Windows console at all. The parts that talk to user32 and gdi32 live in
// overlay_windows.go and are the only parts that cannot be.

// Rect is a rectangle in the console window's client coordinates: the pixel
// area the text is drawn in, with (0,0) at its top left corner. That is
// simpler than the X side, where the top of the window may be a menu bar —
// a console window has no furniture inside its client area.
type Rect struct {
	X, Y, W, H int
}

// Source says how the console window was identified, and therefore how much
// it can be trusted.
type Source int

const (
	// SourceNone means no console window was found.
	SourceNone Source = iota

	// SourceConsole means GetConsoleWindow returned a real, visible window:
	// a classic console, which is conhost, which is what cmd.exe runs in.
	SourceConsole

	// SourceHidden means GetConsoleWindow returned a window that is not on
	// the screen. That is what a pseudoconsole looks like — Windows
	// Terminal hosts the console in one and draws the text itself, so the
	// window exists, is never shown, and is the wrong thing to draw over.
	SourceHidden

	// SourcePseudo means GetConsoleWindow returned a ConPTY pseudo console
	// window: class PseudoConsoleWindow, owned by the terminal on the far
	// side of the pty.
	//
	// It is reported *visible*, which is why looking at visibility alone was
	// not enough. When a Windows Terminal tab is on screen OpenConsole calls
	// ShowWindow(SW_SHOWNOACTIVATE) on it, and when the tab is hidden it
	// minimizes it rather than hiding it, so WS_VISIBLE is never cleared and
	// IsWindowVisible answers true either way. The window is nonetheless 0x0
	// with no client area at all, so an overlay parented to it drew every
	// frame into nothing and gave up with "nothing on the client area" —
	// which is exactly what the picture-never-appears report looks like
	// (docs/WINCON_805_HANDOVER.md F2, F3; measured again in F14 and by the
	// field runs behind F23/F24).
	SourcePseudo
)

func (s Source) String() string {
	switch s {
	case SourceConsole:
		return "GetConsoleWindow"
	case SourceHidden:
		return "a hidden pseudoconsole"
	case SourcePseudo:
		return "a ConPTY pseudo console window (Windows Terminal or another terminal on the far side of a pty)"
	}
	return "nothing"
}

// Trusted reports whether the window is one to draw on.
//
// Only a real, visible classic console window is. Windows Terminal is
// deliberately excluded and needs no overlay: it renders sixel itself, so
// pictures go down the wire as they do on any capable terminal, and drawing
// over its 0x0 pseudo window would put them nowhere.
func (s Source) Trusted() bool { return s == SourceConsole }

// ClassifyConsoleWindow decides what GetConsoleWindow just returned, from the
// window's class name and whether it is on screen.
//
// The class is asked first and visibility second, because a pseudo console
// window answers "visible" (see SourcePseudo). An unfamiliar class is not
// trusted: whatever it belongs to, f4 did not create it and has no measured
// reason to believe pixels put there will be seen.
func ClassifyConsoleWindow(class string, visible bool) Source {
	switch class {
	case "PseudoConsoleWindow":
		return SourcePseudo
	case "ConsoleWindowClass":
		if visible {
			return SourceConsole
		}
		return SourceHidden
	}
	return SourceHidden
}

// CellRect turns a rectangle of character cells into pixels of the client
// area, given the size of a cell.
//
// The console reports its font size directly, so unlike the terminals on the
// other side of this there is nothing to infer, nothing to round and nothing
// to be a pixel or two out by.
func CellRect(cellW, cellH, c1, r1, c2, r2 int) (Rect, bool) {
	if cellW <= 0 || cellH <= 0 || c2 < c1 || r2 < r1 {
		return Rect{}, false
	}
	return Rect{
		X: c1 * cellW,
		Y: r1 * cellH,
		W: (c2 - c1 + 1) * cellW,
		H: (r2 - r1 + 1) * cellH,
	}, true
}

// ClipToClient trims a rectangle to the client area. A picture is placed from
// the grid, and the grid is what the console says it is, so anything sticking
// out is a disagreement between the two rather than something to draw.
func ClipToClient(r Rect, clientW, clientH int) (Rect, bool) {
	if r.W <= 0 || r.H <= 0 || clientW <= 0 || clientH <= 0 {
		return Rect{}, false
	}
	if r.X < 0 {
		r.W += r.X
		r.X = 0
	}
	if r.Y < 0 {
		r.H += r.Y
		r.Y = 0
	}
	if r.X+r.W > clientW {
		r.W = clientW - r.X
	}
	if r.Y+r.H > clientH {
		r.H = clientH - r.Y
	}
	if r.W <= 0 || r.H <= 0 {
		return Rect{}, false
	}
	return r, true
}

// Union is the smallest rectangle holding all of them, which is the window one
// frame goes into. The gaps between the pictures are cut back out of it with a
// region, so the text between a grid of thumbnails still shows.
func Union(rects []Rect) (Rect, bool) {
	var out Rect
	first := true
	for _, r := range rects {
		if r.W <= 0 || r.H <= 0 {
			continue
		}
		if first {
			out, first = r, false
			continue
		}
		if r.X < out.X {
			out.W += out.X - r.X
			out.X = r.X
		}
		if r.Y < out.Y {
			out.H += out.Y - r.Y
			out.Y = r.Y
		}
		if r.X+r.W > out.X+out.W {
			out.W = r.X + r.W - out.X
		}
		if r.Y+r.H > out.Y+out.H {
			out.H = r.Y + r.H - out.Y
		}
	}
	return out, !first
}

// blitInto copies one picture into the frame buffer at an offset, clipped to
// it. The buffer is BGRA, bottom-up, because that is what a device independent
// bitmap is unless it is told otherwise; the row is chosen by the caller.
func blitInto(dst []byte, dstW, dstH int, src []byte, srcW, srcH, srcStride, atX, atY int) {
	for y := 0; y < srcH; y++ {
		dy := atY + y
		if dy < 0 || dy >= dstH {
			continue
		}
		row := (dstH - 1 - dy) * dstW * 4
		for x := 0; x < srcW; x++ {
			dx := atX + x
			if dx < 0 || dx >= dstW {
				continue
			}
			s := y*srcStride + x*4
			d := row + dx*4
			a := uint32(src[s+3])
			if a == 0xFF {
				// RGBA in, BGRA out.
				dst[d+0] = src[s+2]
				dst[d+1] = src[s+1]
				dst[d+2] = src[s+0]
				dst[d+3] = 0xFF
				continue
			}
			if a == 0 {
				continue
			}
			// Source over destination, straight alpha. A picture that
			// arrives in pieces -- a stack of transparent sixel layers
			// from a program in the built-in terminal -- is composed
			// here, and a copy would let each layer erase the one
			// under it.
			dst[d+0] = overByte(dst[d+0], src[s+2], a)
			dst[d+1] = overByte(dst[d+1], src[s+1], a)
			dst[d+2] = overByte(dst[d+2], src[s+0], a)
			dst[d+3] = byte(a + uint32(dst[d+3])*(255-a)/255)
		}
	}
}

// overByte is one channel of source-over with straight (non-premultiplied)
// alpha, rounded rather than truncated so a stack of layers does not drift
// darker with every one of them.
func overByte(dst, src byte, a uint32) byte {
	return byte((uint32(src)*a + uint32(dst)*(255-a) + 127) / 255)
}
