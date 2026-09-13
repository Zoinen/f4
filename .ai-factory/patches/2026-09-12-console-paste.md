# Commit command-line clipboard paste once

Qt already sent clipboard text in one message. The host emitted bracketed paste
events, but panel key routing dropped the markers. The editor consequently
inserted each character separately, with autocomplete and semantic rendering
repeated for every character.

Route markers and literal paste text to the owning command-line editor before
key-only dispatch. Preserve selection replacement and single-line newline
normalization. Reset history state once at commit. Retain FIFO/filter delivery
and bound frame-manager render batching to 100 ms for interrupted streams.

Regression-first evidence: 580 text changes before paste end; 6,002 semantic
renders for 6,000 characters. Fixed results: one text change and one render.
Regressions also cover subsequent typing, Unicode, selection replacement,
history reset, unchanged event order, and missing-end rendering recovery.
