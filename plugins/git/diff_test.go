package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
)

func TestHeadPath(t *testing.T) {
	if got := headPath(statusEntry{Path: "foo.go"}); got != "foo.go" {
		t.Errorf("headPath(plain) = %q, want %q", got, "foo.go")
	}
	if got := headPath(statusEntry{Path: "new.go", OrigPath: "old.go"}); got != "old.go" {
		t.Errorf("headPath(rename) = %q, want the old name %q", got, "old.go")
	}
}

func TestHeadTitle(t *testing.T) {
	if got := headTitle(statusEntry{Path: "foo.go"}); got != "HEAD:foo.go" {
		t.Errorf("headTitle(plain) = %q, want %q", got, "HEAD:foo.go")
	}
	if got := headTitle(statusEntry{Path: "new.go", OrigPath: "old.go"}); got != "HEAD:old.go" {
		t.Errorf("headTitle(rename) = %q, want %q", got, "HEAD:old.go")
	}
}

func TestDiffSideFromBytes(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    []string
		wantErr bool
	}{
		{"multiline", []byte("one\ntwo\nthree\n"), []string{"one", "two", "three"}, false},
		{"no trailing newline", []byte("one\ntwo"), []string{"one", "two"}, false},
		{"empty", []byte(""), nil, false},
		{"binary", []byte("one\x00two"), nil, true},
		{"too large", make([]byte, diffMaxFileSize+1), nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := diffSideFromBytes(tt.data)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("diffSideFromBytes(%s) = %v, nil, want an error", tt.name, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("diffSideFromBytes(%s) returned an error: %v", tt.name, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("diffSideFromBytes(%s) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestHeadFileContentReturnsHeadVersion(t *testing.T) {
	withFakeGit(t, "package foo\n\nfunc Bar() {}\n", nil)

	got, err := headFileContent(context.Background(), "/repo", "foo.go")
	if err != nil {
		t.Fatalf("headFileContent returned an error: %v", err)
	}
	want := []string{"package foo", "", "func Bar() {}"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("headFileContent = %v, want %v", got, want)
	}
}

func TestHeadFileContentMissingAtHeadIsNotAnError(t *testing.T) {
	// A new/untracked file (or an unborn branch with no HEAD at all) makes
	// `git show HEAD:./path` fail; that failure is exactly what an empty
	// HEAD side -- the file shows as entirely inserted -- means, not
	// something to report as an error.
	withFakeGit(t, "fatal: path 'foo.go' does not exist in 'HEAD'\n", errors.New("exit status 128"))

	got, err := headFileContent(context.Background(), "/repo", "foo.go")
	if err != nil {
		t.Fatalf("headFileContent returned an error: %v, want nil", err)
	}
	if got != nil {
		t.Fatalf("headFileContent = %v, want nil", got)
	}
}

func TestHeadFileContentUsesDotSlashPrefix(t *testing.T) {
	// gitrevisions(7): a bare "HEAD:path" resolves relative to the
	// repository's top level, not to the directory git was run in. Every
	// call here must use "HEAD:./path" instead, so it agrees with
	// entry.Path/OrigPath, which `git status` already reported relative to
	// that same directory.
	var gotArgs []string
	original := execGit
	t.Cleanup(func() { execGit = original })
	execGit = func(_ context.Context, _ string, args []string) ([]byte, error) {
		gotArgs = args
		return []byte("content\n"), nil
	}

	if _, err := headFileContent(context.Background(), "/repo/sub", "foo.go"); err != nil {
		t.Fatalf("headFileContent returned an error: %v", err)
	}
	last := gotArgs[len(gotArgs)-1]
	if last != "HEAD:./foo.go" {
		t.Fatalf("git show argument = %q, want %q", last, "HEAD:./foo.go")
	}
}

func TestWorktreeFileContentReadsAnExistingFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "foo.go"), []byte("a\nb\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := worktreeFileContent(dir, "foo.go")
	if err != nil {
		t.Fatalf("worktreeFileContent returned an error: %v", err)
	}
	if want := []string{"a", "b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("worktreeFileContent = %v, want %v", got, want)
	}
}

func TestWorktreeFileContentMissingFileIsNotAnError(t *testing.T) {
	dir := t.TempDir()

	got, err := worktreeFileContent(dir, "gone.go")
	if err != nil {
		t.Fatalf("worktreeFileContent returned an error: %v, want nil", err)
	}
	if got != nil {
		t.Fatalf("worktreeFileContent = %v, want nil", got)
	}
}

func TestWorktreeFileContentSurfacesOtherErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "adir"), 0o700); err != nil {
		t.Fatal(err)
	}

	if _, err := worktreeFileContent(dir, "adir"); err == nil {
		t.Fatal("worktreeFileContent(a directory) should return an error")
	}
}

func TestWorktreeFileContentNestedPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "internal", "app"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "internal", "app", "foo.go"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := worktreeFileContent(dir, "internal/app/foo.go")
	if err != nil {
		t.Fatalf("worktreeFileContent returned an error: %v", err)
	}
	if want := []string{"x"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("worktreeFileContent = %v, want %v", got, want)
	}
}

// TestStatusPanelEnterClaimsKeyWithNoRows exercises the ProcessKey
// wiring itself: pressing Enter must be claimed (showDiff runs asynchronously
// and posts its result back to the UI goroutine, so its outcome is not
// observed here -- see internal/app/compare_content_ui.go's own
// actionCompareFilesByContent for the same, deliberately UI-thread-only
// split) even when the panel has no rows to act on at all.
func TestStatusPanelEnterClaimsKeyWithNoRows(t *testing.T) {
	withFakeGit(t, "# branch.head main\n", nil)

	controller, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = controller.Close() }()

	panel := controller.(*statusPanel)
	if len(panel.table.Rows) != 0 {
		t.Fatalf("rows = %d, want 0 for a clean repo", len(panel.table.Rows))
	}

	if !controller.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN}) {
		t.Fatal("plain Enter was not claimed")
	}
}

func TestSelectedEntryWithNoRows(t *testing.T) {
	withFakeGit(t, "# branch.head main\n", nil)

	controller, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = controller.Close() }()

	panel := controller.(*statusPanel)
	if _, ok := panel.selectedEntry(); ok {
		t.Fatal("selectedEntry() on an empty table should report ok=false")
	}
	// showDiff must return immediately (no background goroutine, no panic)
	// when there is nothing selected.
	panel.showDiff()
}
