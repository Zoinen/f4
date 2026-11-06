# Masonry first viewport offset, 2026-11-03

The fixed-input verifier rejected o165, o167 and o169 because their first
wheel sample started at logical y=14.286, or 25 physical pixels at 175%.
The first delivered input in these runs arrived after the first sparse
publication. In o166 and o168 it arrived before that publication and
recorded y=0. In o169 the render probe recorded y=14.286 before any wheel
event. The offset therefore predates wheel handling.

The o170 diagnostic records the write inside sparse Masonry rewrap. Before
input it restored row 0 at y=14.286 with a saved row offset of zero, moving
the viewport from y=0 to y=14.286. No explicit cursor offset was active.
The later dense rewrap moved the viewport back to zero. This identifies a
viewport anchor error during the publication sequence and explains why
input admission varied with publication timing.

The fixed-input gate still requires the first viewport. The o170 trace
establishes attribution and was rejected for timing acceptance.

The regression reproduced exactly 25 physical pixels at DPR 1.75. The fix
preserves the viewport origin when sparse rewrap uses an implicit leading
row anchor. Explicit cursor positioning still uses its requested offset.
The regression and adjacent sparse-layout checks passed.

The o171 capture disabled verbose media tracing and passed the first
viewport gate. Its first input arrived 201.077 ms after navigation at y=0.
PresentMon recorded 4,799 displayed intervals with zero dropped present
rows. The display interval p99 was 6.284 ms and the maximum was 25.006 ms.
One interval exceeded 9.375 ms, about 197 ms into the planned input window.
The recorder skipped 163 input slots and recorded 215 coverage-hold
samples. This confirms correct viewport entry and leaves both the display
outlier and motion coverage outside the acceptance limits.
