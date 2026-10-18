package panel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/sysinfo"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// f4#1148 (mitek1977-coder, 15.10, point 3): "Show removable / CD / remote
// drive parameters" hide only the label, the file system and the size of such
// a drive; the drive itself always stays in the list.
func TestDriveMenuKindOptionsHideOnlyTheParameters(t *testing.T) {
	kinds := map[string]driveMenuKind{"/media/usb": driveMenuKindRemovable, "/media/cd": driveMenuKindCD, "/mnt/net": driveMenuKindRemote, "/data": driveMenuKindFixed}
	oldKind, oldFS, oldDrives := driveMenuRowKind, driveMenuRowFS, driveMenuPlatformDrives
	t.Cleanup(func() { driveMenuRowKind, driveMenuRowFS, driveMenuPlatformDrives = oldKind, oldFS, oldDrives })
	driveMenuRowKind = func(_, path string) driveMenuKind { return kinds[path] }
	driveMenuRowFS = func(path string) (sysinfo.FSInfo, bool) {
		return sysinfo.FSInfo{Total: 8 << 30, Free: 4 << 30, Type: "vfat", Label: "LABEL"}, true
	}
	drives := []sysinfo.DriveEntry{
		{Name: "USB stick", InfoPath: "/media/usb", Factory: func() vfs.VFS { return nil }},
		{Name: "CD", InfoPath: "/media/cd", Factory: func() vfs.VFS { return nil }},
		{Name: "Share", InfoPath: "/mnt/net", Factory: func() vfs.VFS { return nil }},
		{Name: "Data", InfoPath: "/data", Factory: func() vfs.VFS { return nil }},
	}
	all := config.DriveMenuShowLabel | config.DriveMenuShowFilesystem | config.DriveMenuShowSize
	kindOptions := config.DriveMenuShowRemovable | config.DriveMenuShowCD | config.DriveMenuShowRemote

	for _, drv := range drives {
		on := driveMenuPlatformRowFor(drv, all|kindOptions)
		if on.Label == "" || on.Filesystem == "" || on.Total == "" {
			t.Errorf("%s with its option on: %+v, want the label, the file system and the size", drv.Name, on)
		}
		off := driveMenuPlatformRowFor(drv, all)
		hidden := off.Label == "" && off.Filesystem == "" && off.Total == "" && off.Free == ""
		if wantHidden := drv.Name != "Data"; hidden != wantHidden {
			t.Errorf("%s with the kind options off: %+v, parameters hidden %v, want %v", drv.Name, off, hidden, wantHidden)
		}
	}

	// In the menu itself every drive is listed with the kind options off.
	cfg := t.TempDir()
	oldUserConfigDir := config.UserConfigDir
	config.UserConfigDir = func() (string, error) { return cfg, nil }
	t.Cleanup(func() { config.UserConfigDir = oldUserConfigDir })
	if err := os.MkdirAll(filepath.Join(cfg, "f4", "settings"), 0o700); err != nil {
		t.Fatal(err)
	}
	oldOptions := config.App.DriveMenuOptions
	t.Cleanup(func() { config.App.DriveMenuOptions = oldOptions })
	config.App.DriveMenuOptions = all
	restore := sysinfo.SnapshotDrives()
	t.Cleanup(restore)
	sysinfo.SetDrives(nil)
	driveMenuPlatformDrives = func() []sysinfo.DriveEntry { return drives }
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	t.Cleanup(pf.Close)
	pf.ResizeConsole(120, 30)
	pf.ShowDriveMenu(0)
	menu, ok := driveMenuFromFrame(vtui.FrameManager.GetTopFrame())
	if !ok {
		t.Fatal("no drive menu on top")
	}
	var rows []string
	for _, item := range menu.Items {
		rows = append(rows, item.Text)
	}
	text := strings.Join(rows, "\n")
	for _, drv := range drives {
		if !strings.Contains(text, drv.Name) {
			t.Errorf("%s is not in the drive menu with the kind options off:\n%s", drv.Name, text)
		}
	}
}
