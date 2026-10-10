package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/pdftext"
	"github.com/unxed/f4/internal/toast"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// pdfFile lets a vfs file serve as the io.ReaderAt the PDF reader wants.
type pdfFile struct {
	ctx  context.Context
	file vfs.ReadAtCloser
}

func (r pdfFile) ReadAt(p []byte, off int64) (int, error) {
	return r.file.ReadAt(r.ctx, p, off)
}

// pdfReport reads the text of the PDF at path on v, page by page, and renders
// it as Markdown.
func pdfReport(ctx context.Context, v vfs.VFS, path, name string) (string, error) {
	file, err := v.Open(ctx, path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	res, err := pdftext.Extract(pdfFile{ctx: ctx, file: file}, file.Size())
	if err != nil {
		return "", err
	}
	return pdftext.Report(res, name), nil
}

// actionPDFText shows the text of the PDF under the cursor in the Markdown
// viewer (f4#1665). Nothing in the file is run; a file the reader cannot
// handle (encrypted, damaged) is reported in a toast.
func actionPDFText(pf *panel.PanelsFrame) {
	fsp := pf.GetActivePanel()
	if fsp == nil {
		return
	}
	idx := fsp.GetCursorIndex()
	if idx < 0 || idx >= len(fsp.Entries) || fsp.Entries[idx].IsDir {
		return
	}
	name := fsp.GetSelectedName()
	path := fsp.Vfs.Join(fsp.Vfs.GetPath(), name)
	text, err := pdfReport(context.Background(), fsp.Vfs, path, name)
	switch {
	case errors.Is(err, pdftext.ErrNotPDF):
		toast.Show(fmt.Sprintf(i18n.Msg("PDF.NotPDF"), name), 3*time.Second)
	case errors.Is(err, pdftext.ErrEncrypted):
		toast.Show(fmt.Sprintf(i18n.Msg("PDF.Encrypted"), name), 3*time.Second)
	case err != nil:
		toast.Show(fmt.Sprintf(i18n.Msg("PDF.ReadFailed"), name, err), 3*time.Second)
	default:
		vfs.ShowPanelHelp(name, text)
		vtui.FrameManager.Redraw()
	}
}

// isPDFFile says whether a name is one F3 opens as formatted text.
func isPDFFile(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".pdf")
}

// tryOpenPDFViewer is F3 on a PDF (f4#1665): its text, page by page, in the
// same formatted viewer a Markdown file gets, F4 there going to the plain
// viewer. A file the reader cannot handle (encrypted, damaged, unreadable)
// opens in the ordinary viewer, which has its own answers for those.
func tryOpenPDFViewer(pf *panel.PanelsFrame, v vfs.VFS, path string) bool {
	if pf == nil || v == nil || !isPDFFile(path) {
		return false
	}
	var report string
	pf.RunProgressTaskAfter(openingProgressDelay, " Opening... ", "Preparing to open file...", false, func(ctx context.Context, update func(msg string, percent int)) error {
		update("Reading the PDF...", -1)
		var err error
		report, err = pdfReport(ctx, v, path, v.Base(path))
		return err
	}, func(err error) {
		if errors.Is(err, context.Canceled) {
			return
		}
		if err != nil {
			vtui.DebugLog("PDF: %s opens as plain: %v", path, err)
			openPlainViewer(pf, v, path, false)
			return
		}
		showMarkdownView(pf, v, path, []byte(report))
	})
	return true
}
