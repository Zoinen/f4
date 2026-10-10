# PDF files (`plugins/pdfview`)

[f4#1665](https://github.com/unxed/f4/issues/1665): PDF documents as text and pictures, in the terminal.

* **F3 on a `.pdf`** opens the formatted viewer with the text of the document, page after page, as Markdown.
* **Ctrl+PgDn on a `.pdf`** opens it as a read-only tree in a file panel: `text.md` (the whole text, the same as the F3 view), `Pages/page-NNN.txt` (one text file per page) and `Images/pNNN-<name>.<ext>` (the pictures embedded in the document, named by the page they sit on). F3 on a picture in `Images/` opens f4's image viewer, so it is shown in the terminal by whatever graphics protocol the terminal supports (sixel, kitty and the like).

The document is only read; nothing in it is executed. The text comes from the fonts' ToUnicode maps or simple encodings.

## Limits

* Only text and embedded raster pictures are shown. The vector graphics of a page (drawings, charts made of lines) are not drawn, by design.
* A scanned PDF holds a picture per page and no text: `text.md` has nothing to show and the pictures are in `Images/`.
* Encrypted PDFs are refused ("the PDF is encrypted"); a text whose fonts have no usable encoding may come out incomplete.
