//go:build windows

package vfs

import (
	"os"

	"github.com/unxed/vtui"
	"golang.org/x/sys/windows"
)

// readReparseTag returns the reparse point tag of the entry at path, which
// info describes without following links. The Win32 documentation names
// FindFirstFile as the way to retrieve the tag: WIN32_FIND_DATA.dwReserved0
// holds it whenever dwFileAttributes carries FILE_ATTRIBUTE_REPARSE_POINT.
// FindFirstFile searches the parent directory for the entry instead of
// opening the entry itself. The path is prepared only once info shows a
// reparse point, so a plain listing pays nothing extra.
// Zero means not a reparse point or no tag available; a failed lookup is
// logged.
func readReparseTag(path string, info os.FileInfo) uint32 {
	if !isReparsePoint(info) {
		return 0
	}
	name, err := windows.UTF16PtrFromString(prepareOSPath(path))
	if err != nil {
		vtui.DebugLog("VFS: reparse tag of %q: %v", path, err)
		return 0
	}
	var data windows.Win32finddata
	handle, err := windows.FindFirstFile(name, &data)
	if err != nil {
		vtui.DebugLog("VFS: reparse tag of %q: FindFirstFile: %v", path, err)
		return 0
	}
	if err := windows.FindClose(handle); err != nil {
		vtui.DebugLog("VFS: reparse tag of %q: FindClose: %v", path, err)
	}
	if data.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT == 0 {
		return 0
	}
	return data.Reserved0
}
