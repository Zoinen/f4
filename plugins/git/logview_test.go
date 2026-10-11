package git

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestNewLogViewLoadsLogSynchronously(t *testing.T) {
	withFakeGit(t, "aaaa1111"+logFieldSep+"aaaa111"+logFieldSep+"Alice"+logFieldSep+"2026-09-28"+logFieldSep+"First\n"+
		"bbbb2222"+logFieldSep+"bbbb222"+logFieldSep+"Bob"+logFieldSep+"2026-09-27"+logFieldSep+"Second", nil)

	lv, err := newLogView("/repo")
	if err != nil {
		t.Fatalf("newLogView returned an error: %v", err)
	}
	if len(lv.table.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(lv.table.Rows))
	}
}

func TestNewLogViewSurfacesGitFailure(t *testing.T) {
	withFakeGit(t, "fatal: your current branch 'main' does not have any commits yet\n", errors.New("exit status 128"))

	_, err := newLogView("/repo")
	if err == nil {
		t.Fatal("newLogView should fail when git log fails")
	}
	if !strings.Contains(err.Error(), "does not have any commits yet") {
		t.Fatalf("error = %q, want it to surface git's own stderr", err.Error())
	}
}

func TestLogViewF5Refreshes(t *testing.T) {
	withFakeGit(t, "aaaa"+logFieldSep+"a"+logFieldSep+"Alice"+logFieldSep+"2026-09-28"+logFieldSep+"one", nil)

	lv, err := newLogView("/repo")
	if err != nil {
		t.Fatal(err)
	}

	// Second load reports two commits: a real refresh must replace, not
	// append to, the first load's rows.
	execGit = func(context.Context, string, []string) ([]byte, error) {
		return []byte("aaaa" + logFieldSep + "a" + logFieldSep + "Alice" + logFieldSep + "2026-09-28" + logFieldSep + "one\n" +
			"bbbb" + logFieldSep + "b" + logFieldSep + "Bob" + logFieldSep + "2026-09-27" + logFieldSep + "two"), nil
	}

	if !lv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F5}) {
		t.Fatal("plain F5 was not claimed")
	}
	if len(lv.table.Rows) != 2 {
		t.Fatalf("rows after F5 = %d, want 2", len(lv.table.Rows))
	}
}

func TestLogViewF5RefreshFailureShowsToastAndKeepsOldRows(t *testing.T) {
	withFakeGit(t, "aaaa"+logFieldSep+"a"+logFieldSep+"Alice"+logFieldSep+"2026-09-28"+logFieldSep+"one", nil)

	lv, err := newLogView("/repo")
	if err != nil {
		t.Fatal(err)
	}

	execGit = func(context.Context, string, []string) ([]byte, error) {
		return []byte("fatal: not a git repository\n"), errors.New("exit status 128")
	}

	if !lv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F5}) {
		t.Fatal("plain F5 was not claimed")
	}
	if len(lv.table.Rows) != 1 {
		t.Fatalf("rows after a failed F5 = %d, want 1 (unchanged)", len(lv.table.Rows))
	}
}

func TestLogViewEscapeAndF10Close(t *testing.T) {
	for _, key := range []uint16{vtinput.VK_ESCAPE, vtinput.VK_F10} {
		withFakeGit(t, "aaaa"+logFieldSep+"a"+logFieldSep+"Alice"+logFieldSep+"2026-09-28"+logFieldSep+"one", nil)

		lv, err := newLogView("/repo")
		if err != nil {
			t.Fatal(err)
		}
		if !lv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: key}) {
			t.Fatalf("key %d was not claimed", key)
		}
		if !lv.IsDone() {
			t.Fatalf("key %d did not close the view", key)
		}
	}
}

func TestLogViewEnterClaimsKeyWithNoRows(t *testing.T) {
	withFakeGit(t, "", nil) // an empty `git log` (nothing parses out of it): reload succeeds with zero rows.

	lv, err := newLogView("/repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(lv.table.Rows) != 0 {
		t.Fatalf("rows = %d, want 0", len(lv.table.Rows))
	}
	// showDiff (logdiff.go) runs asynchronously and posts its result back to
	// the UI goroutine, so its outcome is not observed here -- see diff.go's
	// own TestStatusPanelEnterClaimsKeyWithNoRows for the same,
	// deliberately UI-thread-only split. What matters here is that Enter is
	// claimed even with nothing to act on.
	if !lv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN}) {
		t.Fatal("plain Enter was not claimed")
	}
}

func TestStatusPanelCtrlEOpensLog(t *testing.T) {
	withFakeGit(t, "# branch.head main\n", nil)

	controller, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = controller.Close() }()

	if !controller.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_E,
		ControlKeyState: vtinput.LeftCtrlPressed}) {
		t.Fatal("Ctrl+E was not claimed")
	}
}

func TestPlainEFallsThroughToTheTable(t *testing.T) {
	withFakeGit(t, "# branch.head main\n1 M. N... 100644 100644 100644 aaaaaaa bbbbbbb staged.go\n", nil)

	controller, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = controller.Close() }()

	var calls int
	execGit = func(context.Context, string, []string) ([]byte, error) {
		calls++
		return []byte("# branch.head main\n1 M. N... 100644 100644 100644 aaaaaaa bbbbbbb staged.go\n"), nil
	}

	// Plain 'E' (no Ctrl) must be left for QuickSearch, not treated as the
	// log gesture -- the same reasoning commit.go's own
	// TestPlainKFallsThroughToTheTable documents for Ctrl+K.
	controller.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_E})

	if calls != 0 {
		t.Fatalf("git invocations after plain 'E' = %d, want 0 (not the log gesture)", calls)
	}
}

func TestLogRowGetCellText(t *testing.T) {
	row := logRow{entry: logEntry{ShortHash: "abc1234", Author: "Alice", Date: "2026-09-28", Subject: "Fix the thing"}}
	if got := row.GetCellText(colHash); got != "abc1234" {
		t.Errorf("GetCellText(colHash) = %q, want %q", got, "abc1234")
	}
	if got := row.GetCellText(colAuthor); got != "Alice" {
		t.Errorf("GetCellText(colAuthor) = %q, want %q", got, "Alice")
	}
	if got := row.GetCellText(colDate); got != "2026-09-28" {
		t.Errorf("GetCellText(colDate) = %q, want %q", got, "2026-09-28")
	}
	if got := row.GetCellText(colSubject); got != "Fix the thing" {
		t.Errorf("GetCellText(colSubject) = %q, want %q", got, "Fix the thing")
	}
	if got := row.GetCellText(99); got != "" {
		t.Errorf("GetCellText(99) = %q, want empty", got)
	}
}

func TestLogViewGetTypeIsUniqueFromDiffView(t *testing.T) {
	withFakeGit(t, "", nil)
	lv, err := newLogView("/repo")
	if err != nil {
		t.Fatal(err)
	}
	// vtui.TypeUser+9 is internal/diffview.DiffView's own slot (see that
	// package's doc comment on the convention); this view must not collide
	// with it.
	if lv.GetType() == vtui.TypeUser+9 {
		t.Fatalf("LogView.GetType() collides with DiffView's vtui.TypeUser+9")
	}
}
