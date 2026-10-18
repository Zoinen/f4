package panel

import (
	"strings"
	"testing"

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
