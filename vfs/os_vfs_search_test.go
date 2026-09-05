package vfs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOSVFSFindFilesOptions(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"alpha.txt":        "Needle here\n",
		"needles.txt":      "needles only\n",
		"notes.log":        "another file\n",
		"nested/deep.txt":  "needle below\n",
		"nested/empty.dat": "",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}

	v := NewOSVFS(root)
	ctx := context.Background()

	hits, err := v.FindFiles(ctx, root, FindQuery{
		Masks:      []string{"*.txt"},
		Text:       "NEEDLE",
		IgnoreCase: true,
	})
	if err != nil {
		t.Fatalf("basic search: %v", err)
	}
	if got := len(hits); got != 3 {
		t.Fatalf("basic search found %d files, want 3: %+v", got, hits)
	}

	hits, err = v.FindFiles(ctx, root, FindQuery{
		Masks:      []string{"*.txt"},
		Text:       "needle",
		IgnoreCase: true,
		WholeWords: true,
	})
	if err != nil {
		t.Fatalf("whole-word search: %v", err)
	}
	if got := len(hits); got != 2 {
		t.Fatalf("whole-word search found %d files, want 2: %+v", got, hits)
	}

	hits, err = v.FindFiles(ctx, root, FindQuery{
		Masks:      []string{"*.txt"},
		Text:       `needle\s+(here|below)`,
		Regex:      true,
		IgnoreCase: true,
	})
	if err != nil {
		t.Fatalf("regexp search: %v", err)
	}
	if got := len(hits); got != 2 {
		t.Fatalf("regexp search found %d files, want 2: %+v", got, hits)
	}

	hits, err = v.FindFiles(ctx, root, FindQuery{
		Masks:         []string{"*.txt"},
		Text:          "needle",
		IgnoreCase:    true,
		NotContaining: true,
	})
	if err != nil {
		t.Fatalf("negative search: %v", err)
	}
	if got := len(hits); got != 0 {
		t.Fatalf("negative search found %d files, want 0: %+v", got, hits)
	}

	hits, err = v.FindFiles(ctx, root, FindQuery{Masks: []string{"nested"}, FindFolders: true})
	if err != nil {
		t.Fatalf("folder search: %v", err)
	}
	if len(hits) != 1 || hits[0].Item.Name != "nested" || !hits[0].Item.IsDir {
		t.Fatalf("folder search returned %+v, want nested directory", hits)
	}

	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(filepath.Join(root, "alpha.txt"), link); err == nil {
		hits, err = v.FindFiles(ctx, root, FindQuery{Masks: []string{"link*"}})
		if err != nil {
			t.Fatalf("default symlink search: %v", err)
		}
		if len(hits) != 0 {
			t.Fatalf("default search followed symlink: %+v", hits)
		}
		hits, err = v.FindFiles(ctx, root, FindQuery{Masks: []string{"link*"}, FindSymlinks: true})
		if err != nil {
			t.Fatalf("symlink search: %v", err)
		}
		if len(hits) != 1 || !hits[0].Item.IsSymlink {
			t.Fatalf("symlink search returned %+v, want one symlink", hits)
		}
	}
}
