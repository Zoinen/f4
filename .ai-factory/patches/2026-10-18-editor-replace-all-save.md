# Preserve piece-table backing during whole-buffer edits

Replace All followed by F2 reproduced on unxed/main 14d22ef86: the editor
displayed the replacement but saved old disk bytes. SetText created a new
Original buffer; optimized saves interpreted it as unchanged file ranges.
Undo snapshots also retained offsets into the discarded backing buffers.

Keep the existing piece table and replace its pieces through Delete/Insert.
Replacement bytes then live in Add, and existing undo/redo backing survives.
Regression coverage exercises actual Replace All, undo/redo, save and reload
with mapped and chunked files, plus repeated SetText and empty replacements.

This local learning note is intentionally excluded from the upstream PR.
