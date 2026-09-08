package main

import (
	"strings"
	"testing"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestDriveMenuPlatformRowsAlignColumns(t *testing.T) {
	options := driveMenuShowType | driveMenuShowLabel | driveMenuShowFilesystem | driveMenuShowSize
	rows := []driveMenuPlatformRow{
		{base: "C:", kind: "fixed", label: "Win10", filesystem: "NTFS", total: "953.0 GiB", free: "300.6 GiB"},
		{base: "K:", kind: "network", label: "DISK-K", filesystem: "NTFS", total: "13.8 TiB", free: "1.2 TiB"},
	}
	lines := driveMenuPlatformRowsText(rows, options)
	if len(lines) != 2 {
		t.Fatalf("got %d rows, want 2: %q", len(lines), lines)
	}
	for _, column := range []string{"fixed", "Win10", "NTFS"} {
		if got, want := strings.Index(lines[0], column), strings.Index(lines[1], map[string]string{
			"fixed": "network", "Win10": "DISK-K", "NTFS": "NTFS",
		}[column]); got != want {
			t.Fatalf("column %q starts at %d and %d, want equal starts", column, got, want)
		}
	}
	if !strings.Contains(lines[0], "953.0 GiB | 300.6 GiB") || !strings.Contains(lines[1], " 13.8 TiB |   1.2 TiB") {
		t.Fatalf("size columns are not aligned/right-justified: %q", lines)
	}
}

func TestDriveMenuPhysicalDiskHasNoTypeDescription(t *testing.T) {
	got := driveMenuPlatformItemText(DriveEntry{Name: "Physical Disks"}, driveMenuShowType)
	if got != "Physical Disks" {
		t.Fatalf("physical disk row = %q, want no type suffix", got)
	}
}

func TestDriveMenuOptionsDialogSizeIsContentBased(t *testing.T) {
	width, height := driveMenuOptionsDialogSize()
	if width >= 78 {
		t.Fatalf("options dialog width = %d, want narrower than the old fixed width", width)
	}
	if height != len(driveMenuOptionSpecs)+7 {
		t.Fatalf("options dialog height = %d, want %d", height, len(driveMenuOptionSpecs)+7)
	}
	longest := 0
	for _, spec := range driveMenuOptionSpecs {
		label, _, _ := vtui.ParseAmpersandString(Msg(spec.label))
		if w := vtui.StringWidth(label); w > longest {
			longest = w
		}
	}
	if width < longest+8 {
		t.Fatalf("options dialog width = %d, does not fit longest checkbox label of %d", width, longest)
	}
}

func TestDriveMenuOptions_DefaultsAndFormatting(t *testing.T) {
	if parseDriveMenuOptions("") != defaultDriveMenuOptions {
		t.Fatalf("empty options did not use defaults: %#x", parseDriveMenuOptions(""))
	}
	if parseDriveMenuOptions("not-a-number") != defaultDriveMenuOptions {
		t.Fatalf("invalid options did not use defaults")
	}
	if got := driveMenuPlatformItemText(DriveEntry{Name: "/ Root"}, driveMenuShowType|driveMenuShowFilesystem); !strings.Contains(got, "/") {
		t.Fatalf("root row lost its name: %q", got)
	}
	if got := driveMenuSize(1024*1024*3, false); got != "3 MiB" {
		t.Fatalf("integer drive size = %q, want 3 MiB", got)
	}
	if got := driveMenuSize(1024*1024*3, true); !strings.Contains(got, "3.0") {
		t.Fatalf("decimal drive size = %q, want a decimal value", got)
	}
}

func TestPanelsFrame_DriveMenu_F9OpensOptions(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	oldOptions := AppConfig.DriveMenuOptions
	AppConfig.DriveMenuOptions = defaultDriveMenuOptions
	t.Cleanup(func() { AppConfig.DriveMenuOptions = oldOptions })

	pf.showDriveMenu(0)
	menu, ok := driveMenuFromFrame(vtui.FrameManager.GetTopFrame())
	if !ok {
		t.Fatalf("drive menu not opened: %T", vtui.FrameManager.GetTopFrame())
	}
	if !strings.Contains(Msg("Drive.BottomHint"), "F9") {
		t.Fatalf("drive menu hint does not advertise F9: %q", Msg("Drive.BottomHint"))
	}
	if !menu.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F9,
	}) {
		t.Fatal("F9 was not consumed by the drive menu")
	}
	if vtui.FrameManager.GetTopFrame() == menu {
		t.Fatal("F9 did not open the drive options dialog")
	}
	dlg, ok := vtui.FrameManager.GetTopFrame().(vtui.Container)
	if !ok {
		t.Fatalf("drive options frame is not a container: %T", vtui.FrameManager.GetTopFrame())
	}
	vtui.AssertLayout(t, dlg)
	vtui.FrameManager.Pop()
	menu.Close()
}
