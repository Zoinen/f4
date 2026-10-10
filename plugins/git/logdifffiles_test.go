package git

import (
	"testing"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestLogDiffFileRowGetCellText(t *testing.T) {
	row := logDiffFileRow{entry: logDiffEntry{Status: 'M', Path: "foo.go"}}
	if got := row.GetCellText(colLogDiffFileStatus); got != "M" {
		t.Errorf("GetCellText(colLogDiffFileStatus) = %q, want %q", got, "M")
	}
	if got := row.GetCellText(colLogDiffFilePath); got != "foo.go" {
		t.Errorf("GetCellText(colLogDiffFilePath) = %q, want %q", got, "foo.go")
	}
	if got := row.GetCellText(99); got != "" {
		t.Errorf("GetCellText(99) = %q, want empty", got)
	}
}

// TestLogDiffFileRowGetCellTextRename mirrors statusRow's own rename
// rendering (panel.go's TestStatusRowGetCellText, if there is one) -- "old
// -> new" in the path column, the same shape a working-tree rename already
// gets on the status panel.
func TestLogDiffFileRowGetCellTextRename(t *testing.T) {
	row := logDiffFileRow{entry: logDiffEntry{Status: 'R', OrigPath: "old.go", Path: "new.go"}}
	if got := row.GetCellText(colLogDiffFilePath); got != "old.go -> new.go" {
		t.Errorf("GetCellText(colLogDiffFilePath) = %q, want %q", got, "old.go -> new.go")
	}
}

func TestNewLogDiffFilesViewPopulatesRows(t *testing.T) {
	files := []logDiffEntry{
		{Status: 'M', Path: "foo.go"},
		{Status: 'A', Path: "bar.go"},
	}
	v := newLogDiffFilesView("/repo", "abc123", "abc123", files)
	if len(v.table.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(v.table.Rows))
	}
	if got := v.GetTitle(); got == "" {
		t.Fatal("GetTitle() is empty")
	}
}

func TestLogDiffFilesViewSelectedEntry(t *testing.T) {
	files := []logDiffEntry{{Status: 'M', Path: "foo.go"}}
	v := newLogDiffFilesView("/repo", "abc123", "abc123", files)

	entry, ok := v.selectedEntry()
	if !ok {
		t.Fatal("selectedEntry() reported ok=false with one row present")
	}
	if entry.Path != "foo.go" {
		t.Fatalf("selectedEntry().Path = %q, want %q", entry.Path, "foo.go")
	}
}

func TestLogDiffFilesViewSelectedEntryWithNoRows(t *testing.T) {
	v := newLogDiffFilesView("/repo", "abc123", "abc123", nil)
	if _, ok := v.selectedEntry(); ok {
		t.Fatal("selectedEntry() on an empty table should report ok=false")
	}
	// showDiff must return immediately (no background goroutine, no panic)
	// when there is nothing selected, the same contract LogView.showDiff's
	// own TestLogViewShowDiffDoesNothingWithoutASelection (logdiff_test.go)
	// checks for a single-file commit.
	v.showDiff()
}

func TestLogDiffFilesViewEscapeAndF10Close(t *testing.T) {
	for _, key := range []uint16{vtinput.VK_ESCAPE, vtinput.VK_F10} {
		v := newLogDiffFilesView("/repo", "abc123", "abc123", []logDiffEntry{{Status: 'M', Path: "foo.go"}})
		if !v.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: key}) {
			t.Fatalf("key %d was not claimed", key)
		}
		if !v.IsDone() {
			t.Fatalf("key %d did not close the view", key)
		}
	}
}

func TestLogDiffFilesViewEnterClaimsKeyWithNoRows(t *testing.T) {
	v := newLogDiffFilesView("/repo", "abc123", "abc123", nil)
	// showDiff (above) runs asynchronously and posts its result back to the
	// UI goroutine, so its outcome is not observed here -- see logdiff_test.go's
	// own TestLogViewShowDiffDoesNothingWithoutASelection for the same,
	// deliberately UI-thread-only split. What matters here is that Enter is
	// claimed even with nothing to act on.
	if !v.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN}) {
		t.Fatal("plain Enter was not claimed")
	}
}

// TestLogDiffFilesViewGetTypeIsUniqueFromCousins guards the vtui.TypeUser+N
// slot LogDiffFilesView claims (+12) against colliding with any of the
// three views before it in the same chain -- the same collision check
// TestLogViewGetTypeIsUniqueFromDiffView (logview_test.go) and
// TestBranchViewGetTypeIsUniqueFromDiffView (branchview_test.go) already
// run for their own slots.
func TestLogDiffFilesViewGetTypeIsUniqueFromCousins(t *testing.T) {
	v := newLogDiffFilesView("/repo", "abc123", "abc123", nil)
	for _, other := range []vtui.FrameType{vtui.TypeUser + 9, vtui.TypeUser + 10, vtui.TypeUser + 11} {
		if v.GetType() == other {
			t.Fatalf("LogDiffFilesView.GetType() collides with %v", other)
		}
	}
}

func TestPresentLogDiffFilesOpensLogDiffFilesView(t *testing.T) {
	files := []logDiffEntry{
		{Status: 'M', Path: "foo.go"},
		{Status: 'A', Path: "bar.go"},
	}
	presentLogDiffFiles("/repo", "abc123", "abc123", files)

	top, ok := vtui.FrameManager.GetTopFrame().(*LogDiffFilesView)
	if !ok {
		t.Fatalf("top frame after presentLogDiffFiles is %T, want *LogDiffFilesView", vtui.FrameManager.GetTopFrame())
	}
	if len(top.table.Rows) != 2 {
		t.Fatalf("rows on the opened view = %d, want 2", len(top.table.Rows))
	}
}

// TestPresentLogDiffFilesWithNilFrameManagerDoesNotPanic mirrors
// commit.go's own TestShowCommitMessageEditorWithNilFrameManagerDoesNotCallOnOk
// (commit_test.go): presentLogDiffFiles must not assume vtui.FrameManager is
// non-nil, the same guard every other "push a screen" function in this
// plugin already has.
func TestPresentLogDiffFilesWithNilFrameManagerDoesNotPanic(t *testing.T) {
	original := vtui.FrameManager
	vtui.FrameManager = nil
	defer func() { vtui.FrameManager = original }()

	presentLogDiffFiles("/repo", "abc123", "abc123", []logDiffEntry{{Status: 'M', Path: "foo.go"}})
}
