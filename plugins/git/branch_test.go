package git

import (
	"reflect"
	"testing"
)

func TestParseBranchList(t *testing.T) {
	output := []byte("* main\n  feature/foo\n+ other-worktree\n")
	got := parseBranchList(output)
	want := []branchEntry{
		{Name: "main", Current: true},
		{Name: "feature/foo", Current: false},
		{Name: "other-worktree", Current: false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseBranchList = %#v, want %#v", got, want)
	}
}

func TestParseBranchListSkipsDetachedHeadPlaceholder(t *testing.T) {
	// git-branch(1)'s own placeholder for a detached HEAD or an in-progress
	// rebase: it names no local branch a checkout/switch could target.
	got := parseBranchList([]byte("* (HEAD detached at abcd123)\n  main\n"))
	want := []branchEntry{{Name: "main", Current: false}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseBranchList = %#v, want %#v", got, want)
	}
}

func TestParseBranchListSkipsBlankLines(t *testing.T) {
	got := parseBranchList([]byte("\n* main\n\n  feature\n\n"))
	want := []branchEntry{
		{Name: "main", Current: true},
		{Name: "feature", Current: false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseBranchList = %#v, want %#v", got, want)
	}
}

func TestParseBranchListEmpty(t *testing.T) {
	if got := parseBranchList([]byte("")); got != nil {
		t.Fatalf("parseBranchList(empty) = %#v, want nil", got)
	}
}
