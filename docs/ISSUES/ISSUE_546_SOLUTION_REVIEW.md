# Issue #546 solution review

## Problem model

The previous Unicode fix made editor grapheme navigation and painting cluster-aware, but the viewer still measured and emitted one terminal cell per decoded rune. Combining marks, Indic conjuncts, and Thaana marks were therefore counted repeatedly. A wrapped row could exceed its frame and overwrite a modal dialog or the scrollbar. Mouse coordinates in the editor also needed a final byte-boundary guard because a fallback visual-to-logical mapping could expose a code-point offset inside a cluster.

## Candidate solutions

1. **Chosen: one shared viewer row/cell path plus editor boundary snapping.** Use the existing `textlayout` cluster model for viewer wrapping, visual-order painting, and source-byte offsets. Reuse the same row calculation for Down and Ctrl+End. Snap mouse-derived editor offsets to the enclosing logical cluster boundary.
2. **Rejected: patch each affected dialog and scrollbar separately.** This would hide the symptom for the screenshots but leave every viewer overlay and future modal vulnerable to the same overflowing text row.
3. **Rejected: turn off BiDi/grapheme handling or only truncate `ScreenBuf.Write`.** Truncation prevents leaks but still leaves wrong wrapping, cursor/URL coordinates, and split glyphs. Disabling visual handling would regress the Unicode behavior fixed by the earlier PR.

## Review pass 1 — correctness

- Viewer wrapping and painting now use the same cluster boundaries, so a row cannot split a combining sequence or Indic conjunct.
- RTL clusters are emitted in terminal order while retaining logical byte offsets for URL hover and Ctrl-click.
- New tests cover Sanskrit wrapping, Thaana visual order, combining sequences, and editor mouse offsets inside clusters.
- All three viewer traversal paths were checked: initial render, keyboard Down fallback, and Ctrl+End tail indexing.

## Review pass 2 — compatibility

- Tabs retain their configured tab stops; CRLF consumes both bytes while CR is not painted.
- No-wrap mode still consumes through the real newline after the visible chunk, preserving existing large-line behavior.
- URL cell ranges continue to receive one source offset per terminal cell, including wide characters and tabs.
- The current upstream base also had a mapped-editor indexing guard and missing progress adapter that prevented the package from compiling or indexing mapped files; both are restored as small compatibility fixes required for a clean testable base.

## Review pass 3 — performance and safety

- Viewer tail indexing is bounded to a width-sized row window; it does not repeatedly segment the remaining 192 KiB tail for every wrapped row.
- Rendered cell slices are capped to the viewer width, preventing writes into dialogs, neighboring frames, or the scrollbar.
- The editor snap only reads the current logical line and does not alter the byte-addressable storage model.
- Existing full-file, mapped-file, and tail-only tests remain part of the verification set.

## Decision

The chosen approach is the smallest general fix that addresses the viewer screenshots, modal-frame corruption, and reverse-selection boundary report together without changing the underlying file offsets or disabling Unicode/BiDi support.
