# Interleaved alpha batching: isolated experiment

## Decision

Keep this experiment outside the normal build. It substantially reduces draw
batches, but the clean alternating comparison does not establish smoother frame
delivery. The existing Details clip fix remains in the normal worktree app.
Constant 160 Hz without spikes remains unachieved.

## Mechanism and correctness checks

Qt 6.11.1 contains an optional `QSGBATCHRENDERER_INVALIDATE_WEDGED_NODES` path.
It excludes already-assigned nodes from alpha overlap checks and accumulates
the invalidation order range until render preparation completes. Both behaviors
are required together. Merely skipping the overlap check is not this experiment.

The isolated renderer translation unit selects this existing algorithm. Its
optional lower/upper range state lives in a render-thread-local per-renderer
table, erased by the renderer destructor, so the installed Renderer class ABI
does not change. No SDK library or production dependency patch is replaced.

The new `interleavedAlphaBatchesPreservePainterOrderAcrossChanges` regression
compares nine captured states with a reference that forces independent,
full-window clip roots: normal interleaving, overlapping movement, color/geometry
changes, insertion in painter order, removal, visibility, partial clipping,
opacity, and ancestor translation. It verifies physical leaf origins and unit
transforms at actual DPR 1.75. The original algorithm uses seven initial batches;
the candidate uses two, with identical rendered output in all states.

The native renderer suite passes 31 results on each of Direct3D 11 and OpenGL at
175%, including fixture setup/cleanup. The normal algorithm also passes the new
rendered-order checks. Set `F4_EXPECT_INTERLEAVED_ALPHA_MERGE=1` when running the
isolated candidate to additionally require the two-batch reduction. This optional
assertion does not claim that the normal build includes the experiment.

## Workload attribution

Post-input census, Downloads 186 entries, left Masonry density 152, right Details,
actual 160 Hz and DPR 1.75:

| Category | Normal after Details fix | Isolated candidate |
| --- | ---: | ---: |
| Total text batches | 81 | 45 |
| Masonry raster-branch caption text | 46 | 11 |
| Masonry raster-branch textures | 37 | 8 |
| Masonry raster-branch caption backgrounds | 37 | 11 |
| Masonry vector-branch caption text | 9 | 9 |

These are diagnostic snapshots, not frame-time measurements.

## Clean frame delivery

Immutable clean builds, A1/B1/B2/A2, two 10-second wheel windows per run: 40 seconds
per version. Native per-frame probes and census disabled; actual motion and
reversals verified. All runs have 186 entries, density 152, scroll extent
4101.142857 logical pixels, 160 Hz, DPR 1.75, original collector, 5 ms GC budget,
zero caption measurements/model-change signals, and active-window input.

| Run | Version | Display intervals >9.375 ms | Worst (ms) | Window p99 (ms) |
| --- | --- | ---: | ---: | --- |
| A1 | Normal | 3 | 18.7688 | 6.3459 / 6.2849 |
| B1 | Candidate | 3 | 12.5023 | 6.2933 / 6.2911 |
| B2 | Candidate | 2 | 18.7243 | 6.3124 / 6.3014 |
| A2 | Normal | 3 | 12.4948 | 6.2946 / 6.3005 |

Six long intervals versus five is not persuasive evidence of a cadence gain.
Do not promote a global rendering change based only on fewer batches. The next
investigation is the measured GC phases exceeding the 6.25 ms frame budget;
non-GC render/driver spikes also remain possible and require direct attribution.

## Reproduction

Diagnostic helpers: `artifacts/build-wedged-batching.py`,
`artifacts/build-wedged-tests.cmd`, `artifacts/compare-wedged-batching-160.py`.
The normal build still uses `artifacts/qt-combined-upload/Qt6Quick.lib`.
Clean comparison data uses prefix `artifacts/modes-profile/wedged-batching-160-`;
the diagnostic owner census is `wedged-batching-wheel-masonry.json`.

> AI assistance has been used to create this output.
