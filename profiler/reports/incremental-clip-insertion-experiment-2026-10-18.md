# Incremental clip insertion: native proof, insufficient scrolling benefit

## Verified mechanism

Qt 6.11.1's batch renderer unconditionally requests FullRebuild when adding a
clip, even when an existing parent batch root has unused drawing-order slots.
The artifact-only renderer prototype tags that existing parent, seeds the new
clip's temporary geometry budget from it, and lets Qt's geometry visits debit
the ancestor reservations. Nested new roots without assigned ranges and scenes
without an existing parent retain the original full-rebuild path. Exhausted
geometry reservations also retain Qt's normal full render-list rebuild.

V1 reserves the prospective clip padding before visiting geometry. V2 assigns
new-root padding during the tagged rebuild, capped by the remaining budgets
of all ancestors. It does not change Qt's ABI, geometry buffer ownership,
merging, shaders, texture sampling or document rendering.

The native regression fails on the committed renderer at the latched
full-rebuild assertion. The candidate verifies unchanged total drawing-order
range after insertion, scissor clipping, translucent blue over red, and
unrelated yellow drawing in another clip root. Expanded fixtures check empty
clips followed by drawing, insufficient geometry reservations, and nested-root
fallback. Actual scene origins and unit transforms are checked for the affected
rectangles at the test's requested 175% scale. These tests contain no text or
raster-icon leaves.

V1 and V2 each pass eight fixture/test results, including the existing opacity
and ancestor-reservation regressions. The final V2 exhausted-budget case adds
eight geometry nodes under a parent with only four spare orders. Its native
results are `artifacts/incremental-clip-v2-expanded-175.txt`.

This is narrow coverage. The candidate has not passed the full SVG suite,
arbitrary clip reparenting/stencil coverage, long-run reservation/depth audits,
or the portable rollout gates. It is not integrated into the dependency recipe.

## Real workload

The Downloads workload is 184 entries, density 152, initial extent 4161.142857,
actual DPR 1.75, active 160 Hz window. Wheel delta 120 repeats every 30 ms for
four seconds in each direction. The corrected harness does not jump to a stale
extent on reversal. Detailed source counters and PresentMon run without UI
Automation or a CPU sampler; timings include instrumentation overhead.

With the original four-order minimum, the V1 candidate still records 15 full
rebuild frames. Recorded clip-add fallback events decrease from 11 in the prior
source-equivalent baseline to three, but reservation exhaustion replaces them.
Unmap uploads are 193,568,224 bytes. This is not an elimination of the spikes.

A fresh matched pair tests the same explicit 128-order environment override:

| Metric | Fresh baseline | V2 candidate |
|---|---:|---:|
| Complete rebuild frames | 8 | 6 |
| Unmap upload bytes | 122,997,680 | 123,011,804 |
| Buffer creations | 5,838 | 5,168 |
| Maximum drawing-order count | 18,580 | 18,840 |
| Native frame p99 | 13.8692 ms | 13.8189 ms |
| Down display p99 | 18.7086 ms | 18.7415 ms |
| Up display p99 | 6.3296 ms | 6.3221 ms |
| Down display intervals >9.375 ms | 19 | 26 |
| Up display intervals >9.375 ms | 4 | 3 |

Capture names are `incremental-baseline160-p128-wheel-masonry` and
`incremental-clips160-v2-p128-wheel-masonry` under `artifacts/modes-profile`.
They use fresh processes and the same current gallery sources. One pair does
not establish a timing improvement. Despite fewer global rebuilds, upload work
and display p99 do not improve. The 128-order minimum is only a diagnostic
override; no product reservation policy has been selected or shipped.

The candidate's resting window capture was inspected: both panels, SVGs,
captions and thumbnails are present without the geometry corruption seen in
earlier buffer-handoff experiments. A resting capture does not prove every
intermediate scrolling frame or general scene correctness.

## Why this is insufficient

`Renderer::buildRenderListsForTaggedRoots()` expands each tagged root to all its
subroots with `tagSubRoots()`, then invalidates/recycles every batch owned by
those roots. Tagging the shared SVG parent therefore still rebuilds already
loaded SVG children and uploads their geometry. Counting only FullRebuild would
misidentify these captures as a useful optimization.

The next investigation must preserve unaffected SVG batches inside a changing
parent, including their draw-order ranges and resources. A reusable-root design
must also avoid introducing a full rebuild every time a cold viewport slot is
created; the companion persistent-root experiment demonstrated that failure.
Any further allocator/reuse change needs overlapping-alpha and source/geometry
replacement regressions plus real rendered captures, not upload counters alone.

## Disposition

No candidate renderer or new order-padding default is shipped. The temporary
native test is copied to `artifacts/IncrementalClipNativeFixture.cpp` with its
patch in `artifacts/incremental-clip-native-prototype.patch`, then removed from
the product test source. Candidate libraries and build scripts remain under
`artifacts/qt-incremental-clip*` and `artifacts/build-*incremental*`.
The persistent-QML prototype is also reverted. Existing committed native-icon
and blocked-opacity fixes remain intact. Constant 160 Hz is unfinished.

## Normal build restored

After reverting the experiments, the normal host and embedded CGO-disabled
launcher are rebuilt. The committed native renderer suite passes four results
at 175%; resource-only imports and compiled-host startup pass four results.
The Windows system-DLL audit and tagged embedded payload/extraction tests pass.
Both saved prototype patches pass `git apply --check` against the restored
sources. Tracked product sources in both repositories have no remaining edits.

The running `D:/Code/f4-zoin/f4-zoin.exe --gui=qt` uses the matching extracted
normal host with SHA256
`6da10713964bc5d1d6af82b30274f0b6f2ae74b9a980067f91ad7218f4243e5e`.
Its title identifies `zoin_branch/cached-pixel-grid`, startup stderr is empty,
and the diagnostic host and reservation overrides are cleared. Identity is
recorded in `artifacts/post-clip-experiments-identity.json`.

AI assistance has been used to create this output.
