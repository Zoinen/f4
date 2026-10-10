//go:build windows

package vtui

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var procGetCurrentConsoleFontEx = kernel32.NewProc("GetCurrentConsoleFontEx")

// consoleFontInfoEx is CONSOLE_FONT_INFOEX.
type consoleFontInfoEx struct {
	cbSize     uint32
	nFont      uint32
	fontSize   [2]int16
	fontFamily uint32
	fontWeight uint32
	faceName   [32]uint16
}

const tmpfTrueType = 0x04 // TMPF_TRUETYPE in the font family of a TrueType font

// consoleFontIsRaster reports whether the console the process writes to uses a
// raster (bitmap) font. Without a console, or if the call fails, it says no:
// the default glyph is kept.
func consoleFontIsRaster() bool {
	h, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil || h == 0 || h == windows.InvalidHandle {
		return false
	}
	info := consoleFontInfoEx{cbSize: uint32(unsafe.Sizeof(consoleFontInfoEx{}))}
	r, _, _ := procGetCurrentConsoleFontEx.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		return false
	}
	return info.fontFamily&tmpfTrueType == 0
}
