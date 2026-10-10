package multiarc

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

// provider_test.go covers CanOpen/Open's happy paths and the "no CLI tool
// on PATH" and "parent is not local" outcomes. It never cancels the
// context CanOpen is handed, and never points it at a path that exists but
// is not a regular file -- both real, and both silent "no" rather than an
// error, which is exactly the kind of branch a lite build depends on to
// just skip an archive it cannot safely open rather than crash the panel.

func TestProviderCanOpenRejectsCanceledContext(t *testing.T) {
	withFakeTools(t, func(string) (string, error) { return "/usr/bin/tar", nil }, nil)

	dir := t.TempDir()
	archivePath := filepath.Join(dir, "backup.tar.gz")
	if err := os.WriteFile(archivePath, []byte("stub"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	parent := vfs.NewOSVFS(dir)
	p := &Provider{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if p.CanOpen(ctx, parent, "backup.tar.gz") {
		t.Error("CanOpen should refuse an already-canceled context")
	}
}

func TestProviderCanOpenRejectsNonRegularFile(t *testing.T) {
	withFakeTools(t, func(string) (string, error) { return "/usr/bin/tar", nil }, nil)

	dir := t.TempDir()
	// A directory named like an archive: detectFormat claims the name, but
	// os.Stat's Mode().IsRegular() must still say no.
	archiveDir := filepath.Join(dir, "backup.tar.gz")
	if err := os.Mkdir(archiveDir, 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	parent := vfs.NewOSVFS(dir)
	p := &Provider{}

	if p.CanOpen(context.Background(), parent, "backup.tar.gz") {
		t.Error("CanOpen should refuse a directory even if its name matches an archive extension")
	}
}
