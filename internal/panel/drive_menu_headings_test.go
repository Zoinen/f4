package panel

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/sysinfo"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// The Tools and Links captions of the drive menu are written into the rules
// that divide the sections, not on rows of their own: the menu is two rows
// shorter and the captions cannot be selected (f4#1148).
func TestDriveMenuCaptionsLiveInTheSeparators(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	defer sysinfo.SnapshotDrives()()
	sysinfo.SetDrives([]sysinfo.DriveEntry{
		{Name: "Alpha", Factory: func() vfs.VFS { return nil }},
		{Name: "Beta", Factory: func() vfs.VFS { return nil }},
	})

	pf.ShowDriveMenu(0)
	menu, ok := driveMenuFromFrame(vtui.FrameManager.GetTopFrame())
	if !ok {
		t.Fatal("no drive menu on top")
	}

	captions := map[string]int{}
	for i, item := range menu.Items {
		if item.Separator && item.Text != "" {
			captions[item.Text] = i
			if menu.IsSelectable != nil && menu.IsSelectable(i) {
				t.Errorf("the %q rule can be selected", item.Text)
			}
			continue
		}
		if text := strings.ReplaceAll(item.Text, "&", ""); text == i18n.Msg("Drive.Tools") || text == i18n.Msg("Drive.Links") {
			t.Errorf("caption %q still takes a row of its own at %d", text, i)
		}
	}
	tools, ok := captions[i18n.Msg("Drive.Tools")]
	if !ok {
		t.Fatalf("no Tools caption in the rules: %v", captions)
	}
	// The built-in tools come first (Other panel, Temporary panel, and the
	// registry on Windows), then the plugin tools (f4#1148).
	builtin := map[string]bool{
		strings.TrimSpace(strings.ReplaceAll(i18n.Msg("Panel.Other"), "&", "")):     true,
		strings.TrimSpace(strings.ReplaceAll(i18n.Msg("TempPanel.Drive"), "&", "")): true,
		"Windows Registry": true,
	}
	first := tools + 1
	for first < len(menu.Items) && builtin[strings.TrimSpace(strings.ReplaceAll(menu.Items[first].Text, "&", ""))] {
		first++
	}
	if first == tools+1 {
		t.Fatalf("the built-in tools are not first under the Tools rule: %q", menu.Items[tools+1].Text)
	}
	if next := strings.TrimSpace(strings.ReplaceAll(menu.Items[first].Text, "&", "")); next != "Alpha" {
		t.Fatalf("the row after the built-in tools is %q, want the first plugin tool", next)
	}
	for _, item := range menu.Items {
		clean := strings.ReplaceAll(item.Text, "&", "")
		if clean == "Alpha" || clean == "Beta" {
			if strings.Contains(item.Text, "&") {
				t.Errorf("tool row %q unexpectedly has an automatic hotkey: %q", clean, item.Text)
			}
		}
	}
	if _, ok := captions[i18n.Msg("Drive.Links")]; !ok {
		t.Fatalf("no Links caption in the rules: %v", captions)
	}
}
