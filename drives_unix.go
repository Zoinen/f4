//go:build !windows

package main

import (
	"os"
	"runtime"

	"github.com/unxed/f4/vfs"
)

func getPlatformDrives() []DriveEntry {
	home, _ := os.UserHomeDir()
	drives := []DriveEntry{
		{Name: "/ Root", Icon: "folder-root", Factory: func() vfs.VFS { return vfs.NewOSVFS("/") }},
		{Name: "~ Home", Icon: "house", Factory: func() vfs.VFS { return vfs.NewOSVFS(home) }},
	}
	if runtime.GOOS != "darwin" {
		drives = append(drives, DriveEntry{
			Name:    "Physical Disks (/dev)",
			Icon:    driveMenuIconPhysical,
			Factory: func() vfs.VFS { return vfs.NewDisksVFS() },
		})
	}
	return drives
}
