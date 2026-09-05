package vfs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestPatchInPlace_RejectsWithoutWriting is about what a refusal leaves behind.
// In-place patching only works while every unchanged piece stays at the offset
// it came from; an insertion moves them and has to be refused. The refusal used
// to come after the pieces before it had already been written, so a patch that
// could not be applied still overwrote the start of the file — and the caller,
// falling back to a full rewrite, then read those damaged bytes back through
// its own buffer and saved them.
func TestPatchInPlace_RejectsWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	const original = "hello world\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}

	v := NewOSVFS(dir)
	// What the editor produces after typing "XYZ " at the start: new bytes
	// first, then the original content, now four bytes further along.
	pieces := []PatchPiece{
		{Data: []byte("XYZ "), Length: 4},
		{Offset: 0, Length: int64(len(original))},
	}

	if err := v.PatchInPlace(context.Background(), path, pieces); err == nil {
		t.Fatal("PatchInPlace accepted an insertion, which it cannot express")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Errorf("file = %q after a refused patch, want it untouched as %q", got, original)
	}
}

// TestPatchInPlace_AppliesSameLengthEdits keeps the accepting case working: an
// edit that leaves every unchanged piece where it was is exactly what in-place
// patching is for, and it must still write.
func TestPatchInPlace_AppliesSameLengthEdits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("hello world\n"), 0600); err != nil {
		t.Fatal(err)
	}

	v := NewOSVFS(dir)
	// "hello" replaced by "HELLO": same length, so " world\n" stays at offset 5.
	pieces := []PatchPiece{
		{Data: []byte("HELLO"), Length: 5},
		{Offset: 5, Length: 7},
	}

	if err := v.PatchInPlace(context.Background(), path, pieces); err != nil {
		t.Fatalf("PatchInPlace: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "HELLO world\n" {
		t.Errorf("file = %q, want %q", got, "HELLO world\n")
	}
}

func TestPatchInPlace_TruncatesShorterReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("aGVsbG8gd29ybGQ=\n"), 0600); err != nil {
		t.Fatal(err)
	}

	v := NewOSVFS(dir)
	pieces := []PatchPiece{{Data: []byte("hello world"), Length: 11}}
	if err := v.PatchInPlace(context.Background(), path, pieces); err != nil {
		t.Fatalf("PatchInPlace: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello world" {
		t.Errorf("file = %q, want %q", got, "hello world")
	}
}
