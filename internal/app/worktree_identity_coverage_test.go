package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorktreeBranchAtRejectsMalformedAndDetachedCheckouts(t *testing.T) {
	dir := t.TempDir()
	if got := worktreeBranchAt(dir); got != "" {
		t.Fatalf("missing .git file resolved to %q", got)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("not a gitdir link\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := worktreeBranchAt(dir); got != "" {
		t.Fatalf("malformed .git file resolved to %q", got)
	}

	gitdir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: "+filepath.Base(gitdir)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := worktreeBranchAt(dir); got != "" {
		t.Fatalf("missing relative gitdir resolved to %q", got)
	}

	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: "+gitdir+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitdir, "HEAD"), []byte("detached\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := worktreeBranchAt(dir); got != "" {
		t.Fatalf("detached HEAD resolved to %q", got)
	}
	if err := os.WriteFile(filepath.Join(gitdir, "HEAD"), []byte("ref: refs/tags/release\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := worktreeBranchAt(dir); got != "" {
		t.Fatalf("non-branch ref resolved to %q", got)
	}
}
