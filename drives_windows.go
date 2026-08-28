//go:build windows

package main

import (
	"github.com/unxed/f4/vfs"
	"golang.org/x/sys/windows"
)

func getPlatformDrives() []DriveEntry {
	var drives []DriveEntry
	bitmask, err := windows.GetLogicalDrives()
	if err != nil {
		return drives
	}
	for i := 0; i < 26; i++ {
		if bitmask&(1<<uint(i)) != 0 {
			letter := string(rune('A' + i))
			path := letter + ":\\"
			icon := driveMenuIconLocal
			if root, utf16Err := windows.UTF16PtrFromString(path); utf16Err == nil &&
				windows.GetDriveType(root) == windows.DRIVE_REMOTE {
				icon = driveMenuIconNetwork
			}
			drives = append(drives, DriveEntry{
				Name:    letter + ": Local",
				Icon:    icon,
				Factory: func() vfs.VFS { return vfs.NewOSVFS(path) },
			})
		}
	}
	drives = append(drives, DriveEntry{
		Name:    "Physical Disks",
		Icon:    driveMenuIconPhysical,
		Factory: func() vfs.VFS { return vfs.NewDisksVFS() },
	})
	return drives
}
