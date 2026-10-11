package pdftext

import (
	"fmt"
	"io"
	"strings"

	"github.com/unxed/f4/internal/i18n"
)

// Result is the text of a PDF, one string per page.
type Result struct {
	Pages []string
	// Lost counts characters that had no way to become text (a font with no
	// ToUnicode map and glyph-id codes); they appear as U+FFFD.
	Lost int
	// Truncated says the page or text limit cut the extraction short.
	Truncated bool
}

// load reads the whole file into memory and finds its objects.
func load(r io.ReaderAt, size int64) (*doc, error) {
	if size < 0 || size > MaxFileSize {
		return nil, fmt.Errorf("the PDF is larger than %d MiB", MaxFileSize>>20)
	}
	data := make([]byte, size)
	if _, err := r.ReadAt(data, 0); err != nil && err != io.EOF {
		return nil, err
	}
	return loadDoc(data)
}

// Extract reads the file of the given size and returns its text.
func Extract(r io.ReaderAt, size int64) (*Result, error) {
	d, err := load(r, size)
	if err != nil {
		return nil, err
	}
	res := &Result{}
	total := 0
	pages := d.pages()
	if len(pages) >= maxPages {
		res.Truncated = true
	}
	for _, page := range pages {
		text := d.pageText(page, &res.Lost)
		total += len(text)
		res.Pages = append(res.Pages, text)
		if total > maxTotalText {
			res.Truncated = true
			break
		}
	}
	return res, nil
}

// fenceFor returns a code fence that text cannot close.
func fenceFor(text string) string {
	fence := "```"
	for strings.Contains(text, fence) {
		fence += "`"
	}
	return fence
}

// Report renders the extracted text as Markdown, a page per section, in the
// interface language.
func Report(res *Result, file string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# `%s`\n\n", strings.ReplaceAll(file, "`", "'"))
	fmt.Fprintf(&b, "%s\n\n", fmt.Sprintf(i18n.Msg("PDF.Pages"), len(res.Pages)))
	if res.Lost > 0 {
		fmt.Fprintf(&b, "%s\n\n", fmt.Sprintf(i18n.Msg("PDF.Lost"), res.Lost))
	}
	if res.Truncated {
		fmt.Fprintf(&b, "%s\n\n", i18n.Msg("PDF.Truncated"))
	}
	for i, text := range res.Pages {
		fmt.Fprintf(&b, "## %s\n\n", fmt.Sprintf(i18n.Msg("PDF.Page"), i+1))
		if text == "" {
			fmt.Fprintf(&b, "%s\n\n", i18n.Msg("PDF.NoText"))
			continue
		}
		fence := fenceFor(text)
		fmt.Fprintf(&b, "%s\n%s\n%s\n\n", fence, text, fence)
	}
	return b.String()
}
