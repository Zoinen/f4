# Batch invalidation and isolated thumb transform — 2026-10-21

Status: attribution and a tested diagnostic prototype. No production source, SDK archive, embedded payload or normal running application was replaced. The constant-160-Hz goal remains incomplete.

## Invalidation causes

The reversible instrumentation classifies calls to `invalidateBatchAndOverlappingRenderOrders` by geometry, material, opacity, or transform context. Recursive transform walks retain the outermost source node. Records include the invalidated batch's material and render-order range and the number of overlapping batches invalidated by that call.

A single item/node label snapshot is collected after synchronization while Qt holds GUI objects locked. It is not a per-frame UI Automation observer. Source addresses without a matching snapshot remain unmapped. A snapshot does not prove the identity of a later reused transient node address; conclusions below rely on the persistent scrollbar control, source repetition and a separate native regression.

Accepted attribution prefix: `real-scroll-invalidation-20261021`, Downloads, 187 entries, density 152, DPR 1.75, 160 Hz, middle pointer one quarter of the window height down. Both cold/warm input windows pass focus and bounded-buffer checks. No builds/tests/analysis run during input.

| Pass/block | Invalidation calls | Transform calls | Thumb transform calls | Geometry calls |
| --- | ---: | ---: | ---: | ---: |
| Cold / 0 | 1,873 | 1,870 | 1,596 | 3 |
| Cold / 1 | 1,882 | 1,881 | 1,600 | 1 |
| Warm / 0 | 1,875 | 1,874 | 1,600 | 1 |
| Warm / 1 | 1,881 | 1,880 | 1,599 | 1 |

The dominant source maps to the content MouseArea of `galleryPanelScrollBar`. Its invalidated batch contains the `galleryScrollBarHandle` smooth-color material and a single render-order position. It does not cascade into other batches. Thus it requests batching work each frame, but does not explain the whole ordinary-upload population. Less frequent raster-branch transforms and preview/geometry changes remain.

This corrects the interpretation of the earlier thumb-freeze test: suppressing only the native thumb matrix write did not freeze Qt's GUI-side thumb positioning during synchronization. It therefore did not rule out the thumb as the main source of *invalidation calls*. The trace distinguishes that from the main source of total upload work.

## Isolation prototype and regression

The artifact-only renderer recognizes an opt-in flag on a `QSGTransformNode` and promotes that transform to a batch root through the same existing path used by vector roots. The diagnostic scroll job marks only its thumb transform when enabled. No source geometry, item positions, matrices, opacity, clipping, or visible behavior is intentionally changed.

The prototype uses `0x00800000` in Qt's subclass-reserved flag range, restricted to transform nodes. This is an experimental private contract for the pinned Qt 6.11.1 source, not an approved public Qt API or production-ready ABI policy. Flag collision/compatibility audit and portable recipe integration have not been performed. No QML control hierarchy was changed.

A native regression moves a small translucent rectangle ten times at DPR 1.75:

- Baseline ordinary and marked transforms both upload ten times. The marked case fails the zero-upload assertion (3 passed, 1 expected failure, no skips).
- Patched ordinary transform still uploads ten times; marked transform uploads zero times (4 passed, no failures/skips).
- Actual leaf origins and local unit vectors are checked at every static pose. Both images match byte-for-byte in all RGBA channels within each backend.
- D3D11 expanded regression suite: 46 passed, no failures/skips.
- Focused OpenGL regression: 4 passed, no failures/skips, including zero marked uploads.
- The two captured backend image sets are preserved separately. A final D3D capture reproduces the original PNG hashes after the OpenGL run.
- Host import-isolation/compiled-host checks: 4 passed; system DLL audit passes.

## Live same-executable pair

Prefixes: `real-scroll-isolated-thumb-{control,enabled}-20261021`. The environment flag only marks the thumb in the enabled arm. Both arms use the same executable and verified receipt. All captures run at 160 Hz / DPR 1.75 with matched cold/warm extents (4075.142857 / 3623.142857), 187 entries and density 152.

| Pass/block | Invalidation calls control → enabled | Preparation median control → enabled | Preparation p99 control → enabled |
| --- | ---: | ---: | ---: |
| Cold / 0 | 1,911 → 314 | 0.31485 → 0.30340 ms | 2.1862 → 2.2179 ms |
| Cold / 1 | 1,902 → 304 | 0.30110 → 0.29180 ms | 1.7240 → 1.8158 ms |
| Warm / 0 | 1,909 → 309 | 0.31290 → 0.30065 ms | 1.7557 → 1.8802 ms |
| Warm / 1 | 1,900 → 301 | 0.32295 → 0.28500 ms | 1.7638 → 1.8371 ms |

This removes about 1,600 invalidations per ten-second block and modestly lowers measured medians. It does not improve preparation p99 in this pair. Render-to-frame-end p99 improves only in cold block 1; the other three increase slightly. Both arms have zero display intervals above 9.375 ms. Maximum displayed-step error is 3.9728 px in control cold block 0, versus <=0.3573 px in every enabled block. No motion-coverage stalls occur. One pair is insufficient to attribute this display-tail difference causally.

## Decision and remaining work

Keep the prototype unpromoted. The mechanism is verified and small steady work is removed, but no general tail-time win is established. Do not add a private marker to production solely to claim the performance goal is achieved. This pair was not reversed or rapid-wheel-tested because this is not a promotion candidate yet.

The important distinction is now measured: a dominant count of cheap thumb invalidations versus larger bursts from entering/reconstructed content. The remaining path needs batch-level residency/lifetime attribution around those content boundaries and investigation of the separate preparation/submission and display-step tails. Merely disabling valid updates or changing the smoothness gates is not acceptable.

Evidence, source, receipts, labels, test logs, images and analysis are archived with SHA-256 manifest in `profiler/experiments/batch-invalidation-thumb-2026-10-21`. Executables, objects and raw traces are excluded. No commit or push was made.
