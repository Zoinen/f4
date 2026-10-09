//go:build windows

package sysinfo

import "testing"

// FS reads the volume fields through a handle to the volume's root rather
// than through GetVolumeInformationW, whose Wine implementation prints to the
// process's Unix stderr on every call (issue #474). The switch must not lose
// what the Info panel and the drive menu show: a real volume still reports
// its filesystem name, filename limit and serial.
func TestFSReportsVolumeInformation(t *testing.T) {
	info, ok := FS(t.TempDir())
	if !ok {
		t.Fatal("FS(temp dir) = not ok")
	}
	if info.Type == "" {
		t.Errorf("FS(temp dir).Type is empty")
	}
	if info.MaxFilename <= 0 {
		t.Errorf("FS(temp dir).MaxFilename = %d, want > 0", info.MaxFilename)
	}
	if info.Serial == "" {
		t.Errorf("FS(temp dir).Serial is empty")
	}
}
