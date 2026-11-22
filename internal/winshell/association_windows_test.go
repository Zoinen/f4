package winshell

import (
	"errors"
	"path/filepath"
	"syscall"
	"testing"
)

func TestOpenAssociatedPathReportsMissingFile(t *testing.T) {
	dir := t.TempDir()
	err := OpenAssociatedPath(filepath.Join(dir, "missing executable.exe"), dir)
	if !errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		t.Fatalf("OpenAssociatedPath error = %v, want ERROR_FILE_NOT_FOUND", err)
	}
}
