# Short terminal output must not scroll into grid padding

The backend's full active-grid extent included blank bottom-alignment padding.
Qt compared it with the wire viewport span, showing an almost-full scrollbar
even when the actual output fit. Wheel input moved into that artificial range
and the next tail update snapped it back.

Add an absolute contentStart before real history exists; preserve absolute row
and selection coordinates, all stored history, and alternate-screen contents.
Clamp backend scroll requests to real content. Qt derives overflow and scrollbar
fractions from the content range and physical viewport, clamps local scrolling,
and consumes wheel input without moving or requesting a window when output fits.

Regression-first: Qt failed on the visible scrollbar for ten output lines in a
forty-row grid; the Go 24-grid/20-viewport fixture accepted blank-padding scroll.
Both now pass. Tests cover resizing to genuine overflow, unchanged wheel/tail
state, all actual text leaves' pixel origins and unit transforms at DPR 1.75,
and a rendered capture. Full document suite: 63 passed, 3 opt-in skipped.
Further coverage can exercise short-output resizing between mixed-DPI monitors.
