package terminal

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
)

func TestTerminalSemanticLiveRewriteRetainsColorsThroughoutScrollback(t *testing.T) {
	vtui.SetDefaultPalette()
	tv := NewTerminalView(80, 8)
	defer tv.Close()
	parser := NewAnsiParser(tv, nil)
	parser.Process([]byte("\x1b[31mMoving 100 files\x1b[0m\r\n\x1b[34mWorker old\x1b[0m"))
	before := tv.SemanticModel(nil)
	parser.Process([]byte("\r\x1b[1A\x1b[2K\x1b[32mDone\x1b[0m\r\n\x1b[2K\x1b[34mWorker new\x1b[0m"))
	after := tv.SemanticModel(nil)
	if after.ContentExtent != before.ContentExtent {
		t.Fatalf("live rewrite grew history: %d -> %d", before.ContentExtent, after.ContentExtent)
	}
	if after.ToMap()["windowContentKey"] == before.ToMap()["windowContentKey"] {
		t.Fatal("live rewrite did not change the published window content key")
	}
	text := terminalModelText(semantic.AppMapSlice(after.ToMap()["windowRows"]))
	if strings.Contains(text, "Moving") || strings.Contains(text, "files") || strings.Contains(text, "old") {
		t.Fatalf("erase-line retained old progress contents: %q", text)
	}
	assertRun := func(model *extui.TerminalModel, label, color string) string {
		t.Helper()
		for _, row := range model.WindowRows {
			for _, run := range row.Runs {
				if run.Text == label {
					if run.Foreground != color {
						t.Fatalf("%q foreground=%q, want %q", label, run.Foreground, color)
					}
					return row.ContentKey
				}
			}
		}
		t.Fatalf("%q absent from semantic window", label)
		return ""
	}
	green := semantic.SemanticAttrColor(vtui.SetIndexFore(DefaultTermAttr, 2), true)
	blue := semantic.SemanticAttrColor(vtui.SetIndexFore(DefaultTermAttr, 4), true)
	assertRun(after, "Done", green)
	oldWorkerKey := assertRun(after, "Worker new", blue)

	// Repainting identical text with another SGR color must invalidate that row.
	parser.Process([]byte("\r\x1b[32mWorker new\x1b[0m"))
	after = tv.SemanticModel(nil)
	if key := assertRun(after, "Worker new", green); key == oldWorkerKey {
		t.Fatal("style-only live rewrite retained the old row content key")
	}

	showStart := func() *extui.TerminalModel {
		t.Helper()
		tv.HandleSemanticAction(map[string]any{
			"target": vtui.SemanticID(tv), "action": "terminal.scroll",
			"visualRow": 0, "followTail": false,
		})
		model := tv.SemanticModel(nil)
		if len(model.WindowRows) > 3*model.ViewportRows {
			t.Fatalf("semantic publication is not bounded: %d rows", len(model.WindowRows))
		}
		assertRun(model, "Done", green)
		assertRun(model, "Worker new", green)
		return model
	}
	parser.Process([]byte(strings.Repeat("\r\nhistory", 12)))
	if len(tv.GridHistory) == 0 {
		t.Fatal("fixture did not move the live rows into staging history")
	}
	showStart()
	parser.Process([]byte(strings.Repeat("\r\nhistory", 2100)))
	if tv.Pt.Size() == 0 {
		t.Fatal("fixture did not extrude rows into permanent scrollback")
	}
	model := showStart()
	if model.FollowTail || model.ViewportStart != 0 {
		t.Fatal("live output moved the user's old-history viewport")
	}
}

func TestTerminalSemanticShortOutputHasNoScrollablePadding(t *testing.T) {
	vtui.SetDefaultPalette()
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	parser := NewAnsiParser(tv, nil)
	parser.Process([]byte("short output"))
	tv.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(tv), "action": "terminal.viewport", "rows": 20,
	})
	before := tv.SemanticModel(nil)
	if got := semantic.AppInt64(before.ToMap()["contentStart"]); got != 23 {
		t.Fatalf("content start=%d, want first actual output at absolute row 23", got)
	}
	if before.ContentExtent != 24 || before.CursorAbsoluteRow != 23 {
		t.Fatal("content metadata must not renumber terminal rows or change bottom gravity")
	}
	tv.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(tv), "action": "terminal.scroll", "visualRow": 0,
	})
	after := tv.SemanticModel(nil)
	if after.ViewportStart != before.ViewportStart || !after.FollowTail {
		t.Fatalf("short output scrolled into padding: before=%d after=%d follow=%v",
			before.ViewportStart, after.ViewportStart, after.FollowTail)
	}
	if got := tv.semanticBoundarySelectionText(23, 0, 23, 12); got != "short output" {
		t.Fatalf("absolute selection changed: %q", got)
	}
	parser.Process([]byte(strings.Repeat("\r\nreal history", 30)))
	withHistory := tv.SemanticModel(nil)
	if got := semantic.AppInt64(withHistory.ToMap()["contentStart"]); got != 0 {
		t.Fatalf("stored history was excluded from scrolling: contentStart=%d", got)
	}
}
