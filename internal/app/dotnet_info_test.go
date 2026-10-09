package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/dotnet"
	"github.com/unxed/f4/vfs"
)

func TestDotnetReportRefusesAFileThatIsNotAnAssembly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(path, []byte("plain text"), 0o600); err != nil {
		t.Fatal(err)
	}
	v := vfs.NewOSVFS(dir)
	if _, err := dotnetReport(context.Background(), v, path, "notes.txt"); !errors.Is(err, dotnet.ErrNotAssembly) {
		t.Fatalf("err = %v, want ErrNotAssembly", err)
	}
	if _, err := dotnetReport(context.Background(), v, filepath.Join(dir, "missing"), "missing"); err == nil {
		t.Fatal("a missing file was reported on")
	}
}
