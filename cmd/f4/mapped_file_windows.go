//go:build windows

package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// mapFileRegion maps the whole file read-only. Windows keeps the file alive for
// as long as a view is open, so no other process can truncate it underneath the
// editor — the same protection the Unix build has to get from a fault handler,
// at the cost of the file reading as in-use to everything else.
func mapFileRegion(fd uintptr, size int64) ([]byte, error) {
	mapping, err := windows.CreateFileMapping(windows.Handle(fd), nil, windows.PAGE_READONLY, 0, 0, nil)
	if err != nil {
		return nil, err
	}
	// The view holds its own reference, so the mapping object is not needed
	// past this point.
	defer windows.CloseHandle(mapping)

	addr, err := windows.MapViewOfFile(mapping, windows.FILE_MAP_READ, 0, 0, uintptr(size))
	if err != nil {
		return nil, err
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(addr)), int(size)), nil
}

func unmapFileRegion(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	return windows.UnmapViewOfFile(uintptr(unsafe.Pointer(&data[0])))
}
