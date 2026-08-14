//go:build windows

package vfs

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func resolveDevicePath(name string) string {
	if !strings.HasPrefix(name, "\\\\.\\") {
		return "\\\\.\\" + name
	}
	return name
}

func getPlatformBlockDevices(ctx context.Context) []VFSItem {
	var items []VFSItem
	for i := 0; i < 64; i++ {
		if ctx.Err() != nil {
			break
		}
		name := fmt.Sprintf("PhysicalDrive%d", i)
		diskPath := "\\\\.\\" + name
		ptr, err := windows.UTF16PtrFromString(diskPath)
		if err != nil {
			continue
		}
		h, err := windows.CreateFile(ptr, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
		if err == nil {
			var length int64
			var returned uint32
			errIoctl := windows.DeviceIoControl(h, 0x7405C, nil, 0, (*byte)(unsafe.Pointer(&length)), 8, &returned, nil)
			windows.CloseHandle(h)
			if errIoctl != nil || length <= 0 {
				length = 0
			}
			items = append(items, VFSItem{
				Name:      name,
				Size:      length,
				SizeKnown: true,
				MTime:     time.Now(),
			})
		}
	}
	return items
}

func getDeviceSize(devPath string, f *os.File) int64 {
	if f != nil {
		if pos, err := f.Seek(0, io.SeekEnd); err == nil && pos > 0 {
			f.Seek(0, io.SeekStart)
			return pos
		}
	}
	ptr, err := windows.UTF16PtrFromString(devPath)
	if err != nil {
		return 0
	}
	h, err := windows.CreateFile(ptr, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(h)
	var length int64
	var returned uint32
	if err := windows.DeviceIoControl(h, 0x7405C, nil, 0, (*byte)(unsafe.Pointer(&length)), 8, &returned, nil); err == nil {
		return length
	}
	return 0
}
