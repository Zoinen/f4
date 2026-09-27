package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestCopyActionConflictStaysOnOriginatingPanels(t *testing.T) {
	for _, kind := range []string{"in-place", "copy-dialog", "copy-direct", "move-direct"} {
		t.Run(kind, func(t *testing.T) {
			t.Cleanup(paneltest.SwapFrameManager(t))
			vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
			oldConfig, oldQueue := config.App, fileops.GlobalQueueManager
			t.Cleanup(func() { config.App, fileops.GlobalQueueManager = oldConfig, oldQueue })
			config.App.DefaultFileOpMode = 0
			config.App.ConfirmCopy = kind == "copy-dialog"
			config.App.ConfirmMove = false
			config.App.AutoSaveDialogSettings = false
			queue := fileops.NewQueueManagerWithTasks()
			fileops.GlobalQueueManager = queue
			root, dest := t.TempDir(), t.TempDir()
			for _, path := range []string{filepath.Join(root, "source.txt"), filepath.Join(root, "existing.txt"), filepath.Join(dest, "source.txt")} {
				if err := os.WriteFile(path, []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			pf := panel.NewPanelsFrame()
			defer pf.Close()
			pf.ResizeConsole(100, 30)
			pf.ActiveIdx = 0
			source := pf.Panels[0].(*panel.FileSystemPanel)
			source.Vfs = vfs.NewOSVFS(root)
			pf.Panels[1].(*panel.FileSystemPanel).Vfs = vfs.NewOSVFS(dest)
			source.Entries = []*panel.FileEntry{{VFSItem: vfs.VFSItem{Name: "source.txt"}}}
			source.SetCursorIndex(0)
			vtui.FrameManager.Push(pf)
			origin := vtui.FrameManager.ActiveIdx
			if kind == "in-place" {
				actionCopyInPlace(pf)
				dlg := vtui.FrameManager.GetTopFrame()
				for _, child := range dlg.(vtui.Container).GetChildren() {
					if edit, ok := child.(*vtui.Edit); ok {
						edit.SetText("existing.txt")
						break
					}
				}
				dlg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})
				vtui.FrameManager.Pop()
			} else {
				actionCopyMove(pf, kind == "move-direct")
				if kind == "copy-dialog" {
					dlg := vtui.FrameManager.GetTopFrame()
					dlg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})
					vtui.FrameManager.Pop()
				}
			}
			deadline := time.After(5 * time.Second)
			for len(queue.Tasks()) == 0 {
				select {
				case task := <-vtui.FrameManager.TaskChan:
					task()
				case <-time.After(time.Millisecond):
				case <-deadline:
					t.Fatal("copy action did not enqueue")
				}
			}
			queue.EnsureQueueWorkspace()
			var queueFrame vtui.Frame
			for _, screen := range vtui.FrameManager.Screens {
				for _, frame := range screen.Frames {
					if _, ok := frame.(*fileops.QueueFrame); ok {
						queueFrame = frame
					}
				}
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			queued := queue.Tasks()[0]
			done := make(chan error, 1)
			go func() { done <- queued.Run(ctx, &fileops.DummyReporter{}, queueFrame) }()
			for {
				select {
				case task := <-vtui.FrameManager.TaskChan:
					task()
					if dlg, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window); ok && dlg.IsWarning {
						if vtui.FrameManager.ActiveIdx != origin {
							t.Errorf("[FIX:copy-anchor] conflict moved to queue screen %d; origin=%d", vtui.FrameManager.ActiveIdx, origin)
						}
						cancel()
					}
				case err := <-done:
					queued.SetState("Cancelled")
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("conflict result=%v, want cancellation", err)
					}
					return
				case <-deadline:
					cancel()
					<-done
					queued.SetState("Cancelled")
					t.Fatal("conflict dialog never appeared")
				}
			}
		})
	}
}
