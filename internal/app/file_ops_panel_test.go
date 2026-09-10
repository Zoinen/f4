package app

import (
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Two file-operation tests whose subject is the panels frame rather than the
// operation: one asks that a refresh survives an undocked panel, the other
// that a backgrounded operation forks the workspace. Both go where
// panel.PanelsFrame is.

func TestFileOps_RefreshAllNoPanic(t *testing.T) {
	pf := panel.NewPanelsFrame()
	defer pf.Close()
	// Ensure refresh doesn't crash even if panels are not fully docked
	pf.RefreshAll()
}

func TestFileOps_ForkedWorkspace(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := panel.NewPanelsFrame()
	defer pf.Close()
	vtui.FrameManager.Push(pf)
	initialScreens := len(vtui.FrameManager.Screens)

	tmpSrc := t.TempDir()
	tmpDst := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpSrc, "f1.txt"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	// forked = true
	fileops.ExecuteFileOp(vfs.NewOSVFS(tmpSrc), vfs.NewOSVFS(tmpDst), []string{"f1.txt"}, tmpDst, false, 1, func() { close(done) })

	// Process tasks until the background copy finishes
	timeout := time.After(2 * time.Second)
pump4:
	for {
		select {
		case <-done:
			break pump4
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatalf("Timeout")
		}
	}
	for _, screen := range vtui.FrameManager.Screens {
		for _, frame := range screen.Frames {
			clone, ok := frame.(*panel.PanelsFrame)
			if !ok {
				continue
			}
			for _, pnl := range clone.Panels {
				if fsp, ok := pnl.(*panel.FileSystemPanel); ok {
					paneltest.WaitForLoad(t, fsp)
				}
			}
		}
	}

	// We expect that a new screen was created during the operation
	if len(vtui.FrameManager.Screens) != initialScreens+1 {
		t.Errorf("Forked operation did not create a new workspace screen. Screens: %d", len(vtui.FrameManager.Screens))
	}
}
