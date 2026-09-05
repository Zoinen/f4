package fishplus

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestHashAgainstLocalShell(t *testing.T) {
	c := newLocalShellClient(t)
	ctx := context.Background()
	if !c.CanHash() {
		t.Skip("this host cannot hash a tree remotely")
	}

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub dir"), 0700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"twin one.txt":     "the same content\n",
		"sub dir/twin two": "the same content\n",
		"different.txt":    "the same lengthX\n",
		"alone.txt":        "on its own\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}

	var progress int
	entries, err := c.Hash(ctx, root, func(p HashProgress) {
		if p.Total > 0 && p.Done <= p.Total {
			progress++
		}
	})
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	// Three files share a size, so three are hashed; the odd one out is
	// never read, which is the point of the two passes.
	if len(entries) != 3 {
		t.Fatalf("hashed %d files, want 3: %+v", len(entries), entries)
	}
	for _, e := range entries {
		if filepath.Base(e.Path) == "alone.txt" {
			t.Error("a file with a size of its own was hashed anyway")
		}
	}
	if progress == 0 {
		t.Error("the job reported no progress")
	}

	groups, err := c.Duplicates(ctx, root, nil)
	if err != nil {
		t.Fatalf("Duplicates: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("got %d duplicate groups, want 1: %v", len(groups), groups)
	}
	got := []string{filepath.Base(groups[0][0]), filepath.Base(groups[0][1])}
	sort.Strings(got)
	if got[0] != "twin one.txt" || got[1] != "twin two" {
		t.Errorf("the duplicate group is %v", got)
	}

	// Same size, different content: the sizes brought them together and the
	// hashes have to tell them apart again.
	if len(groups[0]) != 2 {
		t.Errorf("a file that only shares a size was called a duplicate: %v", groups[0])
	}

	if _, err := c.Hash(ctx, filepath.Join(root, "alone.txt"), nil); err == nil {
		t.Error("a regular file was accepted as a tree to hash")
	}
	if err := c.Session().Noop(ctx); err != nil {
		t.Fatalf("session out of sync after hashing: %v", err)
	}
}
