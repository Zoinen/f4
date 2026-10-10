package git

import (
	"path/filepath"
	"testing"

	"github.com/unxed/vtinput"
)

func untrackedDirRepo(t *testing.T) string {
	t.Helper()
	repo := realGitRepo(t)
	writeRepoFile(t, filepath.Join(repo, "keep.txt"), "keep\n")
	runRealGit(t, repo, "add", "-A")
	runRealGit(t, repo, "commit", "-q", "-m", "initial")
	writeRepoFile(t, filepath.Join(repo, "d", "a.txt"), numbered(1, 4))
	writeRepoFile(t, filepath.Join(repo, "d", "b.txt"), "b\n")
	return repo
}

// TestF4OnUntrackedDirectoryListsItsFiles: F4 on "d/" turns the row into the
// files inside it, cursor on the first; F4 on a file then picks its lines.
func TestF4OnUntrackedDirectoryListsItsFiles(t *testing.T) {
	repo := untrackedDirRepo(t)
	p := openStatusPanelIn(t, repo)
	if e, ok := p.selectedEntry(); !ok || e.Path != "d/" {
		t.Fatalf("entry under the cursor = %+v, want the untracked d/", e)
	}
	p.showHunks()
	e, ok := p.selectedEntry()
	if !ok || e.Path != "d/a.txt" || e.XY != "??" {
		t.Fatalf("after F4 the cursor is on %+v, want ?? d/a.txt", e)
	}
	var paths []string
	for _, r := range p.table.Rows {
		paths = append(paths, r.(statusRow).entry.Path)
	}
	if len(paths) != 2 || paths[0] != "d/a.txt" || paths[1] != "d/b.txt" {
		t.Fatalf("rows = %v, want d/a.txt and d/b.txt", paths)
	}
	for _, path := range paths {
		if path == "d/" {
			t.Fatalf("the directory row is still there: %v", paths)
		}
	}

	v := openHunksOf(t, p, modeStage)
	pickRows(t, v, 2, 3) // "+2", "+3"
	v.ProcessKey(key(vtinput.VK_RETURN))
	if !v.IsDone() {
		t.Fatal("the view did not close after staging")
	}
	if got, want := runRealGit(t, repo, "show", ":d/a.txt"), "2\n3\n"; got != want {
		t.Errorf("index d/a.txt = %q, want %q", got, want)
	}
	if got := runRealGit(t, repo, "ls-files", "--stage", "d/b.txt"); got != "" {
		t.Errorf("d/b.txt was staged as well: %q", got)
	}
}

func TestUntrackedFilesOfMissingDirectory(t *testing.T) {
	repo := untrackedDirRepo(t)
	files, err := untrackedFiles(t.Context(), repo, "nope/")
	if err != nil || len(files) != 0 {
		t.Errorf("untrackedFiles(nope/) = %v, %v; want none", files, err)
	}
	p := openStatusPanelIn(t, repo)
	p.expanded = map[string]bool{"d/": true}
	entries := p.expandUntrackedDirs([]statusEntry{{XY: "??", Path: "d/"}, {XY: "??", Path: "z"}})
	if len(entries) != 3 || entries[0].Path != "d/a.txt" || entries[2].Path != "z" {
		t.Errorf("expandUntrackedDirs = %+v", entries)
	}
	p.expanded = map[string]bool{"gone/": true}
	if got := p.expandUntrackedDirs([]statusEntry{{XY: "??", Path: "gone/"}}); len(got) != 1 || got[0].Path != "gone/" {
		t.Errorf("a directory with no files must stay one entry: %+v", got)
	}
}
