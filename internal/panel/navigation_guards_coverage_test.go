package panel

import (
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestPanelsFrameNavigationGuards(t *testing.T) {
	pf := &PanelsFrame{}
	if pf.NavigateToBookmark(nil, Bookmark{}) {
		t.Fatal("nil bookmark panel was accepted")
	}
	if pf.NavigateToPath(&FileSystemPanel{}, "") {
		t.Fatal("empty navigation target was accepted")
	}
	if pf.SyncPassivePanel() {
		t.Fatal("empty frame synchronized panels")
	}

	active := &FileSystemPanel{Vfs: vfs.NewOSVFS(t.TempDir())}
	pf.Panels[0] = active
	if pf.SyncPassivePanel() {
		t.Fatal("frame with no passive panel synchronized")
	}

	pf.Panels[1] = &FileSystemPanel{Vfs: vfs.NewOSVFS(t.TempDir())}
	if pf.NavigateToBookmark(pf.Panels[0].(*FileSystemPanel), Bookmark{Path: ""}) {
		t.Fatal("empty bookmark path was accepted")
	}
}
