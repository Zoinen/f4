package git

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
)

func TestStageActionForStagesUntrackedAndUnstagedChanges(t *testing.T) {
	tests := []struct {
		xy   string
		want stageAction
	}{
		{"??", stageActionAdd},     // untracked
		{" M", stageActionAdd},     // modified in the worktree, nothing staged yet
		{" D", stageActionAdd},     // deleted in the worktree, nothing staged yet
		{"M ", stageActionRestore}, // staged modification, clean worktree
		{"A ", stageActionRestore}, // staged add
		{"MM", stageActionRestore}, // staged modification, further worktree changes
		{"R ", stageActionRestore}, // staged rename
	}
	for _, tt := range tests {
		got := stageActionFor(statusEntry{XY: tt.xy})
		if got != tt.want {
			t.Errorf("stageActionFor(XY=%q) = %v, want %v", tt.xy, got, tt.want)
		}
	}
}

func TestStageActionForConflictsAlwaysAdd(t *testing.T) {
	for xy := range unmergedXY {
		if got := stageActionFor(statusEntry{XY: xy}); got != stageActionAdd {
			t.Errorf("stageActionFor(XY=%q) = %v, want stageActionAdd (resolve)", xy, got)
		}
	}
}

func TestStagePathsForPlainAndRename(t *testing.T) {
	plain := stagePathsFor(statusEntry{Path: "foo.go"})
	if want := []string{"foo.go"}; !reflect.DeepEqual(plain, want) {
		t.Errorf("stagePathsFor(plain) = %v, want %v", plain, want)
	}

	renamed := stagePathsFor(statusEntry{Path: "new.go", OrigPath: "old.go"})
	if want := []string{"old.go", "new.go"}; !reflect.DeepEqual(renamed, want) {
		t.Errorf("stagePathsFor(rename) = %v, want %v", renamed, want)
	}
}

func TestToggleStageStagesAnUntrackedFile(t *testing.T) {
	withFakeGit(t, "# branch.head main\n? new.txt\n", nil)

	controller, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = controller.Close() }()
	panel := controller.(*statusPanel)

	// After staging, the next `git status` the fake returns reports the file
	// as a staged add instead of untracked -- toggleStage's own reload
	// (status.go's reload, not this test) is what should trigger that call.
	var calls [][]string
	execGit = func(_ context.Context, _ string, args []string) ([]byte, error) {
		calls = append(calls, args)
		if args[2] == "add" {
			return []byte(""), nil
		}
		return []byte("# branch.head main\n1 A. N... 000000 100644 100644 0000000 aaaaaaa new.txt\n"), nil
	}

	if !controller.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_INSERT}) {
		t.Fatal("plain Insert was not claimed")
	}

	if len(calls) != 2 {
		t.Fatalf("git invocations = %d, want 2 (add, then a status reload)", len(calls))
	}
	addArgs := calls[0]
	want := []string{"-c", "core.quotepath=false", "add", "--", "new.txt"}
	if !reflect.DeepEqual(addArgs, want) {
		t.Fatalf("add args = %v, want %v", addArgs, want)
	}

	if len(panel.table.Rows) != 1 {
		t.Fatalf("rows after stage = %d, want 1", len(panel.table.Rows))
	}
	row := panel.table.Rows[panel.table.RowAt(panel.table.SelectPos)].(statusRow)
	if row.entry.XY != "A." {
		t.Fatalf("entry after stage: XY = %q, want %q", row.entry.XY, "A.")
	}
	if row.entry.Path != "new.txt" {
		t.Fatalf("cursor after stage is on %q, want it to stay on %q", row.entry.Path, "new.txt")
	}
}

func TestToggleStageUnstagesAStagedFile(t *testing.T) {
	withFakeGit(t, "# branch.head main\n1 M. N... 100644 100644 100644 aaaaaaa bbbbbbb staged.go\n", nil)

	controller, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = controller.Close() }()
	panel := controller.(*statusPanel)

	var restoreArgs []string
	execGit = func(_ context.Context, _ string, args []string) ([]byte, error) {
		if args[2] == "restore" {
			restoreArgs = args
			return []byte(""), nil
		}
		return []byte("# branch.head main\n1 .M N... 100644 100644 100644 aaaaaaa bbbbbbb staged.go\n"), nil
	}

	if !controller.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_INSERT}) {
		t.Fatal("plain Insert was not claimed")
	}

	want := []string{"-c", "core.quotepath=false", "restore", "--staged", "--", "staged.go"}
	if !reflect.DeepEqual(restoreArgs, want) {
		t.Fatalf("restore args = %v, want %v", restoreArgs, want)
	}

	row := panel.table.Rows[panel.table.RowAt(panel.table.SelectPos)].(statusRow)
	if row.entry.XY != ".M" {
		t.Fatalf("entry after unstage: XY = %q, want %q", row.entry.XY, ".M")
	}
}

func TestToggleStageUsesBothPathsForARename(t *testing.T) {
	withFakeGit(t, "# branch.head main\n2 R. N... 100644 100644 100644 aaaaaaa aaaaaaa R100 new_name.go\told_name.go\n", nil)

	controller, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = controller.Close() }()

	var restoreArgs []string
	execGit = func(_ context.Context, _ string, args []string) ([]byte, error) {
		if args[2] == "restore" {
			restoreArgs = args
			return []byte(""), nil
		}
		return []byte("# branch.head main\n"), nil
	}

	controller.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_INSERT})

	want := []string{"-c", "core.quotepath=false", "restore", "--staged", "--", "old_name.go", "new_name.go"}
	if !reflect.DeepEqual(restoreArgs, want) {
		t.Fatalf("restore args for a rename = %v, want %v", restoreArgs, want)
	}
}

func TestToggleStageResolvesAConflictWithAdd(t *testing.T) {
	withFakeGit(t, "# branch.head main\nu UU N... 100644 100644 100644 100644 aaaaaaa bbbbbbb ccccccc conflicted.txt\n", nil)

	controller, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = controller.Close() }()

	var addArgs []string
	execGit = func(_ context.Context, _ string, args []string) ([]byte, error) {
		if args[2] == "add" {
			addArgs = args
			return []byte(""), nil
		}
		return []byte("# branch.head main\n1 M. N... 100644 100644 100644 aaaaaaa bbbbbbb conflicted.txt\n"), nil
	}

	controller.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_INSERT})

	want := []string{"-c", "core.quotepath=false", "add", "--", "conflicted.txt"}
	if !reflect.DeepEqual(addArgs, want) {
		t.Fatalf("add args for a conflict = %v, want %v", addArgs, want)
	}
}

func TestToggleStageLeavesRowsUnchangedWhenGitFails(t *testing.T) {
	withFakeGit(t, "# branch.head main\n? new.txt\n", nil)

	controller, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = controller.Close() }()
	panel := controller.(*statusPanel)

	var statusCalls int
	execGit = func(_ context.Context, _ string, args []string) ([]byte, error) {
		if args[2] == "add" {
			return []byte("fatal: pathspec 'new.txt' did not match any files\n"), errors.New("exit status 128")
		}
		statusCalls++
		return []byte("# branch.head main\n? new.txt\n"), nil
	}

	controller.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_INSERT})

	if statusCalls != 0 {
		t.Fatalf("reload ran %d times after a failed git add, want 0 (no reload on failure)", statusCalls)
	}
	if len(panel.table.Rows) != 1 {
		t.Fatalf("rows after a failed git add = %d, want 1 (unchanged)", len(panel.table.Rows))
	}
	row := panel.table.Rows[0].(statusRow)
	if row.entry.XY != "??" {
		t.Fatalf("entry after a failed git add: XY = %q, want unchanged %q", row.entry.XY, "??")
	}
}

func TestToggleStageDoesNothingWithoutASelection(t *testing.T) {
	withFakeGit(t, "# branch.head main\n", nil)

	controller, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = controller.Close() }()

	var calls int
	execGit = func(_ context.Context, _ string, _ []string) ([]byte, error) {
		calls++
		return []byte("# branch.head main\n"), nil
	}

	if !controller.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_INSERT}) {
		t.Fatal("plain Insert should still be claimed by this panel even with an empty list")
	}
	if calls != 0 {
		t.Fatalf("git invocations with nothing selected = %d, want 0", calls)
	}
}

func TestCtrlInsertFallsThroughToTheTable(t *testing.T) {
	withFakeGit(t, "# branch.head main\n? new.txt\n", nil)

	controller, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = controller.Close() }()

	var calls int
	execGit = func(_ context.Context, _ string, _ []string) ([]byte, error) {
		calls++
		return []byte("# branch.head main\n? new.txt\n"), nil
	}

	// Ctrl+Insert (a common "copy" chord) is not this panel's stage gesture
	// and must not be swallowed as if it were plain Insert.
	controller.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_INSERT,
		ControlKeyState: vtinput.LeftCtrlPressed})

	if calls != 0 {
		t.Fatalf("git invocations after Ctrl+Insert = %d, want 0 (not the stage gesture)", calls)
	}
}

func TestRestoreSelectionByPathFindsMatchingRowAfterReload(t *testing.T) {
	withFakeGit(t, "# branch.head main\n? a.txt\n? b.txt\n? c.txt\n", nil)

	controller, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = controller.Close() }()
	panel := controller.(*statusPanel)

	panel.restoreSelectionByPath("c.txt")
	got := panel.table.Rows[panel.table.RowAt(panel.table.SelectPos)].(statusRow)
	if got.entry.Path != "c.txt" {
		t.Fatalf("restoreSelectionByPath(%q): cursor is on %q", "c.txt", got.entry.Path)
	}

	before := panel.table.SelectPos
	panel.restoreSelectionByPath("missing.txt")
	if panel.table.SelectPos != before {
		t.Fatalf("restoreSelectionByPath(missing path) moved the cursor from %d to %d, want unchanged", before, panel.table.SelectPos)
	}
}
