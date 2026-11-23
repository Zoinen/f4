# PDF files

[f4#1665](https://github.com/unxed/f4/issues/1665) introduced the PDF text view and read-only PDF panel.

## Qt gallery

F3, Enter, and double-click open the PDF one page at a time in the gallery viewer. The file's thumbnail shows its first page. Ctrl+Q opens the same viewer next to the file list. F3 expands that preview and Esc returns to it with the current page and zoom.

- Alt+Space or Alt+PgDn shows the next page.
- Alt+Backspace or Alt+PgUp shows the previous page.
- The viewer's file-navigation shortcuts still move between files.

The gallery uses Qt PDF's `QPdfDocument` for previews and renders the visible region at the current zoom. At 100%, the page size in PDF points is converted using the current screen's logical DPI and the window's device pixel ratio. The portable Qt build enables the Qt PDF module through Conan. Protected documents display a password-required message.

While browsing, the viewer prepares adjacent local PDFs at its physical pixel size. It keeps the current page visible until the next frame is ready, including the visible crop at manual zoom.

The same controls page through multipage TIFF, ICO/CUR variants, independent RAW captures, and primary images in HEIF/AVIF containers. Timed GIF, WebP, and HEIF/AVIF frames play automatically. Stepping manually pauses playback. The play button resumes it.

## Text and embedded pictures

Use the `File.PDFText` action for the page-by-page Markdown report. F3 opens this report in the terminal and with older Qt hosts.

Ctrl+PgDn opens a read-only tree with `text.md` (the whole text), `Pages/page-NNN.txt` (one text file per page), and `Images/pNNN-<name>.<ext>` (embedded raster pictures grouped by page). F3 on an image there opens it in the current frontend's image viewer.

The text comes from the fonts' ToUnicode maps or simple encodings.

### Text extraction limits

* Only text and embedded raster pictures are shown. The vector graphics of a page (drawings, charts made of lines) are not drawn, by design.
* A scanned PDF holds a picture per page and no text: `text.md` has nothing to show and the pictures are in `Images/`.
* Encrypted PDFs are refused ("the PDF is encrypted"); a text whose fonts have no usable encoding may come out incomplete.
