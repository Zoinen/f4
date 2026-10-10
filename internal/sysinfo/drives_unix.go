//go:build !windows

package sysinfo

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/unxed/f4/vfs"
)

func GetPlatformDrives() []DriveEntry {
	home, _ := os.UserHomeDir()
	drives := []DriveEntry{
		{Name: "/ Root", Icon: "folder-root", Factory: func() vfs.VFS { return vfs.NewOSVFS("/") }},
		{Name: "~ Home", Icon: "house", Factory: func() vfs.VFS { return vfs.NewOSVFS(home) }},
	}
	if runtime.GOOS != "darwin" {
		drives = append(drives, DriveEntry{
			Name:    "Physical Disks (/dev)",
			Icon:    DriveMenuIconPhysical,
			Factory: func() vfs.VFS { return vfs.NewDisksVFS() },
		})
	}
	// Live mount points (f4#415): re-read on every call, exactly like the
	// three entries above compute their own free-space metadata fresh each
	// time the menu opens, so a flash drive plugged in or unmounted since
	// the last Alt+F1 shows up, or drops out, right away -- no separate
	// polling or cache-invalidation machinery needed.
	for _, m := range UserMounts() {
		device, mountPoint := m.Device, m.MountPoint
		label := filepath.Base(mountPoint)
		drives = append(drives, DriveEntry{
			Name:          fmt.Sprintf("%s (%s)", label, device),
			InfoPath:      mountPoint,
			UnmountDevice: device,
			Factory:       func() vfs.VFS { return vfs.NewOSVFS(mountPoint) },
		})
	}
	return drives
}
