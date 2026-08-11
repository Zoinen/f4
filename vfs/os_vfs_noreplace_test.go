package vfs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOSVFSRenameNoReplace(t *testing.T) {
	dir := t.TempDir()
	v := NewOSVFS(dir)
	source := filepath.Join(dir, "source.txt")
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(source, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("target"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := v.RenameNoReplace(context.Background(), source, target); !errors.Is(err, ErrDestinationExists) {
		t.Fatalf("got %v, want ErrDestinationExists", err)
	}
	data, _ := os.ReadFile(target)
	if string(data) != "target" {
		t.Fatalf("destination was overwritten: %q", data)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source disappeared: %v", err)
	}

	free := filepath.Join(dir, "free.txt")
	if err := v.RenameNoReplace(context.Background(), source, free); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(free); string(data) != "source" {
		t.Fatalf("renamed data=%q", data)
	}
}

func TestOSVFSRenameHonorsNoOverwriteContext(t *testing.T) {
	dir := t.TempDir()
	filesystem := NewOSVFS(dir)
	source := filepath.Join(dir, "source.txt")
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(source, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("target"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := filesystem.Rename(WithDestinationOverwrite(context.Background(), false), source, target); !errors.Is(err, ErrDestinationExists) {
		t.Fatalf("Rename error = %v, want ErrDestinationExists", err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "target" {
		t.Fatalf("target changed to %q, %v", got, err)
	}
}

func TestOSVFSRenameNoReplaceDirectory(t *testing.T) {
	dir := t.TempDir()
	v := NewOSVFS(dir)
	source := filepath.Join(dir, "old")
	target := filepath.Join(dir, "new")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "child"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := v.RenameNoReplace(context.Background(), source, target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "child")); err != nil {
		t.Fatal(err)
	}
}
