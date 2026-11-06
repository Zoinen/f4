# Current shared-factory Masonry frame attribution

## Result

Constant 160 Hz remains unproven. Two fresh CPU/scheduling captures of the
current shared-factory implementation distinguish remaining GC pauses from
non-GC binding and metadata work. No production change is made in this
experiment. The next investigation should trace property notifications and
metadata-driven layout commits, alongside retained-graph/allocation reduction.
Another generic GC budget adjustment would miss the non-GC case.

## Measurement contract

Both cases use Downloads, 186 entries, density 152, extent 4101.142857 logical
pixels, an active 160 Hz window at DPR 1.75, 30 ms actual wheel input, the
combined QtQuick renderer and the current ZoinGalleryCore/QML resources.
Caption measurements are zero. No UI Automation observer or QML profiler is
running. WPR CPU samples and context switches are observers; these results
attribute costs and do not establish an uninstrumented performance gain.

`current-shared-stalls-160-wheel-masonry` uses the symbolized current host.
`current-shared-gc-160-wheel-masonry` additionally links the existing full-TU
Qt 6.11.1 memory-manager phase probe. It retains the original 5000 us GC budget
and original marking chunks. It does not use the stale nearby-probe Gallery
archive. The native phase recorder has 2739 of 32768 bounded records including
startup and serializes after measurement; phase records do not grow the vector
during scrolling. Clock/recording overhead remains a limitation.

Exact ETW/QPC alignment matches 50283 context switches in the first case and
41195 in the second, each with a 900 ns offset spread at 10 MHz. Both analyzers
assert zero lost ETW events and buffers. Sampled stacks are inclusive and can
contain linker ICF aliases; apparent QHash helper names alone do not establish
a hash-table bottleneck.

## Current host, without GC phase timers

1273 native intervals: median 6.2310 ms, p95 7.6002, p99 9.3713, maximum
16.4554; 13 exceed 9.375 ms. Sync p95 is 0.1943 ms and render p95 2.5625 ms.

At interval end 107105611825600 ns, the 16.4554 ms interval contains GUI
running 15.7544 ms, ready 0.0460, waiting 0.6550. Render is running 4.8124 ms
and waiting 11.6260; measured render work is 3.2923 ms. Eight of fifteen GUI
CPU samples include GCStateMachine::transition, with QObject/QQuickItem wrapper
marking present. This is GUI work rather than long scheduler starvation or
pure GPU execution. Counts are not exact GC durations.

The next 13.6062 ms interval contains sweeping samples and 5.4881 ms measured
render work. Multiple contributors can coexist within a frame interval.

## Phase-timed repeat

1273 native intervals: median 6.2296 ms, p95 7.5930, p99 8.9898, maximum
16.4335; nine exceed 9.375 ms. Sync p95 is 0.1781 ms and render p95 2.2991 ms.

At interval end 107424287057000 ns, the 12.3136 ms period includes a
5.7291 ms MarkDrain phase entered from event-loop continuation. Its entire
duration is scheduled GUI execution. Six samples include GC transition and
five include QObjectWrapper::markObjects. Another MarkDrain takes 5.5381 ms,
of which 5.5351 ms is running and 0.0030 ms is ready.

The longest native interval, ending 107424312359100 ns, is 16.4335 ms. It
contains 3.2630 ms of GC state work including 3.2129 ms DoSweep; ordinary block
sweeping contributes 2.8838 ms of that. Measured rendering is 6.7967 ms, already
above the 6.25 ms budget. No single-cause explanation is justified here.

### Non-GC GUI spike

The 16.1198 ms interval ending 107424422794000 ns overlaps **zero** recorded
GC state phases. GUI running time is 14.5268 ms; ready time 0.1000 ms and
waiting 1.4930 ms. Render work is 2.3863 ms, with the render thread waiting
12.0070 ms across the interval.

Fifteen GUI samples include seven binding-update stacks, six binding slow-write
stacks and four stacks through writeVarProperty/VariantAssociationObject
detachment. Samples at offsets 3.10, 4.10, 5.10 and 8.10 ms show nested QML
variable-property writes and binding notifications. The exact property names
are not resolved by these native stacks.

At offset 9.82 ms a nearby-delegate acquisition creates a component. At 11.86
ms GalleryIconImage::updatePolish loads a pixmap. At 13.38 ms a panel metadata
response reaches ExternalCatalogMetadataTransaction::emitChangedRows,
MasonryLayout::onDataChanged, updateVisualRowsForDelta, setVisualRow,
GalleryEntryVisual::applySnapshot and QQuickImageBase::setSource. At 14.38 ms
another sample on that metadata path reaches flushLightweightRewrap,
rewrapMasonry, updateProperties and retireInactiveDelegates.

These are separate samples, not exact exclusive function timings. They identify
metadata-driven layout and binding propagation as the next trace target; they
do not prove any particular metadata response was redundant or one setter
consumed fourteen milliseconds.

## Display cadence and limits

Strict PresentMon intervals have both display endpoints inside each direction's
scroll window. The phase-timed repeat has downward median 6.2498 ms, p99
6.3564, maximum 18.7526, four intervals above 9.375. Upward median is 6.2501,
p99 6.2882, maximum 12.4850. GPU active maxima are 2.1455 and 2.6959 ms.
Low GPU activity does not rule out CPU-side driver/upload work.

Earlier clean 31-41 ms gaps were not reproduced in these captures and remain
unexplained. A mostly good p99 does not establish constant 160 Hz. The next
production change needs property-level attribution, relevant regressions and
fresh clean display comparisons; no gain is claimed from this diagnostic turn.

## Artifacts and restoration

Compact inputs, exact clock alignments, summaries, selected full sample stacks,
probe source and reproducibility scripts are preserved under
`profiler/experiments/current-shared-frame-attribution-2026-10-18/` with SHA256
manifest. Large ETLs, symbolized CSVs, binaries and build output stay under
ignored/untracked artifacts and are not part of the compact report bundle.

The diagnostic panel state was restored and the existing normal embedded
worktree application relaunched. No canonical checkout application was stopped,
no production binary was replaced, and no commit or push was performed.
