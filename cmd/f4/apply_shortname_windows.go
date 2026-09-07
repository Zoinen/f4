//go:build windows

package main

import "golang.org/x/sys/windows"

func applyCommandShortPath(path string) string {
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return path
	}
	size, err := windows.GetShortPathName(ptr, nil, 0)
	if err != nil || size == 0 {
		return path
	}
	buf := make([]uint16, size)
	size, err = windows.GetShortPathName(ptr, &buf[0], uint32(len(buf)))
	if err != nil || size == 0 {
		return path
	}
	return windows.UTF16ToString(buf[:size])
}
