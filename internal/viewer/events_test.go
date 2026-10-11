package viewer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestViewerReportsOpenAndClose(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	path := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(path, []byte("text\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var events, ids []int
	OnEvent = func(vv *ViewerView, event int) {
		events = append(events, event)
		ids = append(ids, vv.MacroID)
	}
	t.Cleanup(func() { OnEvent = nil })

	vv, err := NewViewerView(context.Background(), vfs.NewOSVFS(filepath.Dir(path)), path)
	if err != nil {
		t.Fatal(err)
	}
	vv.Close()
	vv.Close() // a second Close reports nothing more

	if len(events) != 2 || events[0] != EventRead || events[1] != EventClose {
		t.Fatalf("events = %v, want [read close]", events)
	}
	if ids[0] == 0 || ids[0] != ids[1] {
		t.Errorf("viewer ids = %v, want one non-zero id", ids)
	}
	OnEvent = nil
	vv.notify(EventRead) // no hook: nothing happens
}
