//go:build windows

package vfs

import "golang.org/x/sys/windows"

// NativeEntryName returns the spelling of a known local file in its directory.
// Unlike os.Stat().Name(), FindFirstFile also expands short names and preserves
// the casing of the directory entry without following symlinks.
func NativeEntryName(path, fallback string) string {
	if !WindowsPersonality() {
		return fallback
	}
	name, err := windows.UTF16PtrFromString(prepareOSPath(path))
	if err != nil {
		return fallback
	}
	var data windows.Win32finddata
	handle, err := windows.FindFirstFile(name, &data)
	if err != nil {
		return fallback
	}
	_ = windows.FindClose(handle)
	return windows.UTF16ToString(data.FileName[:])
}
