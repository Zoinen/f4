package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/pdftext"
	"github.com/unxed/f4/vfs"
)

func TestIsPDFFile(t *testing.T) {
	for path, want := range map[string]bool{"a.pdf": true, "/x/B.PDF": true, "a.pdf.txt": false, "pdf": false, "": false} {
		if got := isPDFFile(path); got != want {
			t.Errorf("isPDFFile(%q) = %v", path, got)
		}
	}
}

func TestPDFReportRefusesAFileThatIsNotAPDF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.pdf")
	if err := os.WriteFile(path, []byte("not a pdf"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := pdfReport(context.Background(), vfs.NewOSVFS(dir), path, "notes.pdf"); !errors.Is(err, pdftext.ErrNotPDF) {
		t.Fatalf("err = %v, want ErrNotPDF", err)
	}
}
