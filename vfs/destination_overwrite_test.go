package vfs

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestDestinationOverwriteContext(t *testing.T) {
	t.Parallel()
	if _, known := DestinationOverwrite(context.Background()); known {
		t.Fatal("background context unexpectedly has an overwrite decision")
	}
	for _, overwrite := range []bool{false, true} {
		got, known := DestinationOverwrite(WithDestinationOverwrite(context.Background(), overwrite))
		if !known || got != overwrite {
			t.Fatalf("DestinationOverwrite(%v)=(%v,%v)", overwrite, got, known)
		}
	}
	if _, known := DestinationOverwrite(nil); known {
		t.Fatal("nil context unexpectedly has an overwrite decision")
	}
}

func TestOSVFSCreateHonorsExplicitNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}

	filesystem := NewOSVFS(dir)
	writer, err := filesystem.Create(WithDestinationOverwrite(context.Background(), false), path)
	if writer != nil {
		_ = writer.Close()
		t.Fatal("Create returned a writer for an occupied no-overwrite target")
	}
	if !errors.Is(err, os.ErrExist) {
		t.Fatalf("Create error = %v, want os.ErrExist", err)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "original" {
		t.Fatalf("occupied target was changed to %q", data)
	}

	newPath := filepath.Join(dir, "new.txt")
	writer, err = filesystem.Create(WithDestinationOverwrite(context.Background(), false), newPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(writer, "new"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
}
