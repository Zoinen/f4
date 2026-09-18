package panel

import (
	"testing"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtui"
)

func TestSemanticSplitRecordsWithoutRefreshingPanels(t *testing.T) {
	original := config.App
	t.Cleanup(func() { config.App = original })
	config.App.AutoSaveSettings = false
	pf, _ := panelsFrameWithMouseSelect(t)
	pf.ShowPanels = true
	pf.WidthDecrement = 0
	left := pf.Panels[0].(*FileSystemPanel)
	before := left.Table.Rows
	revision := left.catalogRevision
	cursor := left.GetCursorIndex()
	start := time.Now()
	for i := 0; i < 1000; i++ {
		ratio := 400000
		if i%2 == 1 {
			ratio = 600000
		}
		if !pf.setSemanticSplit(map[string]any{"target": vtui.SemanticID(pf), "ratioMillionths": ratio}) {
			t.Fatal("split rejected")
		}
	}
	t.Logf("1000 split updates: %s", time.Since(start))
	if pf.WidthDecrement != -8 || config.App.WidthDecrement != -8 || !pf.nativeSplitLayoutPending {
		t.Fatalf("split not recorded: %d", pf.WidthDecrement)
	}
	if left.catalogRevision != revision || left.GetCursorIndex() != cursor ||
		len(left.Table.Rows) != len(before) || len(before) > 0 && &left.Table.Rows[0] != &before[0] {
		t.Fatal("split refreshed or changed panel rows")
	}
	for _, ratio := range []int{0, -1, 1000000, 1000001} {
		if pf.setSemanticSplit(map[string]any{"target": vtui.SemanticID(pf), "ratioMillionths": ratio}) {
			t.Fatalf("invalid ratio accepted: %d", ratio)
		}
	}
	if pf.setSemanticSplit(map[string]any{"target": "stale-workspace", "ratioMillionths": 500000}) {
		t.Fatal("stale workspace accepted")
	}
	pf.LastW = 81
	if !pf.setSemanticSplit(map[string]any{"target": vtui.SemanticID(pf), "ratioMillionths": 500000}) ||
		pf.WidthDecrement != 0 {
		t.Fatal("center reset did not restore zero decrement")
	}
	pf.ResizeConsole(81, 25)
	if pf.nativeSplitLayoutPending {
		t.Fatal("console layout did not consume split")
	}
	_, _, x2, _ := left.GetPosition()
	if x2 != 39 {
		t.Fatalf("console split ends at %d", x2)
	}
}
