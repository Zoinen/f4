package terminal

import (
	"testing"

	"github.com/unxed/vtui"
)

func TestTerminalHistorySearchFindsRowsOutsideLoadedWindow(t *testing.T) {
	vtui.SetDefaultPalette()
	tv := seededTerminalSemanticHistory(t, 80, 24, 900)
	defer tv.Close()

	initial := tv.SemanticModel(nil)
	if initial.WindowStart <= 2 {
		t.Fatalf("fixture already has target row loaded: %d", initial.WindowStart)
	}
	if !tv.findHistoryMatch("ROW-000002", true, false) {
		t.Fatal("case-insensitive search missed the saved history")
	}
	model := tv.SemanticModel(nil)
	match := model.SearchMatch
	if match == nil || match.StartRow != 2 || match.EndRow != 2 ||
		match.StartColumn != 0 || match.EndColumn != 10 {
		t.Fatalf("unexpected match: %+v", match)
	}
	if model.FollowTail || model.ViewportStart > 2 || model.ViewportStart+model.ViewportSpan <= 2 {
		t.Fatalf("match was not brought into view: start=%d span=%d", model.ViewportStart, model.ViewportSpan)
	}
	if tv.findHistoryMatch("no matching line", true, false) || tv.SemanticModel(nil).SearchMatch != nil {
		t.Fatal("missing query retained the previous highlight")
	}
}

func TestTerminalHistorySearchMatchesAcrossWrapAndRepeats(t *testing.T) {
	vtui.SetDefaultPalette()
	tv := seededTerminalSemanticHistory(t, 10, 3, 3)
	defer tv.Close()
	tv.SemanticModel(nil)

	const query = "2 payload"
	if !tv.findHistoryMatch(query, true, false) {
		t.Fatal("search missed a match across wrapped rows")
	}
	match := tv.SemanticModel(nil).SearchMatch
	if match == nil || match.StartRow != 4 || match.StartColumn != 9 ||
		match.EndRow != 5 || match.EndColumn != 7 {
		t.Fatalf("unexpected wrapped match: %+v", match)
	}
	if !tv.findHistoryMatch("payload", true, false) {
		t.Fatal("search missed the newest payload")
	}
	newest := tv.SemanticModel(nil).SearchMatch
	if !tv.findHistoryMatch("payload", true, true) {
		t.Fatal("reverse search missed an earlier payload")
	}
	earlier := tv.SemanticModel(nil).SearchMatch
	if earlier.StartRow >= newest.StartRow || earlier.Sequence <= newest.Sequence {
		t.Fatalf("reverse search failed to advance: earlier=%+v newest=%+v", earlier, newest)
	}
	if !tv.findHistoryMatch("payload", false, true) ||
		tv.SemanticModel(nil).SearchMatch.StartRow != newest.StartRow {
		t.Fatal("forward search did not return to the newest payload")
	}
}

func TestTerminalHistorySearchSkipsIdlePromptAndAlternateScreen(t *testing.T) {
	vtui.SetDefaultPalette()
	tv := NewTerminalView(40, 8)
	defer tv.Close()
	for column, r := range "idle prompt" {
		tv.Lines[0][column].Char = uint64(r)
	}
	tv.CursorY = 0
	tv.SemanticModelWithBottomOverlay(nil, 1)
	if tv.findHistoryMatch("idle", true, false) {
		t.Fatal("search found the hidden prompt")
	}
	tv.UseAltScreen = true
	if tv.findHistoryMatch("idle", true, false) || tv.SemanticModel(nil).SearchMatch != nil {
		t.Fatal("alternate screen exposed a history match")
	}
}
