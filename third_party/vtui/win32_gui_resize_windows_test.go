//go:build windows

package vtui

import "testing"

func TestWin32GuiHost_ResizeGridWithoutWindow(t *testing.T) {
	host := &Win32GuiHost{cols: 80, rows: 25, cellW: 8, cellH: 16}
	host.ResizeGrid(120, 40)
	if host.cols != 120 || host.rows != 40 {
		t.Fatalf("logical grid = %dx%d, want 120x40", host.cols, host.rows)
	}

	host.ResizeGrid(0, 40)
	host.ResizeGrid(120, 0)
	if host.cols != 120 || host.rows != 40 {
		t.Fatalf("invalid resize changed logical grid to %dx%d", host.cols, host.rows)
	}
}
