package git

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/dialog"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestNewBranchViewLoadsBranchesSynchronously(t *testing.T) {
	withFakeGit(t, "* main\n  feature/foo\n", nil)

	bv, err := newBranchView("/repo", nil)
	if err != nil {
		t.Fatalf("newBranchView returned an error: %v", err)
	}
	if len(bv.table.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(bv.table.Rows))
	}
}

func TestNewBranchViewSurfacesGitFailure(t *testing.T) {
	withFakeGit(t, "fatal: not a git repository\n", errors.New("exit status 128"))

	_, err := newBranchView("/repo", nil)
	if err == nil {
		t.Fatal("newBranchView should fail when git branch fails")
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("error = %q, want it to surface git's own stderr", err.Error())
	}
}

func TestBranchViewF5Refreshes(t *testing.T) {
	withFakeGit(t, "* main\n", nil)

	bv, err := newBranchView("/repo", nil)
	if err != nil {
		t.Fatal(err)
	}

	execGit = func(context.Context, string, []string) ([]byte, error) {
		return []byte("* main\n  feature\n"), nil
	}

	if !bv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F5}) {
		t.Fatal("plain F5 was not claimed")
	}
	if len(bv.table.Rows) != 2 {
		t.Fatalf("rows after F5 = %d, want 2", len(bv.table.Rows))
	}
}

func TestBranchViewEscapeAndF10Close(t *testing.T) {
	for _, key := range []uint16{vtinput.VK_ESCAPE, vtinput.VK_F10} {
		withFakeGit(t, "* main\n", nil)

		bv, err := newBranchView("/repo", nil)
		if err != nil {
			t.Fatal(err)
		}
		if !bv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: key}) {
			t.Fatalf("key %d was not claimed", key)
		}
		if !bv.IsDone() {
			t.Fatalf("key %d did not close the view", key)
		}
	}
}

func TestBranchViewEnterClaimsKeyWithNoRows(t *testing.T) {
	withFakeGit(t, "", nil) // an empty `git branch --list`: reload succeeds with zero rows.

	bv, err := newBranchView("/repo", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(bv.table.Rows) != 0 {
		t.Fatalf("rows = %d, want 0", len(bv.table.Rows))
	}
	if !bv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN}) {
		t.Fatal("plain Enter was not claimed")
	}
}

func TestSwitchBranchRunsGitSwitchAndReloadsBoth(t *testing.T) {
	withFakeGit(t, "* main\n  feature\n", nil)

	statusController, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = statusController.Close() }()
	status := statusController.(*statusPanel)

	bv, err := newBranchView("/repo", status)
	if err != nil {
		t.Fatal(err)
	}
	// Move the cursor onto "feature" (display position 1: QuickSearch-less
	// sort keeps git's own order, main first).
	bv.table.SelectPos = 1

	var switchArgs []string
	execGit = func(_ context.Context, _ string, args []string) ([]byte, error) {
		if len(args) > 2 && args[2] == "switch" {
			switchArgs = args
			return []byte("Switched to branch 'feature'\n"), nil
		}
		// Both bv.reload and status.reload run after a successful switch.
		if len(args) > 2 && args[2] == "branch" {
			return []byte("  main\n* feature\n"), nil
		}
		return []byte("# branch.head feature\n"), nil
	}

	bv.switchBranch()

	want := []string{"-c", "core.quotepath=false", "switch", "feature"}
	if !reflect.DeepEqual(switchArgs, want) {
		t.Fatalf("switch args = %v, want %v", switchArgs, want)
	}
	if len(bv.table.Rows) != 2 {
		t.Fatalf("branch rows after switch = %d, want 2 (reloaded)", len(bv.table.Rows))
	}
	if status.branch != "feature" {
		t.Fatalf("status.branch after switch = %q, want %q (underlying panel reloaded)", status.branch, "feature")
	}
}

func TestSwitchBranchFailureLeavesBothPanelsUntouched(t *testing.T) {
	original := execGit
	t.Cleanup(func() { execGit = original })
	// withFakeGit's single fixed output cannot serve both setup calls below:
	// newStatusPanel's reload wants `git status --porcelain=v2 --branch`
	// shape ("# branch.head main"), newBranchView's reload wants `git branch
	// --list` shape ("* main\n  feature\n"). Feeding the branch-list shape to
	// both (as a plain withFakeGit call would) leaves status.branch == ""
	// straight out of newStatusPanel, since parseStatus never recognizes a
	// "* main" line -- silently making the "unchanged" assertion below
	// vacuous (it was already "" before switchBranch ever ran) instead of one
	// that actually proves a failed switch leaves the prior branch in place.
	execGit = func(_ context.Context, _ string, args []string) ([]byte, error) {
		if len(args) > 2 && args[2] == "status" {
			return []byte("# branch.head main\n"), nil
		}
		return []byte("* main\n  feature\n"), nil
	}

	statusController, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = statusController.Close() }()
	status := statusController.(*statusPanel)

	bv, err := newBranchView("/repo", status)
	if err != nil {
		t.Fatal(err)
	}
	bv.table.SelectPos = 1

	var reloadCalls int
	execGit = func(_ context.Context, _ string, args []string) ([]byte, error) {
		if len(args) > 2 && args[2] == "switch" {
			return []byte("error: Your local changes to the following files would be overwritten by checkout:\nfoo.go\n"),
				errors.New("exit status 1")
		}
		reloadCalls++
		return []byte("* main\n  feature\n"), nil
	}

	// Not the switch command itself -- this deliberately never attempts to
	// stash or merge on the user's behalf (branchview.go's own doc comment
	// on switchBranch): a failed switch must leave both panels exactly as
	// they were, only surfacing git's own message.
	bv.switchBranch()

	if reloadCalls != 0 {
		t.Fatalf("reload invocations after a failed switch = %d, want 0 (no reload on failure)", reloadCalls)
	}
	if status.branch != "main" {
		t.Fatalf("status.branch after a failed switch = %q, want %q (unchanged)", status.branch, "main")
	}
}

func TestSwitchBranchWithNoRowsIsANoOp(t *testing.T) {
	withFakeGit(t, "", nil)

	bv, err := newBranchView("/repo", nil)
	if err != nil {
		t.Fatal(err)
	}

	var calls int
	execGit = func(context.Context, string, []string) ([]byte, error) {
		calls++
		return nil, nil
	}

	bv.switchBranch()

	if calls != 0 {
		t.Fatalf("git invocations from switchBranch with nothing selected = %d, want 0", calls)
	}
}

func TestBranchRowGetCellText(t *testing.T) {
	row := branchRow{entry: branchEntry{Name: "main", Current: true}}
	if got := row.GetCellText(colBranchCurrent); got != "*" {
		t.Errorf("GetCellText(colBranchCurrent) = %q, want %q", got, "*")
	}
	if got := row.GetCellText(colBranchName); got != "main" {
		t.Errorf("GetCellText(colBranchName) = %q, want %q", got, "main")
	}
	if got := row.GetCellText(99); got != "" {
		t.Errorf("GetCellText(99) = %q, want empty", got)
	}

	other := branchRow{entry: branchEntry{Name: "feature", Current: false}}
	if got := other.GetCellText(colBranchCurrent); got != "" {
		t.Errorf("GetCellText(colBranchCurrent) for a non-current branch = %q, want empty", got)
	}
}

func TestBranchViewGetTypeIsUniqueFromDiffViewAndLogView(t *testing.T) {
	withFakeGit(t, "", nil)
	bv, err := newBranchView("/repo", nil)
	if err != nil {
		t.Fatal(err)
	}
	if bv.GetType() == vtui.TypeUser+9 {
		t.Fatalf("BranchView.GetType() collides with DiffView's vtui.TypeUser+9")
	}
	if bv.GetType() == vtui.TypeUser+10 {
		t.Fatalf("BranchView.GetType() collides with LogView's vtui.TypeUser+10")
	}
}

func TestStatusPanelCtrlSOpensBranches(t *testing.T) {
	withFakeGit(t, "# branch.head main\n", nil)

	controller, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = controller.Close() }()

	if !controller.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_S,
		ControlKeyState: vtinput.LeftCtrlPressed}) {
		t.Fatal("Ctrl+S was not claimed")
	}
}

func TestBranchViewInsertOpensNewBranchDialogWithoutCallingGitYet(t *testing.T) {
	withFakeGit(t, "* main\n", nil)

	bv, err := newBranchView("/repo", nil)
	if err != nil {
		t.Fatal(err)
	}

	var calls int
	execGit = func(context.Context, string, []string) ([]byte, error) {
		calls++
		return []byte("* main\n"), nil
	}

	if !bv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_INSERT}) {
		t.Fatal("plain Insert was not claimed")
	}
	if calls != 0 {
		t.Fatalf("git invocations right after Insert = %d, want 0 (the name dialog is still waiting for input)", calls)
	}
	if _, ok := vtui.FrameManager.GetTopFrame().(*dialog.FileDialog); !ok {
		t.Fatalf("top frame after Insert is %T, want *dialog.FileDialog (the new-branch prompt)", vtui.FrameManager.GetTopFrame())
	}
}

func TestOnNewBranchEnteredRejectsBlankName(t *testing.T) {
	withFakeGit(t, "* main\n", nil)

	bv, err := newBranchView("/repo", nil)
	if err != nil {
		t.Fatal(err)
	}

	var calls int
	execGit = func(context.Context, string, []string) ([]byte, error) {
		calls++
		return nil, nil
	}

	for _, blank := range []string{"", "   ", "\t\n"} {
		bv.onNewBranchEntered(blank)
	}
	if calls != 0 {
		t.Fatalf("git invocations for a blank branch name = %d, want 0", calls)
	}
}

func TestOnNewBranchEnteredTrimsAndCreatesBranch(t *testing.T) {
	withFakeGit(t, "* main\n", nil)

	bv, err := newBranchView("/repo", nil)
	if err != nil {
		t.Fatal(err)
	}

	var branchArgs []string
	execGit = func(_ context.Context, _ string, args []string) ([]byte, error) {
		if len(args) > 2 && args[2] == "branch" && len(args) == 4 {
			branchArgs = args
			return nil, nil
		}
		// bv.reload runs after a successful create.
		return []byte("* main\n  feature\n"), nil
	}

	bv.onNewBranchEntered("  feature  ")

	want := []string{"-c", "core.quotepath=false", "branch", "feature"}
	if !reflect.DeepEqual(branchArgs, want) {
		t.Fatalf("branch args = %v, want %v", branchArgs, want)
	}
	if len(bv.table.Rows) != 2 {
		t.Fatalf("rows after creating a branch = %d, want 2 (reloaded)", len(bv.table.Rows))
	}
}

func TestOnNewBranchEnteredFailureLeavesListUntouched(t *testing.T) {
	withFakeGit(t, "* main\n", nil)

	bv, err := newBranchView("/repo", nil)
	if err != nil {
		t.Fatal(err)
	}

	var reloadCalls int
	execGit = func(_ context.Context, _ string, args []string) ([]byte, error) {
		if len(args) > 2 && args[2] == "branch" && len(args) == 4 {
			return []byte("fatal: a branch named 'feature' already exists\n"), errors.New("exit status 128")
		}
		reloadCalls++
		return []byte("* main\n"), nil
	}

	bv.onNewBranchEntered("feature")

	if reloadCalls != 0 {
		t.Fatalf("reload invocations after a failed create = %d, want 0 (no reload on failure)", reloadCalls)
	}
	if len(bv.table.Rows) != 1 {
		t.Fatalf("rows after a failed create = %d, want 1 (unchanged)", len(bv.table.Rows))
	}
}

func TestBranchViewDeleteAndF8ClaimTheDeleteGesture(t *testing.T) {
	for _, key := range []uint16{vtinput.VK_DELETE, vtinput.VK_F8} {
		withFakeGit(t, "* main\n  feature\n", nil)

		bv, err := newBranchView("/repo", nil)
		if err != nil {
			t.Fatal(err)
		}
		bv.table.SelectPos = 1 // "feature", not the current branch.

		if !bv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: key}) {
			t.Fatalf("key %d was not claimed", key)
		}
	}
}

func TestDeleteBranchOnTheCurrentBranchNeverOpensAConfirmDialog(t *testing.T) {
	withFakeGit(t, "* main\n  feature\n", nil)

	bv, err := newBranchView("/repo", nil)
	if err != nil {
		t.Fatal(err)
	}
	bv.table.SelectPos = 0 // "main", the current branch (branchEntry.Current).

	var calls int
	execGit = func(context.Context, string, []string) ([]byte, error) {
		calls++
		return nil, nil
	}
	before := vtui.FrameManager.GetTopFrame()

	bv.deleteBranch()

	if calls != 0 {
		t.Fatalf("git invocations for deleting the current branch = %d, want 0", calls)
	}
	if after := vtui.FrameManager.GetTopFrame(); after != before {
		t.Fatalf("deleteBranch on the current branch pushed a new frame (%T), want the guard to toast and return", after)
	}
}

func TestDeleteBranchWithNoRowsIsANoOp(t *testing.T) {
	withFakeGit(t, "", nil)

	bv, err := newBranchView("/repo", nil)
	if err != nil {
		t.Fatal(err)
	}

	var calls int
	execGit = func(context.Context, string, []string) ([]byte, error) {
		calls++
		return nil, nil
	}
	before := vtui.FrameManager.GetTopFrame()

	bv.deleteBranch()

	if calls != 0 {
		t.Fatalf("git invocations from deleteBranch with nothing selected = %d, want 0", calls)
	}
	if after := vtui.FrameManager.GetTopFrame(); after != before {
		t.Fatalf("deleteBranch with nothing selected pushed a new frame (%T), want it to just return", after)
	}
}

func TestDeleteBranchConfirmedRunsGitBranchDAndReloads(t *testing.T) {
	withFakeGit(t, "* main\n  feature\n", nil)

	bv, err := newBranchView("/repo", nil)
	if err != nil {
		t.Fatal(err)
	}
	bv.table.SelectPos = 1 // "feature", not the current branch.

	var deleteArgs []string
	execGit = func(_ context.Context, _ string, args []string) ([]byte, error) {
		// Both the delete itself and bv.reload's own `git branch --list`
		// afterward share args[2] == "branch" -- args[3] tells them apart.
		if len(args) > 3 && args[2] == "branch" && args[3] == "-d" {
			deleteArgs = args
			return []byte("Deleted branch feature (was abc1234).\n"), nil
		}
		// bv.reload runs after a successful delete.
		return []byte("* main\n"), nil
	}

	bv.deleteBranch()

	confirm, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok || confirm.OnResult == nil {
		t.Fatalf("top frame after deleteBranch is %T, want the *vtui.Window confirm dialog", vtui.FrameManager.GetTopFrame())
	}
	confirm.OnResult(0) // The dialog's first button ("&Delete").

	want := []string{"-c", "core.quotepath=false", "branch", "-d", "feature"}
	if !reflect.DeepEqual(deleteArgs, want) {
		t.Fatalf("delete args = %v, want %v", deleteArgs, want)
	}
	if len(bv.table.Rows) != 1 {
		t.Fatalf("rows after delete = %d, want 1 (reloaded)", len(bv.table.Rows))
	}
}

func TestDeleteBranchCancelledNeverCallsGit(t *testing.T) {
	withFakeGit(t, "* main\n  feature\n", nil)

	bv, err := newBranchView("/repo", nil)
	if err != nil {
		t.Fatal(err)
	}
	bv.table.SelectPos = 1

	var calls int
	execGit = func(context.Context, string, []string) ([]byte, error) {
		calls++
		return nil, nil
	}

	bv.deleteBranch()

	confirm, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok || confirm.OnResult == nil {
		t.Fatalf("top frame after deleteBranch is %T, want the *vtui.Window confirm dialog", vtui.FrameManager.GetTopFrame())
	}
	confirm.OnResult(1) // The dialog's second button (vtui.Cancel).

	if calls != 0 {
		t.Fatalf("git invocations after cancelling the delete confirmation = %d, want 0", calls)
	}
}

func TestDeleteBranchFailureLeavesTheListUntouched(t *testing.T) {
	withFakeGit(t, "* main\n  feature\n", nil)

	bv, err := newBranchView("/repo", nil)
	if err != nil {
		t.Fatal(err)
	}
	bv.table.SelectPos = 1

	var reloadCalls int
	execGit = func(_ context.Context, _ string, args []string) ([]byte, error) {
		// Both the delete itself and bv.reload's own `git branch --list`
		// afterward share args[2] == "branch" -- args[3] tells them apart.
		if len(args) > 3 && args[2] == "branch" && args[3] == "-d" {
			return []byte("error: The branch 'feature' is not fully merged.\n"), errors.New("exit status 1")
		}
		reloadCalls++
		return []byte("* main\n  feature\n"), nil
	}

	bv.deleteBranch()

	confirm, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok || confirm.OnResult == nil {
		t.Fatalf("top frame after deleteBranch is %T, want the *vtui.Window confirm dialog", vtui.FrameManager.GetTopFrame())
	}
	confirm.OnResult(0)

	if reloadCalls != 0 {
		t.Fatalf("reload invocations after a failed delete = %d, want 0 (no reload on failure)", reloadCalls)
	}
	if len(bv.table.Rows) != 2 {
		t.Fatalf("rows after a failed delete = %d, want 2 (unchanged)", len(bv.table.Rows))
	}
}

func TestPlainSFallsThroughToTheTable(t *testing.T) {
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

	// Plain 'S' (no Ctrl) must be left for QuickSearch, not treated as the
	// branch gesture -- the same reasoning commit.go's own
	// TestPlainKFallsThroughToTheTable documents for Ctrl+K.
	controller.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_S})

	if calls != 0 {
		t.Fatalf("git invocations after plain 'S' = %d, want 0 (not the branch gesture)", calls)
	}
}
