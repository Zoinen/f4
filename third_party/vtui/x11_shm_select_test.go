//go:build (linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || illumos || solaris) && !android

package vtui

import (
	"errors"
	"image"
	"testing"

	"github.com/jezek/xgb/shm"
	"github.com/jezek/xgb/xproto"
)

// f4 #1626: under GNOME's Xwayland the window stopped updating once it was
// maximized while MIT-SHM was on, and the X errors that would have said why
// were dropped by RunEventLoop. These tests cover the decisions that do not
// need an X server: which errors turn SHM off, when a put is checked, and what
// the fallback leaves behind.

const testSHMMajor = 130

func TestX11IsSHMError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"value error from ShmPutImage", xproto.ValueError{MajorOpcode: testSHMMajor, MinorOpcode: 3}, true},
		{"match error from ShmPutImage", xproto.MatchError{MajorOpcode: testSHMMajor, MinorOpcode: 3}, true},
		{"access error from ShmPutImage", xproto.AccessError{MajorOpcode: testSHMMajor, MinorOpcode: 3}, true},
		{"pointer to an SHM error", &xproto.ValueError{MajorOpcode: testSHMMajor}, true},
		{"BadSeg", shm.BadSegError{MajorOpcode: testSHMMajor}, true},
		{"core PutImage error (X_PutImage = 72)", xproto.MatchError{MajorOpcode: 72}, false},
		{"unrelated window error", xproto.WindowError{MajorOpcode: 20}, false},
		{"not an X error", errors.New("connection reset"), false},
		{"nil pointer", (*xproto.ValueError)(nil), false},
	}
	for _, c := range cases {
		if got := x11IsSHMError(c.err, testSHMMajor); got != c.want {
			t.Errorf("%s: x11IsSHMError = %v, want %v", c.name, got, c.want)
		}
	}
	if x11IsSHMError(xproto.ValueError{MajorOpcode: 0}, 0) {
		t.Error("an error must not be taken for an SHM one when MIT-SHM was never initialized")
	}
}

func TestX11SHMNeedsCheckOncePerSize(t *testing.T) {
	if !x11SHMNeedsCheck(1440, 832, 0, 0) {
		t.Error("the first SHM put must be checked")
	}
	if x11SHMNeedsCheck(1440, 832, 1440, 832) {
		t.Error("a put at an already verified size must not wait for the server")
	}
	// Maximizing 180x52 -> 202x59 cells of 8x16 (the sizes in the f4 #1626 log).
	if !x11SHMNeedsCheck(1616, 944, 1440, 832) {
		t.Error("the first put after a resize must be checked")
	}
	if !x11SHMNeedsCheck(1440, 944, 1440, 832) || !x11SHMNeedsCheck(1616, 832, 1440, 832) {
		t.Error("a change of either dimension must be checked")
	}
}

func TestX11SHMFits(t *testing.T) {
	seg := 3840 * 2160 * 4
	if !x11SHMFits(1616, 944, seg) || !x11SHMFits(3840, 2160, seg) {
		t.Error("images up to the segment size must fit")
	}
	if x11SHMFits(3840, 2161, seg) || x11SHMFits(5120, 2880, seg) {
		t.Error("an image larger than the segment must not fit")
	}
	if x11SHMFits(0, 10, seg) || x11SHMFits(10, 0, seg) {
		t.Error("an empty image must not be sent through SHM")
	}
}

func newSHMTestHost(w, h int) *X11Host {
	return &X11Host{
		shmSeg:       7,
		shmMajor:     testSHMMajor,
		shmVerifiedW: w,
		shmVerifiedH: h,
		imgBuf:       image.NewRGBA(image.Rect(0, 0, w, h)),
		bgraBuf:      make([]byte, 16),
		dirtyLines:   make([]bool, h),
	}
}

func TestX11DisableSHMFallsBackToCorePutImage(t *testing.T) {
	h := newSHMTestHost(12, 5)
	h.disableSHMLocked("test")

	if h.shmSeg != 0 {
		t.Fatalf("shmSeg = %d after fallback, want 0", h.shmSeg)
	}
	if h.shmVerifiedW != 0 || h.shmVerifiedH != 0 {
		t.Errorf("verified size = %dx%d after fallback, want reset", h.shmVerifiedW, h.shmVerifiedH)
	}
	if len(h.bgraBuf) != len(h.imgBuf.Pix) {
		t.Errorf("bgraBuf len = %d, want a private buffer of %d bytes", len(h.bgraBuf), len(h.imgBuf.Pix))
	}
	for y, d := range h.dirtyLines {
		if !d {
			t.Fatalf("line %d not marked dirty: the frames lost to SHM must be repainted", y)
		}
	}

	// A second call is a no-op and must not reallocate.
	buf := h.bgraBuf
	h.dirtyLines[0] = false
	h.disableSHMLocked("again")
	if &h.bgraBuf[0] != &buf[0] || h.dirtyLines[0] {
		t.Error("disabling SHM twice must leave the core path alone")
	}
}

func TestX11HandleXErrorDisablesSHMOnlyForSHMErrors(t *testing.T) {
	h := newSHMTestHost(8, 4)
	h.handleXError(xproto.WindowError{MajorOpcode: 20})
	if h.shmSeg == 0 {
		t.Fatal("an unrelated X error must not turn SHM off")
	}

	h.handleXError(xproto.ValueError{MajorOpcode: testSHMMajor, MinorOpcode: 3})
	if h.shmSeg != 0 {
		t.Fatal("a ShmPutImage error must turn SHM off")
	}
	if len(h.bgraBuf) != len(h.imgBuf.Pix) {
		t.Errorf("bgraBuf len = %d after fallback, want %d", len(h.bgraBuf), len(h.imgBuf.Pix))
	}
}

func TestX11EnsureBGRABufFollowsResize(t *testing.T) {
	// Core path: the private buffer follows the image size.
	h := &X11Host{imgBuf: image.NewRGBA(image.Rect(0, 0, 10, 3)), dirtyLines: make([]bool, 3)}
	h.ensureBGRABufLocked()
	if len(h.bgraBuf) != len(h.imgBuf.Pix) {
		t.Fatalf("bgraBuf len = %d, want %d", len(h.bgraBuf), len(h.imgBuf.Pix))
	}
	h.imgBuf = image.NewRGBA(image.Rect(0, 0, 20, 6))
	h.ensureBGRABufLocked()
	if len(h.bgraBuf) != len(h.imgBuf.Pix) {
		t.Fatalf("bgraBuf len = %d after grow, want %d", len(h.bgraBuf), len(h.imgBuf.Pix))
	}

	// SHM path with an image the segment cannot hold: fall back rather than
	// silently skip the rows past the segment's end.
	saved := shmData
	defer func() { shmData = saved }()
	shmData = make([]byte, 10*3*4)
	h = newSHMTestHost(20, 6)
	h.ensureBGRABufLocked()
	if h.shmSeg != 0 {
		t.Fatal("an image larger than the segment must switch to core PutImage")
	}
	if len(h.bgraBuf) != len(h.imgBuf.Pix) {
		t.Errorf("bgraBuf len = %d, want %d", len(h.bgraBuf), len(h.imgBuf.Pix))
	}

	// SHM path with an image that fits: the segment stays the buffer.
	shmData = make([]byte, 64*64*4)
	h = newSHMTestHost(20, 6)
	h.ensureBGRABufLocked()
	if h.shmSeg == 0 || len(h.bgraBuf) != len(shmData) {
		t.Errorf("SHM must stay on while the image fits (shmSeg=%d, len=%d)", h.shmSeg, len(h.bgraBuf))
	}
}
