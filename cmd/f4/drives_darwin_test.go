package main

import (
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestPlatformDrivesHidePhysicalDisksOnDarwin(t *testing.T) {
	for _, drive := range getPlatformDrives() {
		if strings.Contains(drive.Name, "Physical Disks") {
			t.Fatalf("physical-disk menu entry is exposed on macOS: %q", drive.Name)
		}
		if drive.Factory == nil {
			continue
		}
		if _, ok := drive.Factory().(*vfs.DisksVFS); ok {
			t.Fatalf("physical-disk VFS factory is exposed on macOS as %q", drive.Name)
		}
	}
}
