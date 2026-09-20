package panel

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// TestEntrySizeText pins the far2l/Far3 type labels shown where a size goes
// (#392): Up, Folder, Symlink and Junction, with a size only for files and
// counted folders.
func TestEntrySizeText(t *testing.T) {
	cases := []struct {
		name  string
		entry FileEntry
		want  string
	}{
		{"file", FileEntry{VFSItem: vfs.VFSItem{Name: "a.bin", Size: 1234}}, "1 234"},
		{"up", FileEntry{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}, "Up"},
		{"folder", FileEntry{VFSItem: vfs.VFSItem{Name: "sub", IsDir: true}}, "Folder"},
		{"counted folder", FileEntry{VFSItem: vfs.VFSItem{Name: "sub", IsDir: true, Size: 2048}, SizeCalculated: true}, "2 048"},
		{"symlink to dir", FileEntry{VFSItem: vfs.VFSItem{Name: "l", IsDir: true, IsSymlink: true}}, "Symlink"},
		// far2l's default ShowSymlinkSize=0: a link to a file does not show
		// the size of the link itself.
		{"symlink to file", FileEntry{VFSItem: vfs.VFSItem{Name: "l", Size: 9, IsSymlink: true}}, "Symlink"},
		{"windows symlink", FileEntry{VFSItem: vfs.VFSItem{Name: "l", IsDir: true, IsSymlink: true, ReparseTag: vfs.ReparseTagSymlink}}, "Symlink"},
		{"junction", FileEntry{VFSItem: vfs.VFSItem{Name: "All Users", IsDir: true, IsSymlink: true, ReparseTag: vfs.ReparseTagMountPoint}}, "Junction"},
		{"non-link reparse file", FileEntry{VFSItem: vfs.VFSItem{Name: "d.bin", Size: 5, IsSymlink: true, ReparseTag: 0x80000013}}, "5"},
		{"non-link reparse dir", FileEntry{VFSItem: vfs.VFSItem{Name: "cloud", IsDir: true, IsSymlink: true, ReparseTag: 0x80000013}}, "Folder"},
	}
	for _, tc := range cases {
		if got := entrySizeText(&tc.entry); got != tc.want {
			t.Errorf("%s: entrySizeText = %q, want %q", tc.name, got, tc.want)
		}
		if got := tc.entry.GetCellText(1); got != tc.want {
			t.Errorf("%s: Size column = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestFileSystemPanel_StatusLineNamesJunction checks that the status line
// uses the same labels as the Size column instead of its old <DIR>/<LNK>
// placeholders.
func TestFileSystemPanel_StatusLineNamesJunction(t *testing.T) {
	oldCfg := config.App
	defer func() { config.App = oldCfg }()
	config.App.ShowPanelFileInfo = true

	vtui.SetDefaultPalette()
	theme.SetDefaultF4Palette()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 25)
	vtui.FrameManager.Init(scr)

	fp := NewFileSystemPanel(0, 0, 90, 20, vfs.NewOSVFS(t.TempDir()))
	waitForLoad(t, fp)
	// The junction exists only as an entry, so reading its target fails and
	// the status line keeps the label.
	fp.Entries = []*FileEntry{
		{VFSItem: vfs.VFSItem{Name: "sub", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "All Users", IsDir: true, IsSymlink: true, ReparseTag: vfs.ReparseTagMountPoint}},
	}
	fp.IsLoading = false
	if fp.LoadingTimer != nil {
		fp.LoadingTimer.Stop()
	}
	fp.Refresh()

	status := func() string { return testutil.ScreenRow(scr, fp.Y2-1, fp.X1, fp.X2) }

	fp.SetCursorIndex(0)
	fp.Show(scr)
	if got := status(); !strings.Contains(got, "Folder") || strings.Contains(got, "<DIR>") {
		t.Errorf("status line for a folder = %q, want Folder", got)
	}
	fp.SetCursorIndex(1)
	fp.Show(scr)
	if got := status(); !strings.Contains(got, "Junction") || strings.Contains(got, "<LNK") {
		t.Errorf("status line for a junction = %q, want Junction", got)
	}
}
