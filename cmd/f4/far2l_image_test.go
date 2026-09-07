package main

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/unxed/vtinput"
)

// far2lRequest builds one FARTTY_INTERACT_IMAGE request the way far2l builds
// it: the arguments pushed in the order its TTYBackend pushes them, then the
// subcommand, then the command, then the request id on top.
func far2lRequest(id uint8, sub uint8, build func(*vtinput.Far2lStack)) []byte {
	stk := vtinput.Far2lStack{}
	if build != nil {
		build(&stk)
	}
	stk.PushU8(sub)
	stk.PushU8('i')
	stk.PushU8(id)
	return stk
}

// send feeds the request through the parser the way it arrives on the wire and
// returns what came back, with the reply's own id byte already taken off.
func far2lSend(t *testing.T, e *sixelEnv, req []byte) *vtinput.Far2lStack {
	t.Helper()
	e.pty.mu.Lock()
	e.pty.written = nil
	e.pty.mu.Unlock()

	e.p.Process([]byte("\x1b_far2l:" + base64.StdEncoding.EncodeToString(req) + "\x07"))

	// The handler answers on a goroutine of its own, so the reply arrives
	// after the parser has returned.
	var out string
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if out = e.pty.String(); len(out) > 8 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if len(out) < 9 {
		t.Fatalf("no reply: %q", out)
	}
	// \x1b_far2l<base64>\x07
	body := out[len("\x1b_far2l") : len(out)-1]
	raw, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		t.Fatalf("reply is not base64: %q", body)
	}
	stk := vtinput.Far2lStack(raw)
	_ = stk.PopU8() // the id
	return &stk
}

// far2l asks over its own channel and never tries kitty once the far2l
// extension is answered, so this is the answer that decides whether its image
// viewer works at all.
func TestFar2lImageCaps(t *testing.T) {
	e := newSixelEnv(t)
	reply := far2lSend(t, e, far2lRequest(7, 'c', nil))

	// far2l pops the cell height, then the width, then the capabilities.
	ch := reply.PopU16()
	cw := reply.PopU16()
	caps := reply.PopU64()

	if caps&wpImgCapRGBA == 0 {
		t.Errorf("raw pixels must be offered: caps=%#x", caps)
	}
	if cw != 10 || ch != 20 {
		t.Errorf("cell: got %dx%d, want 10x20", cw, ch)
	}
}

// A picture handed over as raw pixels lands on the grid where the sender put
// it, at the size the sender asked for.
func TestFar2lImageSet(t *testing.T) {
	e := newSixelEnv(t)
	const w, h = 4, 2

	req := far2lRequest(1, 's', func(stk *vtinput.Far2lStack) {
		pix := make([]byte, w*h*3)
		for i := range pix {
			pix[i] = byte(i)
		}
		stk.PushBytes(pix)
		stk.PushU32(h)
		stk.PushU32(w)
		stk.PushU16(6) // bottom
		stk.PushU16(9) // right
		stk.PushU16(3) // top
		stk.PushU16(2) // left
		stk.PushU64(wpImgRGB)
		stk.PushString("image_viewer")
	})
	if got := far2lSend(t, e, req).PopU8(); got != 1 {
		t.Fatalf("the picture was refused: %d", got)
	}

	if len(e.tv.images) != 1 {
		t.Fatalf("expected one placement, got %d", len(e.tv.images))
	}
	p := e.tv.images[0]
	if p.Col != 2 || p.Row != 3 || p.Cols != 8 || p.Rows != 4 {
		t.Errorf("placement: got %d,%d %dx%d, want 2,3 8x4", p.Col, p.Row, p.Cols, p.Rows)
	}
	if p.Far2lID != "image_viewer" {
		t.Errorf("identity: %q", p.Far2lID)
	}
	if r, g, b, a := p.Surface.PixelAt(0, 0); r != 0 || g != 1 || b != 2 || a != 255 {
		t.Errorf("first pixel: %d,%d,%d,%d", r, g, b, a)
	}
}

// Unlike sixel, this protocol can address a picture after the fact, which is
// how far2l takes its viewer down.
func TestFar2lImageDelete(t *testing.T) {
	e := newSixelEnv(t)
	set := far2lRequest(1, 's', func(stk *vtinput.Far2lStack) {
		stk.PushBytes(make([]byte, 4*2*3))
		stk.PushU32(2)
		stk.PushU32(4)
		stk.PushU16(6)
		stk.PushU16(9)
		stk.PushU16(3)
		stk.PushU16(2)
		stk.PushU64(wpImgRGB)
		stk.PushString("image_viewer")
	})
	far2lSend(t, e, set)
	if len(e.tv.images) != 1 {
		t.Fatalf("setup: %d placements", len(e.tv.images))
	}

	del := far2lRequest(2, 'd', func(stk *vtinput.Far2lStack) {
		stk.PushString("image_viewer")
	})
	if got := far2lSend(t, e, del).PopU8(); got != 1 {
		t.Errorf("delete was refused: %d", got)
	}
	if len(e.tv.images) != 0 {
		t.Errorf("the picture must be gone: %d left", len(e.tv.images))
	}
}

// Sending under a name that is already on the screen replaces it rather than
// stacking a second copy on top.
func TestFar2lImageSetReplaces(t *testing.T) {
	e := newSixelEnv(t)
	build := func(left uint16) []byte {
		return far2lRequest(1, 's', func(stk *vtinput.Far2lStack) {
			stk.PushBytes(make([]byte, 4*2*3))
			stk.PushU32(2)
			stk.PushU32(4)
			stk.PushU16(6)
			stk.PushU16(left + 7)
			stk.PushU16(3)
			stk.PushU16(left)
			stk.PushU64(wpImgRGB)
			stk.PushString("image_viewer")
		})
	}
	far2lSend(t, e, build(2))
	far2lSend(t, e, build(20))

	if len(e.tv.images) != 1 {
		t.Fatalf("one name is one picture, got %d", len(e.tv.images))
	}
	if e.tv.images[0].Col != 20 {
		t.Errorf("the second send must win: col %d", e.tv.images[0].Col)
	}
}

// A format we never offered has to be refused rather than drawn as garbage.
func TestFar2lImageRefusesUnknownFormat(t *testing.T) {
	e := newSixelEnv(t)
	req := far2lRequest(1, 's', func(stk *vtinput.Far2lStack) {
		stk.PushBytes([]byte{1, 2, 3})
		stk.PushU32(1)
		stk.PushU32(1)
		stk.PushU16(1)
		stk.PushU16(1)
		stk.PushU16(0)
		stk.PushU16(0)
		stk.PushU64(2) // WP_IMG_PNG, which the capabilities do not claim
		stk.PushString("x")
	})
	if got := far2lSend(t, e, req).PopU8(); got != 0 {
		t.Errorf("PNG was not offered and must be refused: %d", got)
	}
	if len(e.tv.images) != 0 {
		t.Error("nothing may be placed")
	}
}

// Transformation is not among the capabilities, so it is refused and the
// sender sends the picture again instead.
func TestFar2lImageTransformRefused(t *testing.T) {
	e := newSixelEnv(t)
	req := far2lRequest(1, 't', func(stk *vtinput.Far2lStack) {
		stk.PushU16(0)
		stk.PushU16(1)
		stk.PushU16(1)
		stk.PushU16(0)
		stk.PushU16(0)
		stk.PushString("x")
	})
	if got := far2lSend(t, e, req).PopU8(); got != 0 {
		t.Errorf("got %d, want a refusal", got)
	}
}

// The numbers far2l's image viewer actually sends. Its log line was
//
//	OnSetConsoleImage: id='image_viewer' flags=0x100001 area={44:6 10:16}
//	                   width=1280 height=980
//	SendWholeImage: error at 0 of 980
//
// and the reason for the error was reading 10 and 16 as the far corner of the
// area rather than as a pixel offset: the picture came out minus thirty three
// cells wide and was refused.
func TestFar2lImageViewerSendIsAccepted(t *testing.T) {
	e := newSixelEnv(t)
	const w, h = 128, 98 // the same shape, small enough for a test

	req := far2lRequest(1, 's', func(stk *vtinput.Far2lStack) {
		stk.PushBytes(make([]byte, w*h*3))
		stk.PushU32(h)
		stk.PushU32(w)
		stk.PushU16(16) // bottom: a pixel offset, not a row
		stk.PushU16(10) // right: a pixel offset, not a column
		stk.PushU16(6)  // top
		stk.PushU16(44) // left
		stk.PushU64(wpImgPixelOffset | wpImgRGB)
		stk.PushString("image_viewer")
	})
	if got := far2lSend(t, e, req).PopU8(); got != 1 {
		t.Fatalf("the viewer's own send was refused: %d", got)
	}

	if len(e.tv.images) != 1 {
		t.Fatalf("expected one placement, got %d", len(e.tv.images))
	}
	p := e.tv.images[0]
	// Not scaled: the picture is its own size in cells, which at ten by
	// twenty is 13 by 5, put down at the corner and shifted by the offset
	// in whole cells — one column across, none down.
	if p.Col != 45 || p.Row != 6 {
		t.Errorf("corner: got %d,%d, want 45,6", p.Col, p.Row)
	}
	if p.Cols != 13 || p.Rows != 5 {
		t.Errorf("size: got %dx%d cells, want 13x5", p.Cols, p.Rows)
	}
}

// Without the flag the far corner is a corner and the picture is scaled to
// cover the area, which is the other half of the same field.
func TestFar2lImageScaledToArea(t *testing.T) {
	e := newSixelEnv(t)
	req := far2lRequest(1, 's', func(stk *vtinput.Far2lStack) {
		stk.PushBytes(make([]byte, 8*8*3))
		stk.PushU32(8)
		stk.PushU32(8)
		stk.PushU16(16) // bottom row
		stk.PushU16(10) // right column
		stk.PushU16(6)
		stk.PushU16(4)
		stk.PushU64(wpImgRGB)
		stk.PushString("image_viewer")
	})
	if got := far2lSend(t, e, req).PopU8(); got != 1 {
		t.Fatalf("refused: %d", got)
	}
	p := e.tv.images[0]
	if p.Col != 4 || p.Row != 6 || p.Cols != 7 || p.Rows != 11 {
		t.Errorf("got %d,%d %dx%d, want 4,6 7x11", p.Col, p.Row, p.Cols, p.Rows)
	}
}
