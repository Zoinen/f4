package panel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/dialog"
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
	oldPlatformDrives, oldKind, oldFS := driveMenuPlatformDrives, driveMenuRowKind, driveMenuRowFS
	t.Cleanup(func() {
		driveMenuPlatformDrives, driveMenuRowKind, driveMenuRowFS = oldPlatformDrives, oldKind, oldFS
	})
	driveMenuPlatformDrives = func() []sysinfo.DriveEntry {
		return []sysinfo.DriveEntry{
			{Name: "C:"},
			{Name: "Windows Registry"},
		}
	}
	driveMenuRowKind = func(_, _ string) driveMenuKind { return driveMenuKindFixed }
	driveMenuRowFS = func(string) (sysinfo.FSInfo, bool) {
		return sysinfo.FSInfo{Label: "System", Type: "NTFS", Total: 100, Free: 50}, true
	}

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
		"Windows Registry":      true,
		"Physical Disks (/dev)": true, // a tool since f4#1148
		"Physical Disks":        true,
	}
	// A tool row is just its name; platform-drive columns must not follow the
	// Windows Registry entry after it is moved to the Tools section.
	rowName := func(text string) string {
		name, _, _ := strings.Cut(strings.ReplaceAll(text, "&", ""), "│")
		return strings.TrimSpace(name)
	}
	first := tools + 1
	for first < len(menu.Items) && builtin[rowName(menu.Items[first].Text)] {
		first++
	}
	if first == tools+1 {
		t.Fatalf("the built-in tools are not first under the Tools rule: %q", menu.Items[tools+1].Text)
	}
	for _, item := range menu.Items {
		if rowName(item.Text) == "Windows Registry" && strings.Contains(item.Text, "│") {
			t.Fatalf("Windows Registry tool row includes drive-column separators: %q", item.Text)
		}
	}
	if next := rowName(menu.Items[first].Text); next != "Alpha" {
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

// f4#1148 (mitek1977-coder): the bookmark slots get their caption in the rule
// above them, like Tools and Links; the slot digit is one space from the path,
// as on the Tools and Links rows; and the bottom line lists Shift+Up/Down
// rather than the second key for Ins.
func TestDriveMenuBookmarksCaptionAndSpacing(t *testing.T) {
	cfg := t.TempDir()
	oldUserConfigDir := config.UserConfigDir
	config.UserConfigDir = func() (string, error) { return cfg, nil }
	t.Cleanup(func() { config.UserConfigDir = oldUserConfigDir })
	if err := os.MkdirAll(filepath.Join(cfg, "f4", "settings"), 0o700); err != nil {
		t.Fatal(err)
	}
	place := t.TempDir()
	if err := os.WriteFile(BookmarksFilePath(),
		[]byte("[3]\nPath="+place+"\nPlugin=\nPluginData=\nPluginFile=\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	pf.ShowDriveMenu(0)
	top := vtui.FrameManager.GetTopFrame()
	menu, ok := driveMenuFromFrame(top)
	if !ok {
		t.Fatal("no drive menu on top")
	}

	caption := strings.TrimSpace(i18n.Msg("Bookmarks.Title"))
	row := -1
	for i, item := range menu.Items {
		if strings.HasPrefix(item.Text, "&3") {
			row = i
		}
	}
	if row < 1 {
		t.Fatalf("no row for bookmark slot 3")
	}
	if rule := menu.Items[row-1]; !rule.Separator || rule.Text != caption {
		t.Errorf("the rule above the bookmarks is %+v, want a separator captioned %q", rule, caption)
	}
	// The path is cut to the box (f4#1148), so compare with it cut the same way.
	boxWidth := menu.X2 - menu.X1 + 1
	if want := "&3 " + dialog.EscapeAmpersand(dialog.TruncPathLeft(place, boxWidth-driveMenuBoxChrome-2)); menu.Items[row].Text != want {
		t.Errorf("bookmark row %q, want %q", menu.Items[row].Text, want)
	}

	frame, ok := top.(*DriveMenuFrame)
	if !ok {
		t.Fatalf("top frame is %T", top)
	}
	if strings.Contains(frame.bottomHint, "Ctrl+N") || !strings.HasSuffix(strings.TrimSpace(frame.bottomHint), "Shift+Up/Down") {
		t.Errorf("bottom line %q: want no Ctrl+N and Shift+Up/Down at the end", frame.bottomHint)
	}
}
