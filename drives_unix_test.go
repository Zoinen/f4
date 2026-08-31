//go:build !windows

package main

import "testing"

func TestPlatformDriveRootAndHomeIcons(t *testing.T) {
	drives := getPlatformDrives()
	want := map[string]string{
		"/ Root": "folder-root",
		"~ Home": "house",
	}
	for _, drive := range drives {
		if icon, ok := want[drive.Name]; ok {
			if drive.Icon != icon {
				t.Fatalf("drive %q icon = %q, want %q", drive.Name, drive.Icon, icon)
			}
			delete(want, drive.Name)
		}
	}
	for name := range want {
		t.Errorf("platform drive %q is missing", name)
	}
}
