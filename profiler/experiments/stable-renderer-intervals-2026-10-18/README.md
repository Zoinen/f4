# Stable renderer intervals: experimental checkpoint

These patches preserve an unshipped Qt 6.11.1 experiment. They are not applied
by the dependency recipe or compiled by the normal test/build targets.

`renderer.patch` applies to `src/quick/scenegraph/coreapi/qsgbatchrenderer.cpp`
after this repository's existing ancestor-order and blocked-opacity patches.
It contains no profiling hooks or exported test APIs. `native-regression.patch`
applies to this repository and adds the corresponding native regression cases.
The tests intentionally require behavior absent from the current production
renderer, so they are archived rather than enabled in the normal suite.
Both patches use zero context; apply with `git apply --unidiff-zero` after
checking the documented base revision and Qt version.

The prototype gives each root ownership of its free drawing-order interval,
borrows capacity from ancestors when needed, refunds removed or unpublished
clip reservations, and retains unchanged batches after partial list rebuilds.
Buffers stay owned by their original native Qt batches. Immediate recycling
during node removal was rejected after a shutdown double-free; the saved
version defers invalidation cleanup to Qt's normal render pass.

The native fixture uses Qt's built-in `QSG_RENDERER_DEBUG=render` diagnostics
to count uploads for a specific root. Run it only in a separate test process;
debug logging must be absent from performance captures. The tested environment
used `ZOIN_TEST_NATIVE_RENDERER=1`, `ZOIN_EXPECT_TEST_DPR=1.75`,
`QT_ENABLE_HIGHDPI_SCALING=0`, `QT_SCALE_FACTOR=1.75`,
`QT_SCALE_FACTOR_ROUNDING_POLICY=PassThrough`, and Basic controls.

See [the findings report](../../reports/stable-renderer-intervals-experiment-2026-10-18.md)
for measurements, test coverage, and outstanding verification. No new padding
default, dependency-recipe integration, or portable rollout is included.
