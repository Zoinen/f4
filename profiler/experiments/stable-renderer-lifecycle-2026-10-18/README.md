# Retained-renderer lifecycle verification

This is an unshipped experiment against parent revision `87258b8af` and
ZoinGallery revision `7ee85a9`. Apply `native-regression.patch` to the parent
repository with ordinary `git apply`. It includes the earlier retained-upload
regressions and the new clip-lifecycle fixture; do not also apply the older
native-regression patch.

The renderer implementation is unchanged from
`../stable-renderer-intervals-2026-10-18/renderer.patch`, which applies with
`--unidiff-zero` to Qt 6.11.1 after the existing production Qt source patches.
No dependency-recipe or production renderer change is included here.

The added fixture cycles nested rectangular or trapezoidal stencil clips 32
times through creation, reparenting, hide/show, reparenting back, and deletion.
Every state is captured and compared against its earlier reference. It also
checks surviving root and element drawing-order bounds after rendering and
requires the maximum global order range to stop growing after warmup.

The fixture uses native scene-graph geometry, with no text or raster leaves.
The capture explicitly distinguishes the stencil boundary from a rectangular
scissor boundary. The unchanged-subroot cases retain their physical-origin and
unit-transform checks at actual DPR 1.75.

See the accompanying
[measurement report](../../reports/retained-renderer-lifecycle-2026-10-18.md).
The normal suite excludes the archived performance assertions because they
require a renderer that is not integrated into the dependency recipe yet.
