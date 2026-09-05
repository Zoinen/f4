package main

// far2l's own way of putting a picture in a terminal.
//
// far2l running inside f4 never tries the kitty protocol. Its TTY backend
// decides like this:
//
//	if (_tty_caps.kind == TTYCaps::FAR2L) { ...ask over the far2l channel... }
//	else if (CheckKittyImagesSupport()) { ... }
//
// So the moment f4 answers the far2l extension handshake — which it does, and
// which buys the keyboard, the clipboard and the rest — far2l stops looking
// for anything else. The kitty receiver in f4's terminal is never reached, the
// caps query goes unanswered, and far2l's image viewer says the backend does
// not support graphics. Which was true, and is the whole of the bug.
//
// The protocol is FARTTY_INTERACT_IMAGE in far2l's WinPort/FarTTY.h. Three of
// its four subcommands are implemented here; the fourth is transformation,
// which is only asked for when the capabilities say it is available.

import (
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// Capability bits, from WinPort/WinCompat.h. Only raw pixels are claimed: PNG
// and JPEG would mean f4 decoding whatever arrives, and far2l already hands
// over pixels when it is told that is what we take.
const (
	wpImgCapRGBA = 0x001

	// Format, the low bits of the flags word.
	wpImgMaskFmt = 0x00ffff
	wpImgRGBA    = 0
	wpImgRGB     = 1

	// wpImgPixelOffset changes what the far corner of the area means: the
	// picture is not scaled to the area, and Right and Bottom carry a
	// pixel offset instead of a coordinate. far2l's image viewer sets it
	// on every send, so this is the ordinary case and not the exotic one.
	wpImgPixelOffset = 0x100000

	// far2lNoCoord is the -1 a SMALL_RECT field carries when the sender
	// wants the terminal to choose.
	far2lNoCoord = 0xFFFF
)

// far2lImageLimit bounds one picture, in pixels. The same budget the kitty
// receiver uses: neither protocol should be the cheaper way of exhausting
// memory.
const far2lImageLimit = 16 << 20

// handleFar2lImage answers one FARTTY_INTERACT_IMAGE request and pushes the
// reply onto the stack the caller will send back.
func (tv *TerminalView) handleFar2lImage(stk *vtinput.Far2lStack, reply *vtinput.Far2lStack) {
	switch stk.PopU8() {
	case 'c': // FARTTY_INTERACT_IMAGE_CAPS
		cw, ch := tv.CellSize()
		// The order is far2l's: it pops the cell height, then the cell
		// width, then the capabilities, so they go on in reverse.
		reply.PushU64(wpImgCapRGBA)
		reply.PushU16(ptyPixels(cw))
		reply.PushU16(ptyPixels(ch))

	case 's': // FARTTY_INTERACT_IMAGE_SET
		reply.PushU8(tv.far2lImageSet(stk))

	case 'd': // FARTTY_INTERACT_IMAGE_DEL
		id := stk.PopString()
		tv.far2lImageDelete(id)
		reply.PushU8(1)

	default:
		// Transformation, and anything a later far2l invents. Refusing
		// is the honest answer and the sender falls back to sending the
		// picture again, which is why the capabilities do not claim it.
		reply.PushU8(0)
	}
}

// far2lImageSet decodes one picture and puts it on the grid. It returns the
// byte far2l expects: one for shown, zero for refused.
func (tv *TerminalView) far2lImageSet(stk *vtinput.Far2lStack) uint8 {
	id := stk.PopString()
	flags := stk.PopU64()
	left, top := stk.PopU16(), stk.PopU16()
	right, bottom := stk.PopU16(), stk.PopU16()
	width, height := stk.PopU32(), stk.PopU32()

	bpp := 0
	switch flags & wpImgMaskFmt {
	case wpImgRGBA:
		bpp = 4
	case wpImgRGB:
		bpp = 3
	default:
		vtui.DebugLog("FAR2L_IMAGE: format %d is not one we asked for", flags&wpImgMaskFmt)
		return 0
	}
	if width == 0 || height == 0 || uint64(width)*uint64(height) > far2lImageLimit {
		return 0
	}

	data := stk.PopBytes(int(width) * int(height) * bpp)
	if len(data) < int(width)*int(height)*bpp {
		vtui.DebugLog("FAR2L_IMAGE: %d bytes for %dx%d is short", len(data), width, height)
		return 0
	}

	pix := make([]byte, int(width)*int(height)*4)
	if bpp == 4 {
		copy(pix, data)
	} else {
		for i, j := 0, 0; j < len(pix); i, j = i+3, j+4 {
			pix[j], pix[j+1], pix[j+2], pix[j+3] = data[i], data[i+1], data[i+2], 0xFF
		}
	}
	surf := vtui.NewImageSurfaceFromPix(int(width), int(height), int(width)*4, pix)
	if !surf.Valid() {
		return 0
	}
	surf.Opaque = true

	tv.mu.Lock()
	defer tv.mu.Unlock()
	cw, ch := tv.cellSizeUnsafe()

	// A coordinate of -1 means the sender left the choice to us: the corner
	// goes to the cursor and the far side follows from the size of the
	// picture, which is what the protocol says and what far2l's own
	// terminal does.
	col, row := int(left), int(top)
	if left == far2lNoCoord {
		col = tv.CursorX
	}
	if top == far2lNoCoord {
		row = tv.CursorY
	}
	// The far corner means one of two things, and reading it as the wrong
	// one is not a small error: far2l's viewer sends area={44:6 10:16} with
	// the pixel offset flag, and treating 10 as a right-hand column gives a
	// picture minus thirty three cells wide. That was the whole of why the
	// viewer said it had failed to send the image.
	cols := kittyCeilDiv(int64(width), int64(cw))
	rows := kittyCeilDiv(int64(height), int64(ch))
	if flags&wpImgPixelOffset != 0 {
		// Not scaled: the picture is its own size, put down at the
		// corner and shifted by the offset. The shift is applied in
		// whole cells and the remainder is dropped, because a placement
		// starts on a cell boundary; the most that costs is the last
		// cell of a pan.
		if right != far2lNoCoord {
			col += int(right) / cw
		}
		if bottom != far2lNoCoord {
			row += int(bottom) / ch
		}
	} else {
		if right != far2lNoCoord {
			cols = int(right) - col + 1
		}
		if bottom != far2lNoCoord {
			rows = int(bottom) - row + 1
		}
	}
	if cols <= 0 || rows <= 0 {
		vtui.DebugLog("FAR2L_IMAGE: %dx%d cells at %d,%d is not a picture", cols, rows, col, row)
		return 0
	}

	tv.far2lDropImage(id)
	tv.kittyAddPlacement(terminalImage{
		Surface: surf,
		Col:     col,
		Row:     row,
		Cols:    cols,
		Rows:    rows,
		SrcW:    int(width),
		SrcH:    int(height),
		Alt:     tv.UseAltScreen,
		Sixel:   true,
		Far2lID: id,
	})
	return 1
}

// far2lImageDelete takes a picture off the grid by the name its sender gave
// it. Unlike sixel, and like kitty, this protocol can address one.
func (tv *TerminalView) far2lImageDelete(id string) {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	tv.far2lDropImage(id)
}

// far2lDropImage removes every placement carrying the identity. The caller
// holds the lock.
func (tv *TerminalView) far2lDropImage(id string) {
	if id == "" {
		return
	}
	kept := tv.images[:0]
	for _, p := range tv.images {
		if p.Far2lID != id {
			kept = append(kept, p)
		}
	}
	tv.images = kept
}
