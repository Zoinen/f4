package main

import "io"

// PtyBackend abstracts the difference between Unix PTY and Windows ConPTY.
type PtyBackend interface {
	io.ReadWriter
	io.Closer
	SetSize(cols, rows int)
	Wait() error
	Run(name string, args ...string) error
	IsBusy() bool
}

// PtyPixelSizer is implemented by backends that can report the size of the
// window in pixels next to its size in cells. A program that draws images
// reads it from the window size of the terminal; a zero there leaves it
// guessing the shape of a character cell, and a wrong guess distorts every
// picture it draws.
type PtyPixelSizer interface {
	SetSizePixels(cols, rows, xpixel, ypixel int)
}

// setPtySize tells the child how large its terminal is, in pixels as well
// whenever both the backend and the host can say.
func setPtySize(p PtyBackend, cols, rows, cellW, cellH int) {
	if p == nil {
		return
	}
	if sizer, ok := p.(PtyPixelSizer); ok && cellW > 0 && cellH > 0 {
		sizer.SetSizePixels(cols, rows, cols*cellW, rows*cellH)
		return
	}
	p.SetSize(cols, rows)
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
