package panel

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/sysinfo"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// f4#1148 (mitek1977-coder): the Tools page lists the built-in tools first and
// everything in the drive menu's own order, and hiding a built-in tool there
// takes it out of the menu.
func TestDriveMenuToolChoicesFollowTheMenu(t *testing.T) {
	cfg := t.TempDir()
	oldUserConfigDir := config.UserConfigDir
	config.UserConfigDir = func() (string, error) { return cfg, nil }
	t.Cleanup(func() { config.UserConfigDir = oldUserConfigDir })
	if err := os.MkdirAll(filepath.Join(cfg, "f4", "settings"), 0o700); err != nil {
		t.Fatal(err)
	}
	oldOptions := config.App.DriveMenuOptions
	t.Cleanup(func() { config.App.DriveMenuOptions = oldOptions })
	config.App.DriveMenuOptions &^= config.DriveMenuSortPluginsByHotkey
	defer sysinfo.SnapshotDrives()()
	sysinfo.SetDrives([]sysinfo.DriveEntry{
		{Name: "Alpha", Factory: func() vfs.VFS { return nil }},
		{Name: "Beta", Factory: func() vfs.VFS { return nil }},
	})

	keys := func() string {
		var out []string
		for _, c := range DriveMenuToolChoices() {
			if c.Key != driveToolKeyRegistry {
				out = append(out, c.Key)
			}
		}
		return fmt.Sprint(out)
	}
	// Physical Disks is the first tool by default (f4#1148).
	hasPhysicalDisks := false
	for _, drive := range sysinfo.GetPlatformDrives() {
		hasPhysicalDisks = hasPhysicalDisks || isPhysicalDisksDrive(drive.Name)
	}
	wantDefault := "[@other-panel @temporary-panel Alpha Beta]"
	wantStored := "[Beta @temporary-panel Alpha @other-panel]"
	if hasPhysicalDisks {
		wantDefault = "[@physical-disks @other-panel @temporary-panel Alpha Beta]"
		wantStored = "[Beta @temporary-panel Alpha @other-panel @physical-disks]"
	}
	if got := keys(); got != wantDefault {
		t.Fatalf("default order %s", got)
	}
	// Shift+Up/Down in the menu stored Beta first and Other panel after Alpha.
	if err := SaveDriveToolsOrder(DriveToolsOrderFilePath(), []string{"Beta", driveToolKeyTemporaryPanel, "Alpha", driveToolKeyOtherPanel}); err != nil {
		t.Fatal(err)
	}
	// A tool the stored order does not name (Physical Disks, new in it) comes last.
	if got := keys(); got != wantStored {
		t.Fatalf("stored order %s", got)
	}

	// Hidden on the Tools page, Other panel is gone from the menu.
	if err := SaveDisabledDriveTools(DriveToolsVisibilityFilePath(), []string{driveToolKeyOtherPanel}); err != nil {
		t.Fatal(err)
	}
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	pf.ShowDriveMenu(0)
	menu, ok := driveMenuFromFrame(vtui.FrameManager.GetTopFrame())
	if !ok {
		t.Fatal("no drive menu on top")
	}
	other := strings.TrimSpace(strings.ReplaceAll(i18n.Msg("Panel.Other"), "&", ""))
	for _, item := range menu.Items {
		if strings.TrimSpace(strings.ReplaceAll(item.Text, "&", "")) == other {
			t.Fatal("the hidden Other panel is still in the drive menu")
		}
	}
}

// f4#1148 (mitek1977-coder): Shift+Up and Shift+Down move a tool of the drive
// menu, as in the bookmark dialog; Ctrl+Up and Ctrl+Down no longer do; the
// menu is replaced without a frame drawn in between.
func TestDriveMenuShiftDownMovesTheTool(t *testing.T) {
	cfg := t.TempDir()
	oldUserConfigDir := config.UserConfigDir
	config.UserConfigDir = func() (string, error) { return cfg, nil }
	t.Cleanup(func() { config.UserConfigDir = oldUserConfigDir })
	if err := os.MkdirAll(filepath.Join(cfg, "f4", "settings"), 0o700); err != nil {
		t.Fatal(err)
	}
	oldOptions := config.App.DriveMenuOptions
	t.Cleanup(func() { config.App.DriveMenuOptions = oldOptions })
	config.App.DriveMenuOptions &^= config.DriveMenuSortPluginsByHotkey
	config.App.DriveMenuOptions |= config.DriveMenuShowPlugins
	defer sysinfo.SnapshotDrives()()
	sysinfo.SetDrives([]sysinfo.DriveEntry{
		{Name: "Alpha", Factory: func() vfs.VFS { return nil }},
		{Name: "Beta", Factory: func() vfs.VFS { return nil }},
	})

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	pf.ShowDriveMenu(0)
	menu, ok := driveMenuFromFrame(vtui.FrameManager.GetTopFrame())
	if !ok {
		t.Fatal("no drive menu on top")
	}
	row := -1
	for i, item := range menu.Items {
		if strings.Contains(item.Text, "Alpha") {
			row = i
		}
	}
	if row < 0 {
		t.Fatal("no row for the Alpha tool")
	}
	menu.SetSelectPos(row)
	order := func() string {
		var out []string
		for _, c := range DriveMenuToolChoices() {
			if !strings.HasPrefix(c.Key, "@") {
				out = append(out, c.Key)
			}
		}
		return fmt.Sprint(out)
	}
	down := func(state vtinput.ControlKeyState) {
		menu.OnKeyDown(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN, ControlKeyState: state})
	}

	down(vtinput.LeftCtrlPressed)
	if got := order(); got != "[Alpha Beta]" {
		t.Errorf("after Ctrl+Down the tools are %s, want them unmoved", got)
	}
	down(vtinput.ShiftPressed)
	if got := order(); got != "[Beta Alpha]" {
		t.Errorf("after Shift+Down the tools are %s, want [Beta Alpha]", got)
	}
	// The old menu stays open until the task that opens the new one closes
	// it, so no frame is drawn without a menu (no flicker); a key that
	// reaches it meanwhile, as a held Shift+Down repeats, moves nothing.
	if menu.IsDone() {
		t.Error("the menu was closed before its replacement opened")
	}
	down(vtinput.ShiftPressed)
	if got := order(); got != "[Beta Alpha]" {
		t.Errorf("a second Shift+Down before the menu came back moved the tools to %s", got)
	}
}
