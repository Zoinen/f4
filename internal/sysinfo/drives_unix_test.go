//go:build !windows

package sysinfo

import (
	"runtime"
	"strings"
	"testing"
)

func TestGetPlatformDrives(t *testing.T) {
	drives := GetPlatformDrives()
	wantNames := []string{"/ Root", "~ Home"}
	if runtime.GOOS != "darwin" {
		wantNames = append(wantNames, "Physical Disks (/dev)")
	}
	if len(drives) < len(wantNames) {
		t.Fatalf("GetPlatformDrives() returned %d entries, want at least %d built-ins", len(drives), len(wantNames))
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

	for _, want := range wantNames {
		if !names[want] {
			t.Errorf("GetPlatformDrives() missing built-in entry %q, got names %v", want, names)
		}
	}
	if runtime.GOOS == "darwin" && names["Physical Disks (/dev)"] {
		t.Fatal("unsupported physical disk browser is exposed on macOS")
	}

	// Live mounts carry navigation metadata. Only Linux implements unmount.
	for _, d := range drives {
		if d.Name == "/ Root" || d.Name == "~ Home" || d.Name == "Physical Disks (/dev)" {
			continue
		}
		if runtime.GOOS == "linux" && d.UnmountDevice == "" {
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
