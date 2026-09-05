package main

// The image viewer's last resort. When the terminal knows no image protocol
// at all — far2l's own VT is the case that prompted issue #663 — but the
// session is a local X one, the picture goes into an X window placed over the
// terminal instead of being replaced by an apology.
//
// Far did the same thing on Windows: its picture viewer was a window of its
// own, put over the console.
//
// Two rules keep this from being a menace. The window of the terminal has to
// have been identified rather than guessed, because an override-redirect
// window is drawn over whatever is underneath it and guessing wrong means
// painting over a stranger's application. And it comes down the moment the
// terminal loses the focus, because nothing in X will take it down for us.

import (
	"encoding/binary"
	"errors"
	"strconv"
	"strings"
	"sync"

	"github.com/unxed/f4/internal/ttyx"
	"github.com/unxed/vtui"
)

// x11ImageOverlay owns the connection and the window. There is one per
// process rather than one per viewer: the connection carries an event loop
// that keeps the focus and the geometry warm, two viewers would fight over
// which window is on top, and the answer to "can pictures be shown at all"
// has to be available before any viewer exists.
type x11ImageOverlay struct {
	sess *ttyx.Session
	ov   *ttyx.Overlay

	// key is what was last drawn, so that a picture nobody is touching is
	// not rescaled and resent on every frame.
	key string
}

var (
	x11OverlayMu    sync.Mutex
	x11OverlayInst  *x11ImageOverlay
	x11OverlayTried bool
)

// sharedX11Overlay connects on the first call and remembers the answer,
// including a negative one: probing costs a round trip and almost every
// terminal worth using needs none of this.
func sharedX11Overlay() *x11ImageOverlay {
	x11OverlayMu.Lock()
	defer x11OverlayMu.Unlock()
	if !x11OverlayTried {
		x11OverlayTried = true
		x11OverlayInst = newX11ImageOverlay()
	}
	return x11OverlayInst
}

// InstallX11Overlay puts the overlay in as the screen's graphics renderer,
// for a terminal that has no image protocol of its own. It is done at startup
// rather than when the first picture is wanted, because everything that shows
// one asks whether the screen supports graphics before it tries, and the
// answer has to be right by then — including in quick view and in the
// built-in terminal, neither of which would ever have asked for an overlay.
//
// A terminal that does have a protocol keeps it: this is the last resort and
// not a preference.
func InstallX11Overlay() {
	scr := vtui.FrameManager.Screen()
	if scr == nil || scr.Graphics().Protocol() != vtui.GraphicsNone {
		return
	}
	sharedX11Overlay()
}

// newX11ImageOverlay connects and creates the window, or returns nil when
// there is nothing to connect to, when the terminal window could not be
// identified, or when the option is off.
func newX11ImageOverlay() *x11ImageOverlay {
	if !AppConfig.ImageOverlay {
		return nil
	}
	sess := sharedTTYXSession()
	if sess == nil {
		return nil
	}
	ov, err := sess.NewOverlay()
	if err != nil {
		vtui.DebugLog("TTYX: no overlay window: %v", err)
		return nil
	}
	vtui.DebugLog("TTYX: overlay ready, mouse passes through: %v", ov.PassesInput())
	x := &x11ImageOverlay{sess: sess, ov: ov}

	// Installed as vtui's external graphics renderer rather than called by
	// whoever happens to be drawing a picture. The layer then reports
	// itself supported, every gate on SupportsGraphics opens at once, and
	// the file viewer, the thumbnail grid, quick view and the pictures a
	// program prints into the built-in terminal all arrive here together,
	// once a frame, without any of them knowing there is no image protocol
	// in the terminal.
	if scr := vtui.FrameManager.Screen(); scr != nil {
		scr.Graphics().SetExternalGraphics(x)
	}
	return x
}

// RenderExternal is vtui's whole-frame call.
//
// It runs with the screen locked, so nothing here may ask the screen anything
// — not its size, not its graphics layer. Everything needed is in the
// arguments, and the first version of this asked for the width and hung f4 on
// the first frame that carried a picture.
func (x *x11ImageOverlay) RenderExternal(list []vtui.ImagePlacement, cellW, cellH, cols, rows int) {
	if len(list) == 0 {
		x.hide()
		return
	}
	if err := x.showMany(cols, rows, list); err != nil && err != errNotNow {
		x.hide()
	}
}

func (x *x11ImageOverlay) close() {
	if x == nil {
		return
	}
	x.ov.Close()
	x.sess.Close()
}

func (x *x11ImageOverlay) hide() {
	if x == nil {
		return
	}
	x.ov.Hide()
	x.key = ""
}

// overlayCellRect converts a rectangle of character cells into a rectangle of
// screen pixels. The size of a cell is not asked for: it is the size of the
// terminal window divided by the number of cells in it, which is exact when
// the terminal leaves no padding and close enough when it does — and unlike
// CSI 16 t it needs no cooperation from a terminal that has already shown it
// cooperates with nothing.
func overlayCellRect(term ttyx.Rect, cols, rows, x1, y1, x2, y2 int) (ttyx.Rect, bool) {
	if cols <= 0 || rows <= 0 || term.W <= 0 || term.H <= 0 {
		return ttyx.Rect{}, false
	}
	if x2 < x1 || y2 < y1 {
		return ttyx.Rect{}, false
	}
	cellW := term.W / cols
	cellH := term.H / rows
	if cellW <= 0 || cellH <= 0 {
		return ttyx.Rect{}, false
	}
	// The frame is clamped to the grid, so that a stale layout cannot send
	// the picture off the side of the window.
	if x1 < 0 {
		x1 = 0
	}
	if y1 < 0 {
		y1 = 0
	}
	if x2 > cols-1 {
		x2 = cols - 1
	}
	if y2 > rows-1 {
		y2 = rows - 1
	}
	if x2 < x1 || y2 < y1 {
		return ttyx.Rect{}, false
	}
	return ttyx.Rect{
		X: term.X + x1*cellW,
		Y: term.Y + y1*cellH,
		W: (x2 - x1 + 1) * cellW,
		H: (y2 - y1 + 1) * cellH,
	}, true
}

// errNoOverlay means there is no way to show a picture at all, and errNotNow
// means there is one but the terminal is not on top of the screen. The
// difference is the whole of the difference between telling the reader that
// their terminal cannot do this and saying nothing while they look at another
// window.
var (
	errNoOverlay = errors.New("no overlay")
	errNotNow    = errors.New("the terminal is not focused")
)

// show puts one placement on the screen.
func (x *x11ImageOverlay) show(cols, rows int, p vtui.ImagePlacement) error {
	return x.showMany(cols, rows, []vtui.ImagePlacement{p})
}

// showMany puts a whole frame's worth of placements on the screen and reports
// whether it managed to. The caller falls back to its apology when it did not.
//
// They go into one window covering the rectangle that holds them all, with a
// shape mask cut to the individual pictures, so that the text between them —
// the captions under a grid of thumbnails — shows through the gaps. A dozen
// windows would do the same thing and cost a dozen of everything.
func (x *x11ImageOverlay) showMany(cols, rows int, list []vtui.ImagePlacement) error {
	if x == nil {
		return errNoOverlay
	}
	if cols <= 0 || rows <= 0 || len(list) == 0 {
		return errNoOverlay
	}
	valid := list[:0:0]
	for _, p := range list {
		if p.Surface.Valid() && p.Cols > 0 && p.Rows > 0 {
			valid = append(valid, p)
		}
	}
	if len(valid) == 0 {
		return errNoOverlay
	}
	list = valid
	win, err := x.sess.Geometry()
	if err != nil {
		x.hide()
		return errNoOverlay
	}
	// The window is not the grid: the top of it may be a menu bar and the
	// right of it a scroll bar. See ttyx_probe.go.
	tw, th, known := hostTextSize(cols, rows)
	term := hostGridRect(win, tw, th, known)
	term = x.refineGrid(term)

	// One window over everything that has to be drawn, so the placements
	// are positioned inside it rather than each getting a window.
	c1, r1, c2, r2 := list[0].Col, list[0].Row, list[0].Col+list[0].Cols-1, list[0].Row+list[0].Rows-1
	for _, p := range list[1:] {
		c1 = minInt(c1, p.Col)
		r1 = minInt(r1, p.Row)
		c2 = maxInt(c2, p.Col+p.Cols-1)
		r2 = maxInt(r2, p.Row+p.Rows-1)
	}
	rect, ok := overlayCellRect(term, cols, rows, c1, r1, c2, r2)
	if !ok {
		x.hide()
		return errNoOverlay
	}

	// Nothing has moved and nothing has changed: the window is already
	// showing the right thing and redrawing it would only cost bandwidth.
	key := overlayFrameKey(list, rect)
	if key == x.key && x.ov.Visible() {
		return nil
	}

	if err := x.ov.Place(rect); err != nil {
		vtui.DebugLog("X11_OVERLAY: %v", err)
		x.hide()
		return errNoOverlay
	}

	buf := make([]byte, rect.W*rect.H*4)
	bounds := make([]ttyx.Rect, 0, len(list))
	for _, p := range list {
		sub, ok := overlayCellRect(term, cols, rows, p.Col, p.Row, p.Col+p.Cols-1, p.Row+p.Rows-1)
		if !ok {
			continue
		}
		// Inside the window rather than on the screen.
		sub.X -= rect.X
		sub.Y -= rect.Y

		sx, sy, sw, sh := p.SrcX, p.SrcY, p.SrcW, p.SrcH
		if sw <= 0 || sh <= 0 {
			sw, sh = p.Surface.Width, p.Surface.Height
			sx, sy = 0, 0
		}
		src := p.Surface
		if sx != 0 || sy != 0 || sw != src.Width || sh != src.Height {
			src = src.Crop(sx, sy, sw, sh)
		}
		scaled := vtui.ScaleSurface(src, sub.W, sub.H)
		if !scaled.Valid() {
			continue
		}
		blitInto(buf, rect.W, rect.H, scaled.Pix, scaled.Width, scaled.Height, scaled.Stride, sub.X, sub.Y)
		bounds = append(bounds, sub)
	}
	if len(bounds) == 0 {
		x.hide()
		return errNoOverlay
	}

	// Everything outside the pictures goes back to the terminal, so the
	// captions between a grid of thumbnails stay readable. A server with no
	// SHAPE extension cannot do it and the window covers the lot, which is
	// a worse picture rather than a broken one.
	if len(bounds) == 1 && bounds[0].X == 0 && bounds[0].Y == 0 &&
		bounds[0].W == rect.W && bounds[0].H == rect.H {
		x.ov.SetBounds(nil)
	} else {
		x.ov.SetBounds(bounds)
	}

	if err := x.ov.Draw(buf, rect.W, rect.H, rect.W*4); err != nil {
		vtui.DebugLog("X11_OVERLAY: %v", err)
		x.hide()
		return errNoOverlay
	}
	x.key = key
	return nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// blitInto composes one scaled picture into the frame buffer at an offset,
// clipped to it.
//
// Composes rather than copies. A picture can arrive in pieces that overlap:
// a program in the built-in terminal drawing a stack of transparent sixel
// layers sends the same rectangle several times, each carrying the pixels the
// layer before got wrong, and a copy would let every layer erase the one
// under it and leave the last one alone on the screen.
func blitInto(dst []byte, dstW, dstH int, src []byte, srcW, srcH, srcStride, atX, atY int) {
	for y := 0; y < srcH; y++ {
		dy := atY + y
		if dy < 0 || dy >= dstH {
			continue
		}
		for x := 0; x < srcW; x++ {
			dx := atX + x
			if dx < 0 || dx >= dstW {
				continue
			}
			s := y*srcStride + x*4
			d := (dy*dstW + dx) * 4
			a := uint32(src[s+3])
			if a == 0xFF {
				copy(dst[d:d+4], src[s:s+4])
				continue
			}
			if a == 0 {
				continue
			}
			dst[d+0] = overByte(dst[d+0], src[s+0], a)
			dst[d+1] = overByte(dst[d+1], src[s+1], a)
			dst[d+2] = overByte(dst[d+2], src[s+2], a)
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

// overlayFrameKey is what was last drawn: every picture, where it came from
// and where it went. A frame nobody is touching is not rescaled and resent.
// refineGrid replaces the grid worked out from the frame with the real thing
// where the terminal draws its text into a window of its own, and applies the
// nudge for the terminals where it does not.
//
// The remaining error where nothing can be found is the padding a terminal
// keeps between its widget and its grid. Nobody reports it: the widget size is
// what CSI 14 t answers with, the grid is somewhere inside it, and the
// difference is a couple of pixels of theme. Hence the setting, which is the
// only way anyone can supply a number nothing else knows.
func (x *x11ImageOverlay) refineGrid(grid ttyx.Rect) ttyx.Rect {
	if inner, ok := x.sess.InnerWindow(grid, overlayGridTolerance); ok {
		if inner != grid {
			vtui.DebugLog("X11_OVERLAY: the text area is a window of its own: %+v", inner)
		}
		grid = inner
	}
	return nudgeGrid(grid)
}

// nudgeGrid applies the setting, and is separate so that the arithmetic can be
// tested without a display.
func nudgeGrid(grid ttyx.Rect) ttyx.Rect {
	grid.X += AppConfig.ImageX11OffsetX
	grid.Y += AppConfig.ImageX11OffsetY
	return grid
}

// overlayGridTolerance is how far a child window may be from the grid worked
// out from the frame and still be believed to be it. Wide enough for the
// padding of any theme, far too tight for anything else in the window tree.
const overlayGridTolerance = 12

func overlayFrameKey(list []vtui.ImagePlacement, rect ttyx.Rect) string {
	var sb strings.Builder
	add := func(v int) {
		sb.WriteString(strconv.Itoa(v))
		sb.WriteByte(0)
	}
	var hashBytes [8]byte
	add(rect.X)
	add(rect.Y)
	add(rect.W)
	add(rect.H)
	for _, p := range list {
		h := p.Surface.Hash()
		binary.LittleEndian.PutUint64(hashBytes[:], h)
		sb.Write(hashBytes[:])
		add(p.Col)
		add(p.Row)
		add(p.Cols)
		add(p.Rows)
		add(p.SrcX)
		add(p.SrcY)
		add(p.SrcW)
		add(p.SrcH)
	}
	return sb.String()
}
