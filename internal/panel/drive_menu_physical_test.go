package panel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/sysinfo"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// f4#1148 (mitek1977-coder, 15.10, point 5): Physical Disks is a tool, not a
// drive: it is the first row under Tools & Plugins, not in the Drive section,
// and it has a hot key action of its own for F4 and the Tools page.
func TestPhysicalDisksIsTheFirstTool(t *testing.T) {
	oldEvery, oldDrives := driveMenuWatchEvery, driveMenuPlatformDrives
	t.Cleanup(func() { driveMenuWatchEvery, driveMenuPlatformDrives = oldEvery, oldDrives })
	driveMenuWatchEvery = 0
	cfg := t.TempDir()
	oldUserConfigDir := config.UserConfigDir
	config.UserConfigDir = func() (string, error) { return cfg, nil }
	t.Cleanup(func() { config.UserConfigDir = oldUserConfigDir })
	if err := os.MkdirAll(filepath.Join(cfg, "f4", "settings"), 0o700); err != nil {
		t.Fatal(err)
	}
	restore := sysinfo.SnapshotDrives()
	t.Cleanup(restore)
	sysinfo.SetDrives(nil)
	driveMenuPlatformDrives = func() []sysinfo.DriveEntry {
		return []sysinfo.DriveEntry{
			{Name: "/ Root", Factory: func() vfs.VFS { return nil }},
			{Name: "Physical Disks (/dev)", Factory: func() vfs.VFS { return nil }},
		}
	}

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	t.Cleanup(pf.Close)
	pf.ResizeConsole(100, 30)
	pf.ShowDriveMenu(0)
	menu, ok := driveMenuFromFrame(vtui.FrameManager.GetTopFrame())
	if !ok {
		t.Fatal("no drive menu on top")
	}
	tools := ruleRows(menu)[strings.TrimSpace(i18n.Msg("Drive.Tools"))]
	if tools == 0 {
		t.Fatal("no Tools rule")
	}
	for i := 0; i < tools; i++ {
		if strings.Contains(menu.Items[i].Text, "Physical Disks") {
			t.Errorf("Physical Disks is still in the Drive section, row %d: %q", i, menu.Items[i].Text)
		}
	}
	if first := strings.TrimSpace(menu.Items[tools+1].Text); first != "Physical Disks (/dev)" {
		t.Errorf("first row under Tools is %q, want Physical Disks", first)
	}
	if got, want := DriveToolActionName(driveToolKeyPhysicalDisks), keymap.DriveMenuActionName("platform.physical-disks"); got != want {
		t.Errorf("hot key action %q, want %q", got, want)
	}
	if choices := DriveMenuToolChoices(); len(choices) == 0 || choices[0].Key != driveToolKeyPhysicalDisks {
		t.Errorf("the Tools page lists %+v first, want Physical Disks", choices)
	}
}
