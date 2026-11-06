# Persistent SVG clip roots: measured and rejected

## Workload and hypothesis

The existing Downloads workload contains 184 entries at density 152, actual
DPR 1.75 and a 160 Hz screen. Wheel events arrive every 30 ms, four seconds
in each direction. The corrected harness sends the first event immediately
and does not reposition to a stale extent before reversing direction.

The prototype keeps a row's structural scene and an empty SVG clip alive while
separately gating its preview, selection, captions, folders and interaction
drawing. Raster drawing remains outside the SVG clip. A pooled slot keeps its
scene-branch family through source readiness and reuse. Existing retention of
up to 16 heavy SVG scenes is unchanged.

The native regression initially fails because the structural clip is absent.
With the prototype it verifies the clip's presence in the renderer's actual
shadow-node map while the row is hidden, identical pixels after revealing it,
and the existing scene-space leaf origins/unit transforms at DPR 1.75. This
proves that the mechanism works, not that it improves scrolling.

## Measurements

These captures use source-site instrumentation and PresentMon. Their frame
times include observer overhead and do not establish production performance.
They have matching folder, density, initial extent 4161.142857, DPR and refresh
metadata. No UI Automation observer or CPU sampler runs during capture.

| Capture | Full rebuild frames | Clip additions | Clip removals | Unmap upload bytes | Buffer creations | Max drawing orders |
|---|---:|---:|---:|---:|---:|---:|
| early-roots-baseline160-wheel-masonry | 15 | 11 | 2 | 204,336,108 | 9,509 | 2,538 |
| persistent-clips160-wheel-masonry | 123 | 364 | 282 | 1,452,880,204 | 53,967 | 3,964 |
| persistent-clips160-v2-wheel-masonry | 48 | 86 | 0 | 524,496,752 | 22,269 | 3,934 |
| persistent-clips160-v3-wheel-masonry | 17 | 9 | 0 | 297,468,536 | 10,954 | 2,592 |

Individual cause events can overflow their bounded trace array. Clip event
counts are recorded events; complete-rebuild and upload totals use aggregate
counters. The baseline is a previously captured source-equivalent build,
not a simultaneous randomized trial. The increases in work are sufficient
to reject the prototype without claiming a precise timing delta.

The first version accidentally moves retired slots back to the common parent
when their view index becomes -1. Their new clip roots therefore move twice
per reuse. V2 preserves the branch through that invalid-index interval and
eliminates the recorded clip removals. However, newly allocated raster slots
still acquire clips, each triggering Qt's full rebuild.

V3 creates a slot's clip only when it first receives an SVG, then retains it
for reuse. This avoids unnecessary raster clip insertions, but full rebuilds
and upload work remain worse than baseline. Its native p99 is 18.1756 ms with
a 29.3994 ms maximum; constant 160 Hz is not achieved.

## Verification limits and disposition

The original native lifecycle fixture passes (three fixture/test results).
A broader 175% selection has 20 passing results and one failure: an existing
catalog-reset test assumes the selection rectangle is directly parented by
the row. The new selection gate invalidates that fixture assumption. The
prototype changes its lookup to walk to the row, but that edit and the later
retirement regression were not rebuilt/retested before rejecting the mechanism.
Do not count those later test changes as verified.

All gallery product and test changes from this experiment are reverted. The
complete final prototype is saved as
`artifacts/persistent-vector-clips-prototype.patch`; native results and capture
JSON/PresentMon files remain local artifacts. The committed native-icon and
blocked-opacity fixes are preserved. The normal embedded worktree app is
restored without a diagnostic host or order-padding override.

Creating structural roots for every future row shifts expensive rebuilds to
slot creation; keeping properties or nodes stable after creation does not
solve first-entry cold scrolling. The next experiment targets Qt's unconditional
full rebuild when inserting a clip under an existing batch root. It must prove
reservation accounting, drawing order, clipping/blending and unchanged unrelated
roots, then improve the actual workload before any production integration.

AI assistance has been used to create this output.
