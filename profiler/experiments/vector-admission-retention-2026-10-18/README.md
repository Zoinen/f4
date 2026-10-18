# Admission plus single-element unmerged retention

This is an isolated experiment, not the portable renderer recipe.

`qt-admission-retention.patch` is self-contained against Qt 6.11.1 after the
integrated stable-interval recipe. It includes the optional v2 upload protocol,
registered transform-root interval ownership, and the earlier single-element
unmerged retention experiment. Apply this patch instead of the other admission
patches, not on top of them. Apply-check passes against LF-normalized bases.

The combination is motivated by new traces: already-ready vector geometry
bypasses initial admission and still uploads during unrelated rebuilds.
Permanently unmergeable single-element batches can keep their original raw
buffers across order changes. No buffer is handed to another batch; dirty
geometry/root/partition changes and merged geometry still invalidate normally.

The native fixture now restores the earlier full-order-change rows, combined
with the new admission and 16-cycle root-lifetime checks. The two unmerged
order-change rows fail against admission alone and pass on the combination.
All 16 results pass on default Windows RHI and OpenGL at DPR 1.75. The focused
gallery SVG selection passes 26 results.

Clean scrolling has one tied pair and one improved long-interval count; p99
remains approximately 12.5 ms. This does not establish constant 160 Hz or a
reason to enable the patch in the release recipe yet.

[Detailed evidence](../../reports/vector-admission-attribution-2026-10-18.md)
