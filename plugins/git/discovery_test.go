package gitplugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRepositoryDiscoveryFindsCurrentGitDirectoryAndCachesBranch(t *testing.T) {
	root := t.TempDir()
	gitDir := writeGitDirectory(t, root, "ref: refs/heads/main\n")
	discovery := NewRepositoryDiscovery(DiscoveryOptions{UpwardDelay: 0})
	t.Cleanup(discovery.Close)

	update := observeAndWait(t, discovery, "left", root)
	if update.Err != nil {
		t.Fatalf("discovery error: %v", update.Err)
	}
	if !update.Result.Found() {
		t.Fatalf("result = %#v, want repository", update.Result)
	}
	if got, want := update.Result.Repository.Root, absolutePath(t, root); got != want {
		t.Errorf("repository root = %q, want %q", got, want)
	}
	if got, want := update.Result.Repository.GitDir, absolutePath(t, gitDir); got != want {
		t.Errorf("git dir = %q, want %q", got, want)
	}
	if got := update.Result.Repository.Branch.Prompt(); got != "main" {
		t.Errorf("branch prompt = %q, want main", got)
	}

	// Cached lookups must not touch the filesystem. Removing the marker after
	// the completed observation therefore cannot change the returned result.
	if err := os.RemoveAll(filepath.Join(root, ".git")); err != nil {
		t.Fatal(err)
	}
	if result := discovery.Lookup(root); !result.Found() || result.Repository.Branch.Prompt() != "main" {
		t.Fatalf("cached result after marker removal = %#v, want cached main repository", result)
	}
	if branch, ok := discovery.CachedBranch(root); !ok || branch.Prompt() != "main" {
		t.Fatalf("cached branch = %#v, %t; want main, true", branch, ok)
	}

	// Entering the same directory starts a fresh worker probe and replaces a
	// stale positive answer when the on-disk marker has changed.
	refreshed := observeAndWait(t, discovery, "left", root)
	if refreshed.Err != nil || refreshed.Result.State != LookupNotRepository {
		t.Fatalf("refreshed result = %#v, want not repository", refreshed)
	}
}

func TestRepositoryDiscoveryWalksUpToGitdirFileAndCachesTraversedPaths(t *testing.T) {
	base := t.TempDir()
	worktree := filepath.Join(base, "worktree")
	start := filepath.Join(worktree, "source", "nested")
	gitDir := filepath.Join(base, "administrative", "worktrees", "one")
	commonDir := filepath.Join(base, "administrative")
	if err := os.MkdirAll(start, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(commonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/feature/git-panel\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "commondir"), []byte("../..\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	relativeGitDir, err := filepath.Rel(worktree, gitDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: "+relativeGitDir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	discovery := NewRepositoryDiscovery(DiscoveryOptions{UpwardDelay: 0})
	t.Cleanup(discovery.Close)
	update := observeAndWait(t, discovery, "left", start)
	if update.Err != nil {
		t.Fatalf("discovery error: %v", update.Err)
	}
	repository := update.Result.Repository
	if !update.Result.Found() {
		t.Fatalf("result = %#v, want repository", update.Result)
	}
	if got, want := repository.Root, absolutePath(t, worktree); got != want {
		t.Errorf("root = %q, want %q", got, want)
	}
	if got, want := repository.GitDir, absolutePath(t, gitDir); got != want {
		t.Errorf("git dir = %q, want %q", got, want)
	}
	if got, want := repository.CommonDir, absolutePath(t, commonDir); got != want {
		t.Errorf("common dir = %q, want %q", got, want)
	}
	if got := repository.Branch.Prompt(); got != "feature/git-panel" {
		t.Errorf("branch prompt = %q, want feature/git-panel", got)
	}
	for _, path := range []string{start, filepath.Dir(start), worktree} {
		result := discovery.Lookup(path)
		if !result.Found() || result.Repository.Root != repository.Root {
			t.Errorf("cache at %q = %#v, want repository rooted at %q", path, result, repository.Root)
		}
	}
}

func TestRepositoryDiscoveryCachesNegativeWalkAndDetachedHead(t *testing.T) {
	noRepository := filepath.Join(t.TempDir(), "one", "two")
	if err := os.MkdirAll(noRepository, 0o755); err != nil {
		t.Fatal(err)
	}
	discovery := NewRepositoryDiscovery(DiscoveryOptions{UpwardDelay: 0})
	t.Cleanup(discovery.Close)
	update := observeAndWait(t, discovery, "left", noRepository)
	if update.Err != nil {
		t.Fatalf("discovery error: %v", update.Err)
	}
	if update.Result.State != LookupNotRepository {
		t.Fatalf("result state = %v, want not repository", update.Result.State)
	}
	for _, path := range []string{noRepository, filepath.Dir(noRepository)} {
		if result := discovery.Lookup(path); result.State != LookupNotRepository {
			t.Errorf("cache at %q state = %v, want not repository", path, result.State)
		}
	}

	root := t.TempDir()
	writeGitDirectory(t, root, "abcdef1234567890abcdef1234567890abcdef12\n")
	update = observeAndWait(t, discovery, "right", root)
	if update.Err != nil || !update.Result.Found() {
		t.Fatalf("detached repository update = %#v", update)
	}
	branch, ok := discovery.CachedBranch(root)
	if !ok || !branch.Detached || branch.Prompt() != "@abcdef1" {
		t.Fatalf("detached branch = %#v, %t; want @abcdef1", branch, ok)
	}
}

func TestRepositoryDiscoveryCachesUnbornSymbolicHead(t *testing.T) {
	root := t.TempDir()
	gitDir := writeGitDirectory(t, root, "ref: refs/heads/trunk\n")
	discovery := NewRepositoryDiscovery(DiscoveryOptions{UpwardDelay: 0})
	t.Cleanup(discovery.Close)

	update := observeAndWait(t, discovery, "panel", root)
	if update.Err != nil || !update.Result.Found() {
		t.Fatalf("unborn repository update = %#v", update)
	}
	branch := update.Result.Repository.Branch
	if !branch.Unborn || branch.Detached || branch.Prompt() != "trunk" {
		t.Fatalf("unborn branch = %#v, want trunk symbolic unborn", branch)
	}

	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "refs", "heads", "trunk"), []byte("abcdef1234567890abcdef1234567890abcdef12\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	update = observeAndWait(t, discovery, "panel", root)
	if update.Err != nil || update.Result.Repository.Branch.Unborn {
		t.Fatalf("resolved symbolic branch update = %#v, want non-unborn trunk", update)
	}
}

func TestRepositoryDiscoverySupersedesOlderGeneration(t *testing.T) {
	base := t.TempDir()
	nested := filepath.Join(base, "nested", "deep")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(base, "second")
	if err := os.MkdirAll(second, 0o755); err != nil {
		t.Fatal(err)
	}
	writeGitDirectory(t, second, "ref: refs/heads/second\n")

	discovery := NewRepositoryDiscovery(DiscoveryOptions{UpwardDelay: 150 * time.Millisecond})
	t.Cleanup(discovery.Close)
	updates := make(chan DiscoveryUpdate, 4)
	firstGeneration, err := discovery.Observe(context.Background(), "panel", nested, func(update DiscoveryUpdate) { updates <- update })
	if err != nil {
		t.Fatal(err)
	}
	secondGeneration, err := discovery.Observe(context.Background(), "panel", second, func(update DiscoveryUpdate) { updates <- update })
	if err != nil {
		t.Fatal(err)
	}
	if secondGeneration <= firstGeneration {
		t.Fatalf("generations = %d then %d, want increase", firstGeneration, secondGeneration)
	}

	select {
	case update := <-updates:
		if update.Generation != secondGeneration {
			t.Fatalf("received superseded update %#v", update)
		}
		if update.Err != nil || !update.Result.Found() || update.Result.Repository.Branch.Prompt() != "second" {
			t.Fatalf("latest update = %#v, want second repository", update)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for latest observation")
	}

	select {
	case update := <-updates:
		t.Fatalf("received unexpected stale update %#v", update)
	case <-time.After(250 * time.Millisecond):
	}
}

func TestRepositoryDiscoveryDelaysOnlyUpwardSearch(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	writeGitDirectory(t, root, "ref: refs/heads/main\n")
	discovery := NewRepositoryDiscovery(DiscoveryOptions{UpwardDelay: 80 * time.Millisecond})
	t.Cleanup(discovery.Close)
	updates := make(chan DiscoveryUpdate, 1)
	started := time.Now()
	if _, err := discovery.Observe(context.Background(), "left", nested, func(update DiscoveryUpdate) { updates <- update }); err != nil {
		t.Fatal(err)
	}
	select {
	case update := <-updates:
		t.Fatalf("upward discovery completed before delay: %#v", update)
	case <-time.After(25 * time.Millisecond):
	}
	select {
	case update := <-updates:
		if update.Err != nil || !update.Result.Found() {
			t.Fatalf("update = %#v, want repository", update)
		}
		if elapsed := time.Since(started); elapsed < 60*time.Millisecond {
			t.Fatalf("upward discovery elapsed %s, want approximately configured delay", elapsed)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for delayed upward discovery")
	}
}

func TestRepositoryDiscoveryDelaysBranchPresentationAfterImmediateLocalHit(t *testing.T) {
	root := t.TempDir()
	writeGitDirectory(t, root, "ref: refs/heads/main\n")
	discovery := NewRepositoryDiscovery(DiscoveryOptions{UpwardDelay: 0, BranchDelay: 80 * time.Millisecond})
	t.Cleanup(discovery.Close)
	updates := make(chan DiscoveryUpdate, 2)
	started := time.Now()
	if _, err := discovery.Observe(context.Background(), "left", root, func(update DiscoveryUpdate) { updates <- update }); err != nil {
		t.Fatal(err)
	}

	select {
	case update := <-updates:
		if update.Err != nil || !update.Result.Found() {
			t.Fatalf("immediate result = %#v, want repository", update)
		}
		if update.Result.Repository.Branch.Name != "" {
			t.Fatalf("immediate branch = %#v, want delayed prompt state", update.Result.Repository.Branch)
		}
		if elapsed := time.Since(started); elapsed > 60*time.Millisecond {
			t.Fatalf("local repository result took %s, want immediate worker result", elapsed)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for immediate repository result")
	}

	select {
	case update := <-updates:
		if update.Err != nil || update.Result.Repository.Branch.Prompt() != "main" {
			t.Fatalf("delayed branch result = %#v, want main", update)
		}
		if elapsed := time.Since(started); elapsed < 60*time.Millisecond {
			t.Fatalf("branch result took %s, want configured delay", elapsed)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for delayed branch result")
	}
}

func observeAndWait(t *testing.T, discovery *RepositoryDiscovery, observerID, directory string) DiscoveryUpdate {
	t.Helper()
	updates := make(chan DiscoveryUpdate, 1)
	generation, err := discovery.Observe(context.Background(), observerID, directory, func(update DiscoveryUpdate) { updates <- update })
	if err != nil {
		t.Fatal(err)
	}
	select {
	case update := <-updates:
		if update.Generation != generation {
			t.Fatalf("update generation = %d, want %d", update.Generation, generation)
		}
		return update
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for discovery of %q", directory)
		return DiscoveryUpdate{}
	}
}

func writeGitDirectory(t *testing.T, root, head string) string {
	t.Helper()
	gitDir := filepath.Join(root, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte(head), 0o644); err != nil {
		t.Fatal(err)
	}
	return gitDir
}

func absolutePath(t *testing.T, path string) string {
	t.Helper()
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(absolute)
}
