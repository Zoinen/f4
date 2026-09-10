package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorktreeIdentity(t *testing.T) {
	checkout := t.TempDir()
	gitdir := t.TempDir()
	_ = os.WriteFile(filepath.Join(checkout, ".git"), []byte("gitdir: "+gitdir), 0600)
	_ = os.WriteFile(filepath.Join(gitdir, "HEAD"), []byte("ref: refs/heads/zoin_branch/one\n"), 0600)
	if worktreeBranchAt(checkout) != "zoin_branch/one" {
		t.Fatal("branch resolution")
	}
	_ = os.WriteFile(filepath.Join(gitdir, "HEAD"), []byte("ref: refs/heads/zoin_branch/two\n"), 0600)
	if !strings.HasSuffix(worktreeBranchAt(checkout), "two") {
		t.Fatal("branch was cached")
	}
	if worktreeBranchAt(t.TempDir()) != "" {
		t.Fatal("main checkout fallback")
	}
}
