package git

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestCommitChangedFiles(t *testing.T) {
	withFakeGit(t, "M\tfoo.go\n", nil)

	got, err := commitChangedFiles(context.Background(), "/repo", "abc123")
	if err != nil {
		t.Fatalf("commitChangedFiles returned an error: %v", err)
	}
	want := []logDiffEntry{{Status: 'M', Path: "foo.go"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commitChangedFiles = %#v, want %#v", got, want)
	}
}

// TestCommitChangedFilesEmptyForAMergeCommit exercises the same "no error,
// zero entries" shape parseNameStatus's own doc comment (log.go) already
// covers, one layer up: `git show` prints nothing for a merge commit
// without -m/-c, and that must not be mistaken for a failure.
func TestCommitChangedFilesEmptyForAMergeCommit(t *testing.T) {
	withFakeGit(t, "", nil)

	got, err := commitChangedFiles(context.Background(), "/repo", "abc123")
	if err != nil {
		t.Fatalf("commitChangedFiles returned an error: %v", err)
	}
	if got != nil {
		t.Fatalf("commitChangedFiles = %#v, want nil", got)
	}
}

func TestCommitChangedFilesSurfacesGitFailure(t *testing.T) {
	withFakeGit(t, "fatal: bad object deadbeef\n", errors.New("exit status 128"))

	_, err := commitChangedFiles(context.Background(), "/repo", "deadbeef")
	if err == nil {
		t.Fatal("commitChangedFiles should fail when git show fails")
	}
}

func TestRevisionFileContentReadsAnExistingRevision(t *testing.T) {
	withFakeGit(t, "package foo\n", nil)

	got, err := revisionFileContent(context.Background(), "/repo", "abc123", "foo.go")
	if err != nil {
		t.Fatalf("revisionFileContent returned an error: %v", err)
	}
	if want := []string{"package foo"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("revisionFileContent = %v, want %v", got, want)
	}
}

// TestRevisionFileContentMissingIsNotAnError covers a root commit's missing
// parent (rev = "<hash>^") the same way TestHeadFileContentMissingAtHeadIsNotAnError
// (diff_test.go) covers HEAD.
func TestRevisionFileContentMissingIsNotAnError(t *testing.T) {
	withFakeGit(t, "fatal: path 'foo.go' does not exist in 'abc123^'\n", errors.New("exit status 128"))

	got, err := revisionFileContent(context.Background(), "/repo", "abc123^", "foo.go")
	if err != nil {
		t.Fatalf("revisionFileContent returned an error: %v, want nil", err)
	}
	if got != nil {
		t.Fatalf("revisionFileContent = %v, want nil", got)
	}
}

func TestRevisionFileContentUsesDotSlashPrefix(t *testing.T) {
	var gotArgs []string
	original := execGit
	t.Cleanup(func() { execGit = original })
	execGit = func(_ context.Context, _ string, args []string) ([]byte, error) {
		gotArgs = args
		return []byte("content\n"), nil
	}

	if _, err := revisionFileContent(context.Background(), "/repo", "abc123^", "foo.go"); err != nil {
		t.Fatalf("revisionFileContent returned an error: %v", err)
	}
	last := gotArgs[len(gotArgs)-1]
	if last != "abc123^:./foo.go" {
		t.Fatalf("git show argument = %q, want %q", last, "abc123^:./foo.go")
	}
}

// TestLogViewShowDiffDoesNothingWithoutASelection mirrors diff.go's own
// TestSelectedEntryWithNoRows: showDiff must return immediately, with no
// git invocation and no panic, when nothing is selected.
func TestLogViewShowDiffDoesNothingWithoutASelection(t *testing.T) {
	withFakeGit(t, "", nil)

	lv, err := newLogView("/repo")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := lv.selectedEntry(); ok {
		t.Fatal("selectedEntry() on an empty table should report ok=false")
	}

	var calls int
	execGit = func(context.Context, string, []string) ([]byte, error) {
		calls++
		return nil, nil
	}
	lv.showDiff()
	if calls != 0 {
		t.Fatalf("git invocations from showDiff with nothing selected = %d, want 0", calls)
	}
}
