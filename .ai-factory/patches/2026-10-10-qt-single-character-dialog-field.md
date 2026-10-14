# Single-character dialog fields in Qt

Console cell widths cannot be used unchanged as native text-field widths: a
one-cell edit loses its entire text viewport to the native field's padding.
The plugin hotkey assignment dialog exposed this with a clipped `A`.

SemanticDialogLayout now supplies a glyph-aware minimum presentation width for
one-cell edits and shifts following same-row widgets by the extra width. Keep
the semantic edit width and keyboard behavior unchanged; ordinary edits retain
their existing dimensions.

Regression: singleCharacterDialogFieldKeepsGlyphAndLabelVisible fails before
the fix and passes afterward at DPR 1.75, checking both glyph room and label gap.
