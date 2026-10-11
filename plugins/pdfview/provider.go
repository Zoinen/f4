package pdfview

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/unxed/f4/internal/pdftext"
	"github.com/unxed/f4/vfs"
)

// provider mounts a PDF on an explicit Ctrl+PgDn only: Enter and F3 keep
// their own meaning (F3 shows the text in the formatted viewer).
type provider struct{}

func (*provider) Name() string  { return "pdfview" }
func (*provider) Priority() int { return 15 }

// PanelEnterAllowed keeps Enter and double-click for opening the file.
func (*provider) PanelEnterAllowed(context.Context, vfs.VFS, string) bool { return false }

// localPDF answers for a regular file on the local disk named like a PDF.
func localPDF(ctx context.Context, parent vfs.VFS, path string) (string, bool) {
	if ctx != nil && ctx.Err() != nil {
		return "", false
	}
	local, ok := parent.(*vfs.OSVFS)
	if !ok || local == nil || !strings.EqualFold(filepath.Ext(path), ".pdf") {
		return "", false
	}
	abs, err := local.Abs(path)
	if err != nil {
		return "", false
	}
	info, err := os.Stat(abs)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	return abs, true
}

func (p *provider) CanOpen(ctx context.Context, parent vfs.VFS, path string) bool {
	abs, ok := localPDF(ctx, parent, path)
	if !ok {
		return false
	}
	f, err := os.Open(abs) //nolint:gosec // the path is the file the user chose in the panel
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, 1024)
	n, _ := f.Read(head)
	return strings.Contains(string(head[:n]), "%PDF-")
}

func (p *provider) Open(ctx context.Context, parent vfs.VFS, path string) (vfs.VFS, error) {
	abs, ok := localPDF(ctx, parent, path)
	if !ok {
		return nil, fmt.Errorf("%s: %w", path, pdftext.ErrNotPDF)
	}
	f, err := os.Open(abs) //nolint:gosec // the path is the file the user chose in the panel
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	res, err := pdftext.Extract(f, st.Size())
	if err != nil {
		return nil, err
	}
	// Pictures are a bonus: a file whose pictures cannot be read still opens.
	images, _ := pdftext.ExtractImages(f, st.Size())
	return newDocumentVFS(parent, filepath.Base(abs), res, images), nil
}
