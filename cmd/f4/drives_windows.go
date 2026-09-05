//go:build windows

package main

import (
	"os"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/f4/vfs/hostmode"
	"golang.org/x/sys/windows"
)

func getPlatformDrives() []DriveEntry {
	if hostmode.Posix() {
		// No drive letters in posix mode -- the whole point of WINE.md
		// Part E is that under Wine this looks like the Linux build, not
		// like Windows-with-extra-steps. Mirrors drives_unix.go exactly.
		home := os.Getenv("HOME")
		drives := []DriveEntry{
			{Name: "/ Root", Factory: func() vfs.VFS { return vfs.NewOSVFS("/") }},
		}
		if home != "" {
			drives = append(drives, DriveEntry{Name: "~ Home", Factory: func() vfs.VFS { return vfs.NewOSVFS(home) }})
		}
		return drives
	}
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
