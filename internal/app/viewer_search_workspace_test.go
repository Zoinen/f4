package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/viewer"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestViewerSearchDialogsStayInWorkspace(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(80, 25)
	vtui.FrameManager.Init(screen)
	path := filepath.Join(t.TempDir(), "search.txt")
	if err := os.WriteFile(path, []byte("some content\n"), 0600); err != nil {
		t.Fatal(err)
	}
	document, err := viewer.NewViewerView(context.Background(), vfs.NewOSVFS(filepath.Dir(path)), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { document.Close() })
	vtui.FrameManager.Push(document)
	workspace := vtui.FrameManager.ActiveIdx
	count := len(vtui.FrameManager.Screens)
	runViewerSearch(document, "absent", false)
	deadline := time.After(3 * time.Second)
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			vtui.FrameManager.Step(0)
		case <-deadline:
			t.Fatal("search result did not arrive")
		}
		if vtui.FrameManager.ActiveIdx != workspace || len(vtui.FrameManager.Screens) != count {
			t.Fatal("search progress or result created another workspace")
		}
		frames := vtui.FrameManager.GetActiveFrames(workspace)
		if len(frames) > 1 && frames[len(frames)-2] != document {
			t.Fatal("search dialog lost its viewer underlay")
		}
		if top := vtui.FrameManager.GetTopFrame(); top.GetTitle() == " Search " {
			top.(*vtui.Window).Close()
			vtui.FrameManager.Step(0)
			if vtui.FrameManager.GetTopFrame() != document {
				t.Fatal("closing search result did not restore viewer")
			}
			return
		}
	}
}

func TestViewerSearchKeepsVisibleMatchInPlace(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(80, 25)
	vtui.FrameManager.Init(screen)
	path := filepath.Join(t.TempDir(), "visible.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("line\n", 50)), 0600); err != nil {
		t.Fatal(err)
	}
	document, err := viewer.NewViewerView(context.Background(), vfs.NewOSVFS(filepath.Dir(path)), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { document.Close() })
	vtui.FrameManager.Push(document)
	document.SetPosition(0, 0, 79, 23)
	document.TopOffset = 10
	document.LastSearch = "line"
	document.LastSearchFound = true
	document.LastSearchOffset = 10
	document.LastSearchTopOffset = 10
	runViewerSearch(document, "line", false)
	deadline := time.After(3 * time.Second)
	for document.LastSearchOffset == 10 {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			vtui.FrameManager.Step(0)
			if vtui.FrameManager.GetTopFrame() != document {
				t.Fatal("quick find-next opened a progress popup")
			}
		case <-deadline:
			t.Fatal("next match did not arrive")
		}
	}
	if document.LastSearchOffset != 15 || document.TopOffset != 10 {
		t.Fatalf("visible search moved viewport: match=%d top=%d", document.LastSearchOffset, document.TopOffset)
	}
}
