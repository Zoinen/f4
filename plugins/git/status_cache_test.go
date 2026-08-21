package gitplugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
)

func TestBuildRepositoryStatusClassifiesFilesAndAggregatesDirectories(t *testing.T) {
	status := gogit.Status{
		"staged.txt":      &gogit.FileStatus{Staging: gogit.Added, Worktree: gogit.Unmodified},
		"nested/work.txt": &gogit.FileStatus{Staging: gogit.Unmodified, Worktree: gogit.Modified},
		"nested/both.txt": &gogit.FileStatus{Staging: gogit.Modified, Worktree: gogit.Modified},
		"conflict.txt":    &gogit.FileStatus{Staging: gogit.UpdatedButUnmerged, Worktree: gogit.UpdatedButUnmerged},
		"ignored.tmp":     &gogit.FileStatus{Staging: gogit.Ignored, Worktree: gogit.Ignored},
	}

	snapshot := buildRepositoryStatus("repo", status)
	if got, want := snapshot.entries["staged.txt"].Class, statusStaged; got != want {
		t.Errorf("staged class = %v, want %v", got, want)
	}
	if got, want := snapshot.entries["nested/work.txt"].Class, statusUnstaged; got != want {
		t.Errorf("worktree class = %v, want %v", got, want)
	}
	if got, want := snapshot.entries["nested/both.txt"].Class, statusBoth; got != want {
		t.Errorf("both class = %v, want %v", got, want)
	}
	if got, want := snapshot.entries["conflict.txt"].Class, statusConflict; got != want {
		t.Errorf("conflict class = %v, want %v", got, want)
	}
	if got, want := snapshot.entries["ignored.tmp"].Class, statusIgnored; got != want {
		t.Errorf("ignored class = %v, want %v", got, want)
	}
	if got, want := snapshot.directories["nested"], statusBoth; got != want {
		t.Errorf("nested aggregate = %v, want %v", got, want)
	}
}

func TestReadRepositoryStatusUsesContextAwareForkStatus(t *testing.T) {
	root := t.TempDir()
	repository, err := gogit.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	worktree, err := repository.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := worktree.StageContext(context.Background(), "tracked.txt"); err != nil {
		t.Fatalf("stage initial file: %v", err)
	}
	if _, err := worktree.CommitContext(context.Background(), "initial", &gogit.CommitOptions{Author: &object.Signature{
		Name:  "f4 test",
		Email: "f4@example.invalid",
		When:  time.Now(),
	}}); err != nil {
		t.Fatalf("initial commit: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	snapshot, err := readRepositoryStatus(context.Background(), Repository{Root: root})
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if got, want := snapshot.entries["tracked.txt"].Class, statusUnstaged; got != want {
		t.Errorf("tracked class = %v, want %v", got, want)
	}
	if got, want := snapshot.entries["new.txt"].Class, statusUntracked; got != want {
		t.Errorf("untracked class = %v, want %v", got, want)
	}
}

func TestMergeStatusClassPrioritizesConflictAndCombinedLayers(t *testing.T) {
	cases := []struct {
		name        string
		left, right statusClass
		want        statusClass
	}{
		{"clean", statusClean, statusUnstaged, statusUnstaged},
		{"staged plus unstaged", statusStaged, statusUnstaged, statusBoth},
		{"conflict", statusBoth, statusConflict, statusConflict},
		{"untracked beats ignored", statusIgnored, statusUntracked, statusUntracked},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := mergeStatusClass(test.left, test.right); got != test.want {
				t.Errorf("mergeStatusClass(%v, %v) = %v, want %v", test.left, test.right, got, test.want)
			}
		})
	}
}
