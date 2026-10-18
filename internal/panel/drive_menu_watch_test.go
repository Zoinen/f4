package panel

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/sysinfo"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// f4#1148 (mitek1977-coder, 15.10, point 4): a USB stick plugged in or pulled
// out while the drive menu is open shows up in it, the cursor stays on the row
// it was on, and a window opened over the menu (the settings of the drive
// chooser) stays on top. The watch's ticker is off here; the check it runs is
// called by hand.
func TestDriveMenuFollowsDrivesThatComeAndGo(t *testing.T) {
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
	drive := func(name string) sysinfo.DriveEntry {
		return sysinfo.DriveEntry{Name: name, Factory: func() vfs.VFS { return nil }}
	}
	before := []sysinfo.DriveEntry{drive("Alpha"), drive("Beta")}
	after := []sysinfo.DriveEntry{drive("Alpha"), drive("USB stick"), drive("Beta")}
	driveMenuPlatformDrives = func() []sysinfo.DriveEntry { return before }

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	t.Cleanup(pf.Close)
	pf.ResizeConsole(100, 30)
	pf.ShowDriveMenu(0)
	menu, ok := driveMenuFromFrame(vtui.FrameManager.GetTopFrame())
	if !ok {
		t.Fatal("no drive menu on top")
	}
	rowOf := func(m *vtui.VMenu, name string) int {
		for i, it := range m.Items {
			if strings.Contains(it.Text, name) {
				return i
			}
		}
		return -1
	}
	menu.SetSelectPos(rowOf(menu, "Beta"))

	// Nothing changed: the menu stays as it is and is still watched.
	if !pf.refreshDriveMenuIfChanged(0, menu, driveListSignature(before), driveListSignature(before)) {
		t.Fatal("an unchanged drive list stopped the watch")
	}
	if top, _ := driveMenuFromFrame(vtui.FrameManager.GetTopFrame()); top != menu {
		t.Fatal("an unchanged drive list rebuilt the menu")
	}

	// A window over the menu, as F9 opens the settings over it.
	over := vtui.NewCenteredDialog(30, 6, " Settings ")
	vtui.FrameManager.Push(over)

	driveMenuPlatformDrives = func() []sysinfo.DriveEntry { return after }
	if pf.refreshDriveMenuIfChanged(0, menu, driveListSignature(before), driveListSignature(after)) {
		t.Error("the replaced menu is still watched")
	}
	if vtui.FrameManager.GetTopFrame() != vtui.Frame(over) {
		t.Fatalf("the window over the menu is no longer on top: %T", vtui.FrameManager.GetTopFrame())
	}
	frames := vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx)
	rebuilt, ok := driveMenuFromFrame(frames[len(frames)-2])
	if !ok || rebuilt == menu {
		t.Fatalf("no rebuilt drive menu under the window: %T", frames[len(frames)-2])
	}
	if rowOf(rebuilt, "USB stick") < 0 {
		t.Error("the plugged-in drive is not in the rebuilt menu")
	}
	if got, want := rebuilt.SelectPos, rowOf(rebuilt, "Beta"); got != want {
		t.Errorf("cursor on row %d, want %d (Beta, where it was)", got, want)
	}
}

// The watch itself: with a short tick, a drive plugged in shows up in the open
// menu without a key being pressed.
func TestDriveMenuWatchRebuildsTheOpenMenu(t *testing.T) {
	oldEvery, oldDrives := driveMenuWatchEvery, driveMenuPlatformDrives
	t.Cleanup(func() { driveMenuWatchEvery, driveMenuPlatformDrives = oldEvery, oldDrives })
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
	var drives atomic.Value
	drives.Store([]sysinfo.DriveEntry{{Name: "Alpha", Factory: func() vfs.VFS { return nil }}})
	driveMenuPlatformDrives = func() []sysinfo.DriveEntry { return drives.Load().([]sysinfo.DriveEntry) }
	driveMenuWatchEvery = 20 * time.Millisecond

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	t.Cleanup(pf.Close)
	pf.ResizeConsole(100, 30)
	pf.ShowDriveMenu(0)
	first, ok := driveMenuFromFrame(vtui.FrameManager.GetTopFrame())
	if !ok {
		t.Fatal("no drive menu on top")
	}
	t.Cleanup(func() { first.Close() })
	drives.Store([]sysinfo.DriveEntry{
		{Name: "Alpha", Factory: func() vfs.VFS { return nil }},
		{Name: "USB stick", Factory: func() vfs.VFS { return nil }},
	})
	deadline := time.Now().Add(15 * time.Second)
	poll := time.NewTicker(10 * time.Millisecond)
	defer poll.Stop()
	for time.Now().Before(deadline) {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-poll.C:
		}
		if top, ok := driveMenuFromFrame(vtui.FrameManager.GetTopFrame()); ok && top != first {
			for _, it := range top.Items {
				if strings.Contains(it.Text, "USB stick") {
					top.Close()
					return
				}
			}
		}
	}
	t.Fatal("the open drive menu did not show the plugged-in drive within 15 seconds")
}
