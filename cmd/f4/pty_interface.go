package main

import (
	"io"
	"sync/atomic"
)

// PtyBackend abstracts the difference between Unix PTY and Windows ConPTY.
type PtyBackend interface {
	io.ReadWriter
	io.Closer
	SetSize(cols, rows int)
	Wait() error
	Run(name string, args ...string) error
	IsBusy() bool
}

// newLocalPTY is injectable so unit tests that construct hundreds of panel
// frames do not spawn hundreds of real interactive shells. Direct PTY tests
// continue to call NewPTY and therefore still cover the platform backend.
var newLocalPTY = func() (PtyBackend, error) {
	return NewPTY()
}

// PtyPixelSizer is implemented by backends that can report the size of the
// window in pixels next to its size in cells. A program that draws images
// reads it from the window size of the terminal; a zero there leaves it
// guessing the shape of a character cell, and a wrong guess distorts every
// picture it draws.
type PtyPixelSizer interface {
	SetSizePixels(cols, rows, xpixel, ypixel int)
}

// Optional acknowledgements let the presentation layer suppress a repeated
// resize only after the backend confirms it. Legacy void setters remain valid
// but must not be remembered as successfully negotiated geometry.
type PtySizeAcknowledger interface {
	SetSizeChecked(cols, rows int) error
}

type PtyPixelSizeAcknowledger interface {
	SetSizePixelsChecked(cols, rows, xpixel, ypixel int) error
}

// setPtySize tells the child how large its terminal is, in pixels as well
// whenever both the backend and the host can say.
func setPtySize(p PtyBackend, cols, rows, cellW, cellH int) (acknowledged bool, err error) {
	if p == nil {
		return false, nil
	}
	if sizer, ok := p.(PtyPixelSizer); ok && cellW > 0 && cellH > 0 {
		if checked, ok := p.(PtyPixelSizeAcknowledger); ok {
			return true, checked.SetSizePixelsChecked(cols, rows, cols*cellW, rows*cellH)
		}
		sizer.SetSizePixels(cols, rows, cols*cellW, rows*cellH)
		return false, nil
	}
	if checked, ok := p.(PtySizeAcknowledger); ok {
		return true, checked.SetSizeChecked(cols, rows)
	}
	p.SetSize(cols, rows)
	return false, nil
}

// ptyPixels clamps a pixel dimension to what the winsize structure carries.
func ptyPixels(v int) uint16 {
	if v < 0 {
		return 0
	}
	if v > 0xFFFF {
		return 0xFFFF
	}
	return uint16(v)
}

// livePTYCount tracks pseudo-terminals opened by any platform's NewPTY
// that have not yet been closed. It exists purely as a leak detector:
// a PTY that is opened but never Close()'d leaves a live goroutine
// blocked in Master.Read() and a real pty slot allocated for the rest
// of the process lifetime. In `go test ./...`, where every test in a
// package shares one process, that adds up test by test until the
// system's pty limit is exhausted and unrelated, timing-sensitive
// tests start failing for no reason connected to what they actually
// test. See zzz_pty_leak_check_test.go for where this is reported.
var livePTYCount int64

func registerPTYOpened() {
	atomic.AddInt64(&livePTYCount, 1)
}

func registerPTYClosed() {
	atomic.AddInt64(&livePTYCount, -1)
}

// LivePTYCount reports how many PTYs opened through any platform's
// NewPTY are still considered open right now.
func LivePTYCount() int64 {
	return atomic.LoadInt64(&livePTYCount)
}
