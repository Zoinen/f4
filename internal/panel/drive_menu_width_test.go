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

// f4#1148 (mitek1977-coder, 15.10): the right border of the drive menu is one
// character after the longest row of the Drive, Tools & Plugins and Links
// sections, and a long bookmark path is cut to that width instead of
// widening the box.
func TestDriveMenuWidthFollowsTheDriveRowsNotTheBookmarks(t *testing.T) {
	cfg := t.TempDir()
	oldUserConfigDir := config.UserConfigDir
	config.UserConfigDir = func() (string, error) { return cfg, nil }
	t.Cleanup(func() { config.UserConfigDir = oldUserConfigDir })
	if err := os.MkdirAll(filepath.Join(cfg, "f4", "settings"), 0o700); err != nil {
		t.Fatal(err)
	}
	long := "/" + strings.Repeat("deep-folder/", 12) + "the-end"
	if err := os.WriteFile(BookmarksFilePath(),
		[]byte("[3]\nPath="+long+"\nPlugin=\nPluginData=\nPluginFile=\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	restore := sysinfo.SnapshotDrives()
	t.Cleanup(restore)
	widest := "A drive whose name is the widest row of the menu"
	sysinfo.SetDrives([]sysinfo.DriveEntry{
		{Name: "Alpha", Factory: func() vfs.VFS { return nil }},
		{Name: widest, Factory: func() vfs.VFS { return nil }},
	})
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	t.Cleanup(pf.Close)
	pf.ResizeConsole(200, 40)
	pf.ShowDriveMenu(0)
	menu, ok := driveMenuFromFrame(vtui.FrameManager.GetTopFrame())
	if !ok {
		t.Fatal("no drive menu on top")
	}

	// The text of a row starts two columns right of the left border (the
	// border and a blank); its last column is one blank before the right one.
	textEnd := func(text string) int {
		clean, _, _ := vtui.ParseAmpersandString(text)
		return menu.X1 + 2 + vtui.StringWidth(clean) - 1
	}
	longestEnd, bookmarkRow := menu.X1, -1
	for i, item := range menu.Items {
		if strings.HasPrefix(item.Text, "&3 ") {
			bookmarkRow = i
			continue
		}
		if !item.Separator && textEnd(item.Text) > longestEnd {
			longestEnd = textEnd(item.Text)
		}
	}
	if bookmarkRow < 0 {
		t.Fatal("no row for bookmark slot 3")
	}
	if gap := menu.X2 - longestEnd - 1; gap != 1 {
		t.Errorf("box %d..%d, the longest drive or tool row ends at %d: %d blanks before the border, want 1", menu.X1, menu.X2, longestEnd, gap)
	}
	bookmark := menu.Items[bookmarkRow].Text
	if textEnd(bookmark) > menu.X2-2 {
		t.Errorf("bookmark row %q runs to column %d, past the text area of the box (ends at %d)", bookmark, textEnd(bookmark), menu.X2-2)
	}
	if !strings.HasPrefix(bookmark, "&3 …") || !strings.HasSuffix(bookmark, "/the-end") {
		t.Errorf("bookmark row %q: want the path cut on the left with the end kept", bookmark)
	}
}
