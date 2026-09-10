# Issue 546 follow-up: solution review

## Observed failure modes

The supplied text files and screenshots show three related failures:

1. Devanagari and Thaana clusters can be split at a virama or a terminal
   mark, which makes dialogs and scrollbars disagree about the number of
   cells occupied by the text.
2. Shift-selection can begin inside a shaped Indic cluster in either
   direction.
3. A mixed left-to-right/right-to-left line can lose the relationship between
   its visual clusters and logical rune offsets.

The editor already had a local Indic-cluster extension, but its bidi path and
the `vtui` dialog widgets still used the older segmentation independently.

## Candidate solutions

### A — selected: one terminal cluster model at every boundary

Keep UAX grapheme segmentation as the base, then join an Indic virama with a
following consonant for terminal editing and bidi mapping. Use that same model
for `vtui.Edit`, `MultiLineEdit`, highlighted text, and caret maps. Keep the
existing deletion-only rule for a final Indic/Thaana mark separate from cursor
and Shift-selection movement.

This fixes the common cause instead of adding a per-dialog exception. It also
preserves the existing public cell-width path, while the editing path uses the
same combined cluster width as `f4`.

### B — rejected: byte/rune-only cursor corrections

Moving the cursor by bytes or individual runes would avoid one observed split,
but would still disagree with bidi visual order and would break combining
marks, emoji sequences, and other scripts. It would also leave dialog drawing
on the old cluster boundaries.

### C — rejected: dialog-size or scrollbar padding hacks

Adding columns or changing a particular dialog's width could hide one
screenshot artifact, but it would not repair selection, bidi offsets, or other
dialogs. It would also make layout depend on the specific sample string.

## Three-pass review

### Pass 1 — correctness

The common `vtui` path now preserves the original logical rune positions while
making Indic conjuncts atomic for editing and bidi. F4's editor keeps the
terminal-compatible trailing-mark deletion rule only for Backspace; cursor and
Shift-selection use complete clusters. RTL deletion follows logical text
direction rather than treating the visual screen edge as an end-of-text no-op.

### Pass 2 — regression and performance

The existing UAX path and its width tests remain unchanged. The extra walk is
used only for text that already requires full grapheme segmentation, and the
ASCII fast path is untouched. Tests cover Devanagari, Thaana, mixed bidi text,
selection, deletion, visual offsets, and rendered-cell preservation.

### Pass 3 — portability

The implementation uses Go Unicode tables and `golang.org/x/text/unicode/bidi`
only; it does not depend on a particular font, window system, or terminal
backend. Verification is being performed on Linux ARM64 with XWayland/X11,
with the native Wayland startup panic tracked separately as an environment
limitation.

## Verification record

- Focused F4 editor, text-layout, and viewer tests pass.
- The new F4 mixed-bidi cluster-preservation and Shift-selection tests pass.
- The new `vtui` terminal-cluster and bidi-map tests pass.
- A native ARM64 build and X11 smoke test remain required before opening the
  pull request.
