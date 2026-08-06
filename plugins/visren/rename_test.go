package visren

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestExecuteRenamesPartialAndNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	fs := vfs.NewOSVFS(dir)
	for name, data := range map[string]string{"a.txt": "a", "b.txt": "b", "occupied.txt": "keep"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	rows := []Preview{
		{Item: testItem("a.txt"), Destination: "one.txt"},
		{Item: testItem("b.txt"), Destination: "occupied.txt"},
	}
	result := ExecuteRenames(context.Background(), fs, dir, rows, func(_, _ string, err error) ErrorAction {
		if !errors.Is(err, vfs.ErrDestinationExists) {
			t.Errorf("unexpected error: %v", err)
		}
		return Skip
	})
	if len(result.Succeeded) != 1 || result.Succeeded[0] != (RenamePair{Old: "a.txt", New: "one.txt"}) {
		t.Fatalf("succeeded=%v", result.Succeeded)
	}
	if len(result.Pending) != 1 || result.Pending[0] != "b.txt" {
		t.Fatalf("pending=%v", result.Pending)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "occupied.txt")); string(data) != "keep" {
		t.Fatalf("occupied target overwritten: %q", data)
	}
}

func TestExecuteRenamesCancelKeepsRemainder(t *testing.T) {
	dir := t.TempDir()
	fs := vfs.NewOSVFS(dir)
	for _, name := range []string{"a", "b", "taken"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	rows := []Preview{{Item: testItem("a"), Destination: "taken"}, {Item: testItem("b"), Destination: "c"}}
	result := ExecuteRenames(context.Background(), fs, dir, rows, func(_, _ string, _ error) ErrorAction { return Cancel })
	if !result.Canceled || len(result.Pending) != 2 || result.Pending[0] != "a" || result.Pending[1] != "b" {
		t.Fatalf("result=%+v", result)
	}
}

func TestExecuteUndoReverseOrder(t *testing.T) {
	dir := t.TempDir()
	fs := vfs.NewOSVFS(dir)
	if err := os.WriteFile(filepath.Join(dir, "c"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Reverse order makes c->b and then b->a possible.
	log := []RenamePair{{Old: "a", New: "b"}, {Old: "b", New: "c"}}
	result := ExecuteUndo(context.Background(), fs, dir, log, nil)
	if len(result.Succeeded) != 2 {
		t.Fatalf("result=%+v", result)
	}
	if _, err := os.Stat(filepath.Join(dir, "a")); err != nil {
		t.Fatal(err)
	}
}
