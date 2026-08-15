//go:build windows

package vtui

import (
	"syscall"
	"unsafe"
)

func getSystemScrollLines() int {
	user32 := syscall.NewLazyDLL("user32.dll")
	proc := user32.NewProc("SystemParametersInfoW")
	if proc.Find() == nil {
		var lines uint32
		r1, _, _ := proc.Call(
			0x0068, // SPI_GETWHEELSCROLLLINES
			0,
			uintptr(unsafe.Pointer(&lines)),
			0,
		)
		if r1 != 0 && lines > 0 {
			if lines == 0xFFFFFFFF { // WHEEL_PAGESCROLL
				return 3
			}
			return int(lines)
		}
	}
	return 3
}
