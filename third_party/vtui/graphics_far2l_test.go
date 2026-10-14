package vtui

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/unxed/vtinput"
)

func decodeFar2lAPC(t *testing.T, raw string) vtinput.Far2lStack {
	t.Helper()
	if !strings.HasPrefix(raw, "\x1b_far2l:") || !strings.HasSuffix(raw, "\x07") {
		t.Fatalf("not a far2l APC: %q", raw)
	}
	b64 := strings.TrimSuffix(strings.TrimPrefix(raw, "\x1b_far2l:"), "\x07")
	payload, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("decode far2l APC: %v", err)
	}
	return vtinput.Far2lStack(payload)
}

func TestFar2lGraphicsProtocolSelection(t *testing.T) {
	old := Far2lEnabled
	Far2lEnabled = true
	t.Cleanup(func() { Far2lEnabled = old })

	var layer GraphicsLayer
	got := layer.Protocol()
	if got != GraphicsFar2l {
		t.Fatalf("far2l must take precedence over inherited kitty markers, got %v", got)
	}
	if p, ok := ParseGraphicsProtocol("far2l"); !ok || p != GraphicsFar2l {
		t.Fatalf("far2l protocol is not parseable: %v, %v", p, ok)
	}
}

func TestFar2lAcknowledgementUpdatesExistingScreen(t *testing.T) {
	oldEnabled := Far2lEnabled
	Far2lEnabled = false
	t.Cleanup(func() { Far2lEnabled = oldEnabled })

	fm := &frameManager{}
	scr := NewSilentScreenBuf()
	scr.Graphics().SetProtocol(GraphicsNone)
	fm.Init(scr)
	fm.dispatchEvent(&vtinput.InputEvent{Type: vtinput.Far2lEventType, Far2lCommand: "ok"}, false)

	if got := scr.Graphics().Protocol(); got != GraphicsFar2l {
		t.Fatalf("late far2l acknowledgement left protocol at %v", got)
	}
}

func TestFar2lEncoderSetUsesPackedCropAndInclusiveArea(t *testing.T) {
	oldID := far2lIDCounter.Load()
	far2lIDCounter.Store(0)
	t.Cleanup(func() { far2lIDCounter.Store(oldID) })

	// Two padded rows: the command must contain only the selected 2x2 crop,
	// not the source stride or pixels outside the source rectangle.
	pix := make([]byte, 32)
	copy(pix[0:], []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12})
	copy(pix[16:], []byte{13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24})
	surf := NewImageSurfaceFromPix(3, 2, 16, pix)
	e := newFar2lEncoder()
	var out byteBuffer
	e.Render(&out, []ImagePlacement{{
		ID: 7, Surface: surf, Col: 2, Row: 3, Cols: 4, Rows: 5,
		SrcX: 1, SrcY: 0, SrcW: 2, SrcH: 2,
	}})

	stk := decodeFar2lAPC(t, out.String())
	if id := stk.PopU8(); id == 0 {
		t.Fatal("request id must be non-zero")
	}
	if got := stk.PopU8(); got != far2lImageCategory {
		t.Fatalf("category = %q, want %q", got, far2lImageCategory)
	}
	if got := stk.PopU8(); got != far2lImageSet {
		t.Fatalf("subcommand = %q, want %q", got, far2lImageSet)
	}
	if got := stk.PopString(); got != far2lImageIdentity(7) {
		t.Fatalf("identity = %q", got)
	}
	if got := stk.PopU64(); got != far2lImageRGBA {
		t.Fatalf("flags = %#x", got)
	}
	if left, top, right, bottom := stk.PopU16(), stk.PopU16(), stk.PopU16(), stk.PopU16(); left != 2 || top != 3 || right != 5 || bottom != 7 {
		t.Fatalf("area = %d,%d..%d,%d", left, top, right, bottom)
	}
	if w, h := stk.PopU32(), stk.PopU32(); w != 2 || h != 2 {
		t.Fatalf("dimensions = %dx%d", w, h)
	}
	want := []byte{5, 6, 7, 8, 9, 10, 11, 12, 17, 18, 19, 20, 21, 22, 23, 24}
	if got := stk.PopBytes(len(want)); string(got) != string(want) {
		t.Fatalf("cropped RGBA = %v, want %v", got, want)
	}
}

func TestFar2lEncoderDeletesStaleImages(t *testing.T) {
	oldID := far2lIDCounter.Load()
	far2lIDCounter.Store(0)
	t.Cleanup(func() { far2lIDCounter.Store(oldID) })

	e := newFar2lEncoder()
	surf := NewImageSurface(1, 1)
	var out byteBuffer
	e.Render(&out, []ImagePlacement{{ID: 9, Surface: surf, Cols: 1, Rows: 1}})
	out.Reset()
	e.Render(&out, nil)

	stk := decodeFar2lAPC(t, out.String())
	_ = stk.PopU8()
	if got := stk.PopU8(); got != far2lImageCategory {
		t.Fatalf("category = %q", got)
	}
	if got := stk.PopU8(); got != far2lImageDelete {
		t.Fatalf("subcommand = %q", got)
	}
	if got := stk.PopString(); got != far2lImageIdentity(9) {
		t.Fatalf("identity = %q", got)
	}
}

func TestScreenBufFlushFar2lGraphics(t *testing.T) {
	oldID := far2lIDCounter.Load()
	far2lIDCounter.Store(0)
	t.Cleanup(func() { far2lIDCounter.Store(oldID) })

	scr := NewScreenBuf()
	var out bytes.Buffer
	scr.Writer = &out
	scr.AllocBuf(4, 2)
	scr.Graphics().SetProtocol(GraphicsFar2l)
	scr.Graphics().Add(ImagePlacement{ID: 1, Surface: NewImageSurface(1, 1), Cols: 1, Rows: 1})
	scr.Flush()

	if !strings.Contains(out.String(), "\x1b_far2l:") {
		t.Fatal("far2l renderer did not append an APC to the frame")
	}
}

// far2l keeps an image until it is deleted, so leaving f4 (Suspend) must
// delete what the encoder left on far2l's screen; otherwise the last picture
// stays over far2l's own panels (f4#1885).
func TestFar2lImagesDroppedOnSuspend(t *testing.T) {
	oldID := far2lIDCounter.Load()
	far2lIDCounter.Store(0)
	t.Cleanup(func() { far2lIDCounter.Store(oldID) })
	t.Cleanup(func() { far2lHostImages.take() })

	e := newFar2lEncoder()
	var out byteBuffer
	e.Render(&out, []ImagePlacement{{ID: 7, Surface: NewImageSurface(1, 1), Cols: 1, Rows: 1}})

	var sb strings.Builder
	dropFar2lHostImages(&sb)
	stk := decodeFar2lAPC(t, sb.String())
	_ = stk.PopU8()
	if stk.PopU8() != far2lImageCategory || stk.PopU8() != far2lImageDelete || stk.PopString() != far2lImageIdentity(7) {
		t.Fatalf("Suspend did not delete the image left on far2l's screen: %q", sb.String())
	}

	sb.Reset()
	dropFar2lHostImages(&sb)
	if sb.Len() != 0 {
		t.Fatalf("a second drop must send nothing, sent %q", sb.String())
	}

	e.Render(&out, nil)
	dropFar2lHostImages(&sb)
	if sb.Len() != 0 {
		t.Fatalf("nothing left on screen, yet the drop sent %q", sb.String())
	}
}

// Suspend deletes far2l's images before inputRestore turns far2l's
// extensions off: far2l ignores the delete once they are off, and quitting
// f4 with a picture in the quick view left it on screen (f4#1885).
func TestSuspendDropsFar2lImagesBeforeLeavingExtensions(t *testing.T) {
	mock := &mockTermOut{}
	oldGetTermOut := getTermOut
	getTermOut = func() interface {
		WriteString(string) (int, error)
		Sync() error
	} {
		return mock
	}
	t.Cleanup(func() { getTermOut = oldGetTermOut })
	oldUsesVT := consoleUsesVT
	consoleUsesVT = func() bool { return true }
	t.Cleanup(func() { consoleUsesVT = oldUsesVT })
	t.Cleanup(func() { far2lHostImages.take() })

	var out byteBuffer
	newFar2lEncoder().Render(&out, []ImagePlacement{{ID: 5, Surface: NewImageSurface(1, 1), Cols: 1, Rows: 1}})

	isPrepared = true
	inputRestore = func() { _, _ = mock.WriteString("<far2l off>") }
	t.Cleanup(func() { isPrepared = false; inAltScreen = false; inputRestore = nil })
	Suspend()

	written := mock.builder.String()
	del, off := strings.Index(written, "\x1b_far2l:"), strings.Index(written, "<far2l off>")
	if del < 0 || off < 0 || del > off {
		t.Fatalf("the image delete must reach far2l before its extensions go off: %q", written)
	}
}
