package editor

import (
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtui"
	"strings"
	"testing"
)

func TestSearchMatchAtVisibleWrappedEdgeKeepsViewport(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	ev := NewEditorView(piecetable.New([]byte(strings.Repeat("x", 400))), nil, "visible-match.txt")
	defer ev.Close()
	ev.SetPosition(0, 0, 20, 5)
	ev.WordWrap = true
	ev.EnsureEngineWidth()
	ev.ScrollTopRow = 2
	end := (ev.ScrollTopRow + ev.ViewportHeight()) * ev.viewportWidth()
	ev.selectFoundPattern(end-3, 3)
	if ev.ScrollTopRow != 2 {
		t.Fatalf("fully visible match moved top to %d", ev.ScrollTopRow)
	}
	ev.selectFoundPattern(end+ev.viewportWidth()*2, 3)
	if ev.ScrollTopRow == 2 {
		t.Fatal("offscreen match was not revealed")
	}
}
