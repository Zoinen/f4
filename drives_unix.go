//go:build !windows

package main

import "os"
import "github.com/unxed/f4/vfs"

func getPlatformDrives() []DriveEntry {
	home, _ := os.UserHomeDir()
	return []DriveEntry{
		{Name: "/ Root", Factory: func() vfs.VFS { return vfs.NewOSVFS("/") }},
		{Name: "~ Home", Factory: func() vfs.VFS { return vfs.NewOSVFS(home) }},
		{Name: "Physical Disks (/dev)", Factory: func() vfs.VFS { return vfs.NewDisksVFS() }},
	}
}
