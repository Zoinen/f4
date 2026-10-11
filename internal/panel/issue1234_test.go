package panel

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// TestIssue1234UpRowKeepsParentTime is the regression test for issue #1234:
// after entering a folder the ".." row showed 01.01.01 00:00 in the panel's
// status line. The row is created with the first chunk of the listing, while
// the parent's timestamps are fetched only after ReadDir has finished, so the
// completed load has to hand them to the row that is already on screen.
func TestIssue1234UpRowKeepsParentTime(t *testing.T) {
	oldCfg := config.App
	defer func() { config.App = oldCfg }()
	config.App.SyncPanelLoad = false

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	parent := t.TempDir()
	child := filepath.Join(parent, "child")
	if err := os.Mkdir(child, 0o750); err != nil {
		t.Fatal(err)
	}
	// #nosec G703 -- the path is inside the private test temp directory.
	if err := os.WriteFile(filepath.Join(child, "a.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	want := time.Date(2020, 2, 3, 4, 5, 6, 0, time.Local)
	if err := os.Chtimes(parent, want, want); err != nil {
		t.Fatal(err)
	}

	fp := NewFileSystemPanel(0, 0, 80, 25, vfs.NewOSVFS(child))
	t.Cleanup(func() {
		fp.cancelProviderOpen()
		if fp.Vfs != nil {
			_ = fp.Vfs.Close()
		}
		if fp.CancelLoad != nil {
			fp.CancelLoad()
		}
		fp.StopLoadingAnimation()
	})
	waitForLoad(t, fp)

	if len(fp.Entries) < 2 || fp.Entries[0].Name != ".." {
		t.Fatalf("panel rows = %v, want \"..\" followed by a.txt", panelNames(fp))
	}
	if got := fp.Entries[0].MTime; !got.Equal(want) {
		t.Errorf("\"..\" row time = %v, want the parent folder's time %v", got, want)
	}
}
