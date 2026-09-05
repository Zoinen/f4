package ttyx

import (
	"fmt"
	"math"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/shape"
	"github.com/jezek/xgb/xproto"
)

// Overlay is a window that sits over the terminal and shows pixels, which is
// how a picture reaches the screen in a terminal that knows no image protocol
// at all.
//
// It is override-redirect, so the window manager neither decorates it, nor
// gives it focus, nor lets the user move it away from the terminal it belongs
// to. Its input region is emptied through the SHAPE extension, so the mouse
// goes straight through it to the terminal underneath and selecting text
// keeps working while a picture is up.
type Overlay struct {
	s      *Session
	win    xproto.Window
	gc     xproto.Gcontext
	mapped bool
	shaped bool
	buf    []byte

	// rect is where the caller asked for the picture, on the screen.
	// local is the same place measured from the terminal's corner, which
	// is what the server is told, because the window is a child of the
	// terminal's. Keeping both means a terminal that moves does not have
	// to be reconfigured: the server has already moved the child, and the
	// next Place works out the same local rectangle and does nothing.
	rect  Rect
	local Rect

	// wanted is what the caller asked for, which is not always what is on
	// the screen.
	wanted bool
}

// NewOverlay creates the window. It is not shown until it is placed.
func (s *Session) NewOverlay() (*Overlay, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		return nil, ErrNoDisplay
	}

	win, err := xproto.NewWindowId(s.conn)
	if err != nil {
		return nil, fmt.Errorf("no window id for the overlay: %w", err)
	}
	// A child of the terminal's own window, not a window of its own over
	// the top of everything. That one decision settles most of what an
	// overlay has to get right, and settles it in the X server rather than
	// in this package:
	//
	//   - it moves with the terminal, because the server moves children;
	//   - it is clipped to the terminal, so it cannot spill past the edge
	//     of the window it belongs to;
	//   - anything the window manager puts above the terminal — the alt-tab
	//     switcher, a notification — is above it too, instead of being
	//     covered by a picture that outranked the whole desktop;
	//   - it is not a top-level window, so no task list and no alt-tab ever
	//     offer it;
	//   - it dies with the connection, so leaving f4 leaves nothing behind.
	//
	// It also stops feeling like a separate window and starts feeling like
	// part of the terminal, which is the point of the thing.
	//
	// The value list follows the order of the mask bits: background pixel,
	// backing store, event mask.
	//
	// Backing store is asked for because nothing here runs an event loop:
	// without it the server would expect us to repaint on Expose and the
	// picture would go blank the first time something passed over it. A
	// server is free to ignore the request, which is why the caller is
	// still told to redraw when it redraws everything else.
	mask := uint32(xproto.CwBackPixel | xproto.CwBackingStore | xproto.CwEventMask)
	values := []uint32{0, xproto.BackingStoreAlways, uint32(xproto.EventMaskExposure)}
	err = xproto.CreateWindowChecked(s.conn, s.depth, win, s.win,
		0, 0, 1, 1, 0, xproto.WindowClassInputOutput, s.visual, mask, values).Check()
	if err != nil {
		return nil, fmt.Errorf("the overlay window could not be created: %w", err)
	}

	gc, err := xproto.NewGcontextId(s.conn)
	if err != nil {
		xproto.DestroyWindow(s.conn, win)
		return nil, fmt.Errorf("no graphics context for the overlay: %w", err)
	}
	if err := xproto.CreateGCChecked(s.conn, gc, xproto.Drawable(win), 0, nil).Check(); err != nil {
		xproto.DestroyWindow(s.conn, win)
		return nil, fmt.Errorf("the overlay graphics context could not be created: %w", err)
	}

	o := &Overlay{s: s, win: win, gc: gc}
	o.clearInputRegion()
	s.overlays = append(s.overlays, o)
	return o, nil
}

// clearInputRegion makes the window transparent to the mouse. A server
// without the SHAPE extension leaves the overlay swallowing clicks, which is
// a nuisance rather than a failure, so it is not an error.
func (o *Overlay) clearInputRegion() {
	if !registerShape(o.s.conn) {
		return
	}
	err := shape.RectanglesChecked(o.s.conn, shape.SoSet, shape.SkInput,
		xproto.ClipOrderingUnsorted, o.win, 0, 0, nil).Check()
	o.shaped = err == nil
}

// registerShape tells this connection about the SHAPE extension, which is all
// that Rectangles needs.
//
// It deliberately does not call shape.Init. That function also writes two
// package level maps in xgb, xgb.NewEventFuncs and xgb.NewErrorFuncs, without
// holding a lock, and the reader goroutine of every open connection reads
// xgb.NewEventFuncs for every event that arrives. Initialising an extension
// while any connection is live is therefore a data race inside the library,
// and a concurrent map access in Go is a crash and not merely a warning. The
// per connection opcode is kept in a map that does have a lock, and it is
// exported, so registering only that is both sufficient and safe: the only
// thing the global maps buy is the ability to receive ShapeNotify, which we
// never ask for.
func registerShape(conn *xgb.Conn) bool {
	reply, err := xproto.QueryExtension(conn, 5, "SHAPE").Reply()
	if err != nil || reply == nil || !reply.Present {
		return false
	}
	conn.ExtLock.Lock()
	conn.Extensions["SHAPE"] = reply.MajorOpcode
	conn.ExtLock.Unlock()
	return true
}

// PassesInput reports whether the mouse goes through the overlay.
func (o *Overlay) PassesInput() bool {
	o.s.mu.Lock()
	defer o.s.mu.Unlock()
	return o.shaped
}

// Place moves the overlay to a rectangle of the screen, raises it and shows
// it. An empty rectangle hides it instead.
func (o *Overlay) Place(r Rect) error {
	o.s.mu.Lock()
	defer o.s.mu.Unlock()
	if o.s.conn == nil {
		return ErrNoDisplay
	}
	if r.W <= 0 || r.H <= 0 {
		o.hide()
		return nil
	}

	// The caller says where on the screen the picture goes; the window it
	// lives in is the terminal's, so the server wants that same place
	// measured from the terminal's corner.
	local := Rect{X: r.X - o.s.geom.X, Y: r.Y - o.s.geom.Y, W: r.W, H: r.H}
	if local.X < math.MinInt32 || local.X > math.MaxInt32 || local.Y < math.MinInt32 || local.Y > math.MaxInt32 ||
		local.W > math.MaxUint16 || local.H > math.MaxUint16 {
		return fmt.Errorf("the overlay rectangle is outside the X11 coordinate range")
	}

	if local != o.local {
		// The value list follows the order of the mask bits: x, y, width,
		// height, stack mode.
		err := xproto.ConfigureWindowChecked(o.s.conn, o.win,
			xproto.ConfigWindowX|xproto.ConfigWindowY|
				xproto.ConfigWindowWidth|xproto.ConfigWindowHeight|
				xproto.ConfigWindowStackMode,
			[]uint32{
				// #nosec G115 -- local coordinates and dimensions were checked against their X11 wire ranges above.
				uint32(int32(local.X)), uint32(int32(local.Y)),
				uint32(local.W), uint32(local.H), // #nosec G115 -- dimensions were checked against MaxUint16 above.
				xproto.StackModeAbove,
			}).Check()
		if err != nil {
			return fmt.Errorf("the overlay could not be placed: %w", err)
		}
		o.rect, o.local = r, local
	} else {
		o.rect = r
		xproto.ConfigureWindow(o.s.conn, o.win, xproto.ConfigWindowStackMode,
			[]uint32{xproto.StackModeAbove})
	}

	o.wanted = true
	if !o.mapped && o.s.focused {
		xproto.MapWindow(o.s.conn, o.win)
		o.mapped = true
	}
	return nil
}

// Suspend takes the overlay off the screen without forgetting that the caller
// wants it there. The event loop calls it when the terminal loses the focus,
// because an override-redirect window is above everything and nothing else in
// X will move it out of the way.
func (o *Overlay) Suspend() {
	o.s.mu.Lock()
	defer o.s.mu.Unlock()
	if o.s.conn == nil || !o.mapped {
		return
	}
	xproto.UnmapWindow(o.s.conn, o.win)
	o.mapped = false
}

// followedParent records that the terminal window moved and took the overlay
// with it. Nothing is sent to the server: the child is already in the right
// place, and only its screen coordinates have changed.
func (o *Overlay) followedParent(dx, dy int) {
	o.s.mu.Lock()
	defer o.s.mu.Unlock()
	if o.rect.W > 0 && o.rect.H > 0 {
		o.rect.X += dx
		o.rect.Y += dy
	}
}

// SetBounds limits the overlay to a set of rectangles given relative to its
// own top left corner, so that everything outside them stays the terminal's.
// It is what lets a grid of thumbnails be one window rather than a dozen: the
// captions between the tiles show through the gaps.
//
// An empty set restores the whole window. A server without the SHAPE
// extension cannot do this, and says so, in which case the caller has to fall
// back to covering the lot.
func (o *Overlay) SetBounds(rects []Rect) bool {
	o.s.mu.Lock()
	defer o.s.mu.Unlock()
	if o.s.conn == nil || !o.shaped {
		return false
	}

	if len(rects) == 0 {
		// A mask of None is the way of saying "the whole window".
		err := shape.MaskChecked(o.s.conn, shape.SoSet, shape.SkBounding,
			o.win, 0, 0, xproto.PixmapNone).Check()
		return err == nil
	}

	list := make([]xproto.Rectangle, 0, len(rects))
	for _, r := range rects {
		if r.W <= 0 || r.H <= 0 {
			continue
		}
		if r.X < math.MinInt16 || r.X > math.MaxInt16 || r.Y < math.MinInt16 || r.Y > math.MaxInt16 ||
			r.W > math.MaxUint16 || r.H > math.MaxUint16 {
			return false
		}
		list = append(list, xproto.Rectangle{
			// #nosec G115 -- each rectangle component was checked against its X11 wire range above.
			X: int16(r.X), Y: int16(r.Y),
			Width: uint16(r.W), Height: uint16(r.H),
		})
	}
	if len(list) == 0 {
		return false
	}
	err := shape.RectanglesChecked(o.s.conn, shape.SoSet, shape.SkBounding,
		xproto.ClipOrderingUnsorted, o.win, 0, 0, list).Check()
	return err == nil
}

// Window is the X window the overlay draws into. It is exported so that a
// program which draws for us — a video player handed the window through
// --wid — can be pointed at it.
func (o *Overlay) Window() xproto.Window { return o.win }

// Rect is where the overlay currently is. It is not necessarily where it was
// last put: the event loop moves it when the terminal window moves.
func (o *Overlay) Rect() Rect {
	o.s.mu.Lock()
	defer o.s.mu.Unlock()
	return o.rect
}

// Draw puts straight RGBA pixels into the overlay, at its top left corner.
// The caller has already scaled them: an overlay does no scaling, because
// whoever produced the picture knows better than we do how it should be
// resampled.
func (o *Overlay) Draw(pix []byte, w, h, stride int) error {
	o.s.mu.Lock()
	defer o.s.mu.Unlock()
	if o.s.conn == nil {
		return ErrNoDisplay
	}
	if w <= 0 || h <= 0 || w > math.MaxUint16 || h > math.MaxInt16 {
		return fmt.Errorf("the pixel dimensions %dx%d are outside the X11 coordinate range", w, h)
	}
	lineBytes := w * 4
	if stride < lineBytes || len(pix) < lineBytes || (h > 1 && (len(pix)-lineBytes)/stride < h-1) {
		return fmt.Errorf("the pixel buffer does not match %dx%d", w, h)
	}

	// A request carries a length in four byte units; leave room for the
	// PutImage header, which is twenty four bytes plus padding.
	maxReq := int(xproto.Setup(o.s.conn).MaximumRequestLength) * 4
	rows := (maxReq - 32) / lineBytes
	if rows < 1 {
		rows = 1
	}
	if rows > h {
		rows = h
	}
	if len(o.buf) < rows*lineBytes {
		o.buf = make([]byte, rows*lineBytes)
	}

	for y := 0; y < h; y += rows {
		n := rows
		if y+n > h {
			n = h - y
		}
		// X wants the channels the other way round on the little endian
		// true colour visuals every modern server uses, and the fourth
		// byte is padding rather than alpha: an overlay is opaque.
		for row := 0; row < n; row++ {
			src := pix[(y+row)*stride : (y+row)*stride+lineBytes]
			dst := o.buf[row*lineBytes : (row+1)*lineBytes]
			for len(src) >= 4 && len(dst) >= 4 {
				dst[0], dst[1], dst[2], dst[3] = src[2], src[1], src[0], 0xFF
				src = src[4:]
				dst = dst[4:]
			}
		}
		err := xproto.PutImageChecked(o.s.conn, xproto.ImageFormatZPixmap,
			xproto.Drawable(o.win), o.gc,
			// #nosec G115 -- Draw rejected dimensions outside the X11 uint16/int16 wire ranges above.
			uint16(w), uint16(n), 0, int16(y), 0, o.s.depth,
			o.buf[:n*lineBytes]).Check()
		if err != nil {
			return fmt.Errorf("the picture could not be sent: %w", err)
		}
	}
	return nil
}

// Hide takes the overlay off the screen without destroying it, which is what
// happens every time the terminal loses the focus.
func (o *Overlay) Hide() {
	o.s.mu.Lock()
	defer o.s.mu.Unlock()
	o.hide()
}

func (o *Overlay) hide() {
	o.wanted = false
	if o.s.conn == nil || !o.mapped {
		return
	}
	xproto.UnmapWindow(o.s.conn, o.win)
	o.mapped = false
}

// Visible reports whether the overlay is currently on the screen, which is not
// the same as whether it was asked for: the event loop takes it down while the
// terminal has no focus.
func (o *Overlay) Visible() bool {
	o.s.mu.Lock()
	defer o.s.mu.Unlock()
	return o.mapped
}

// Close destroys the window.
func (o *Overlay) Close() {
	o.s.unregisterOverlay(o)
	o.s.mu.Lock()
	defer o.s.mu.Unlock()
	o.wanted = false
	if o.s.conn == nil {
		return
	}
	xproto.FreeGC(o.s.conn, o.gc)
	xproto.DestroyWindow(o.s.conn, o.win)
	o.mapped = false
}
