package panel

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/vfs"
)

// "Files size" in the quick view counts the files only, as F3 does: the size
// a file system gives the folders themselves (32 bytes an entry on APFS)
// made the two disagree on the same folder by exactly that much (f4#2005).
func TestQuickViewFilesSizeLeavesFolderSizesOut(t *testing.T) {
	filesSize := func(dirBytes int64) string {
		q := &QuickViewPanel{scanDone: true, scanStats: vfs.OpStats{Files: 3, Bytes: 100, DirBytes: dirBytes, PhysicalBytes: 8192}}
		var line string
		q.renderDir(&FileEntry{VFSItem: vfs.VFSItem{Name: "20260103", IsDir: true}}, func(l string, _ uint64) {
			if strings.Contains(l, "Files size") {
				line = l
			}
		}, 0)
		if line == "" {
			t.Fatal("no Files size row")
		}
		return line
	}
	if with, without := filesSize(20), filesSize(0); with != without {
		t.Fatalf("folder sizes changed Files size: %q vs %q", with, without)
	}
}

// Panel.DirSizeCountsFolders switches the quick view to far2l's sum, files
// plus the size the file system gives the folders (f4#2005).
func TestQuickViewFilesSizeCountsFoldersWhenAsked(t *testing.T) {
	before := config.App
	defer func() { config.App = before }()
	render := func(on bool) string {
		config.App.DirSizeCountsFolders = on
		q := &QuickViewPanel{scanDone: true, scanStats: vfs.OpStats{Files: 3, Bytes: 100, DirBytes: 20, PhysicalBytes: 8192}}
		var line string
		q.renderDir(&FileEntry{VFSItem: vfs.VFSItem{Name: "20260103", IsDir: true}}, func(l string, _ uint64) {
			if strings.Contains(l, "Files size") {
				line = l
			}
		}, 0)
		return line
	}
	off, on := render(false), render(true)
	if off == "" || on == "" || off == on {
		t.Fatalf("the setting must change Files size: off %q, on %q", off, on)
	}
	if !strings.Contains(off, "100") || !strings.Contains(on, "120") {
		t.Fatalf("want 100 B off and 120 B on, got off %q, on %q", off, on)
	}
}
