//go:build windows

package main

import "syscall"

// lockFileExclusively opens path with no sharing at all, so any rename,
// delete or write from another handle fails with a sharing violation.
// Go's own os.OpenFile always passes FILE_SHARE_READ|WRITE|DELETE and
// therefore cannot express this.
func lockFileExclusively(path string) (func(), error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := syscall.CreateFile(p,
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		0, // no sharing
		nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	return func() { syscall.CloseHandle(h) }, nil
}
