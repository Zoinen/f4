//go:build !windows

package sysinfo

import (
	"strings"
	"testing"
)

func TestGetPlatformDrives(t *testing.T) {
	drives := GetPlatformDrives()
	if len(drives) < 3 {
		t.Fatalf("GetPlatformDrives() returned %d entries, want at least the 3 built-in ones", len(drives))
	}

	names := make(map[string]bool, len(drives))
	for _, d := range drives {
		names[d.Name] = true
		if d.Factory == nil {
			t.Errorf("drive %q has a nil Factory", d.Name)
			continue
		}
		// Every factory must build something usable without panicking,
		// regardless of whether the underlying path exists on this runner.
		v := d.Factory()
		if v == nil {
			t.Errorf("drive %q Factory() returned a nil VFS", d.Name)
		}
	}

	for _, want := range []string{"/ Root", "~ Home", "Physical Disks (/dev)"} {
		if !names[want] {
			t.Errorf("GetPlatformDrives() missing built-in entry %q, got names %v", want, names)
		}
	}

	// Any entry beyond the three built-ins comes from a live mount point
	// (UserMounts) and must carry the device/mount metadata that lets the
	// drive menu offer to unmount it.
	for _, d := range drives {
		if d.Name == "/ Root" || d.Name == "~ Home" || d.Name == "Physical Disks (/dev)" {
			continue
		}
		if d.UnmountDevice == "" {
			t.Errorf("live mount entry %q has no UnmountDevice", d.Name)
		}
		if d.InfoPath == "" {
			t.Errorf("live mount entry %q has no InfoPath", d.Name)
		}
		if !strings.Contains(d.Name, d.UnmountDevice) {
			t.Errorf("live mount entry name %q does not mention its device %q", d.Name, d.UnmountDevice)
		}
	}
}
