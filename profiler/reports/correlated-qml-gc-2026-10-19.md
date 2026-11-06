# Correlated QML, GC and scheduling attribution

## Result

The longest QML binding calls are mostly collector work, not intrinsically slow
selection/readiness expressions. Non-GC frame spikes still exist. A simultaneous
CPU capture identifies GUI filesystem checks in the generated SVG component
load path as a concrete next investigation. No production change or cadence gain
is claimed in this diagnostic turn. Constant 160 Hz remains unfinished.

## Capture and clock contract

Two fresh diagnostic processes use the current enrichment-role implementation,
current shared-factory Gallery core/QML, combined QtQuick renderer and matching
full-TU Qt 6.11.1 GC phase library. Original GC budget/chunks remain unchanged.
Downloads has 186 entries, density 152, extent 4101.142857; both captures are
active 160 Hz/DPR 1.75 and use 30 ms wheel input. Caption measurements are zero.
QML profiler records binding, JavaScript and signal-handler ranges. These are
instrumented attribution captures, not clean before/after measurements.

The diagnostic host reads the engine profiler's existing elapsed timer at each
direction's start and capture completion. Native steady-clock timestamps bracket
each read. A derived accessor obtains a pointer to the protected Qt timer; no
Qt timer or ownership is changed. This code exists only in the artifact host.

First capture: three clock offsets spread by 300 ns, with a conservative maximum
bracket/deviation uncertainty of 850 ns. All 267 QML GalleryPanelFacade wheel
handler starts fall inside their corresponding native wheel-handler intervals.
Second capture: offset spread 100 ns, uncertainty 200 ns, all 268 wheel handlers
match. This validates cross-clock alignment rather than assuming trace epochs.
Analyzer assertions reject excessive offset spread or a mismatched wheel event.

The analyzer uses exact native direction windows. It unions nested QML ranges
and GC state intervals before computing overlaps, avoiding duplicate time from
binding/JavaScript/signal nesting. Subtracting GC still leaves inclusive elapsed
work, not exclusive CPU time or one property's own implementation cost.

## First correlated capture

`correlated-qml-gc-wheel-masonry`: 1274 native intervals, median 6.2418 ms,
p95 7.2934, p99 8.8005, maximum 12.6579, eight intervals above 9.375 ms.
The XML completes successfully (26,621,170 bytes). There are 2121 bounded GC
records including startup, below the pre-reserved cap of 32768.

The longest binding is GalleryEntryDelegateBase.qml:16 (Accessible.selected),
6.1799 ms. Its nested selected expression at line 114 is 6.1784 ms. Both overlap
6.1476 ms of native GC state work; only about 0.0323/0.0308 ms remains after
subtraction. This is an allocString-triggered collection spanning many marking
states, including a 2.9139 ms MarkDrain; it is not one 6.15 ms atomic phase.
GalleryVectorImage.qml:103 similarly falls from 5.5816 ms inclusive to 0.1510 ms
after removing GC overlap.

The longest remaining QML range is preview commitComponent at 0.8236 ms, with
its nested sourceComponent binding at 0.8218. These are the same call chain,
not two additive costs. Selection arithmetic is not the justified next target.

Six of the eight long native intervals have no GC overlap. For example, an
11.6369 ms interval contains 2.56945 ms of unioned QML ranges and 3.0815 ms
measured render work; a 10.0291 ms interval contains 3.2899 ms unioned QML work
and 2.8359 ms render work. Native/render and GUI ranges can overlap across
threads; their durations must not simply be added to explain the whole period.

## Simultaneous CPU/scheduling repeat

`correlated-qml-gc-sampled-wheel-masonry` adds WPR sampled CPU/context switches.
It records 1275 native intervals: median 6.2333 ms, p95 7.4505, p99 8.8728,
maximum 11.5878, eleven above 9.375 ms. Four of those eleven overlap GC and
seven do not. Its QML XML completes successfully (26,170,567 bytes); GC records
are 2129, below capacity. The largest GC-subtracted QML range is 1.5970 ms
preview commitComponent, again inclusive of component creation.

Exact ETW/QPC alignment matches 52237 context switches, with 900 ns offset
spread at 10 MHz. Scheduling/sample analyzers assert zero lost events/buffers.
Symbol aliases from linker identical-code folding remain a limitation.

At native interval end 109955740541700 ns, period 11.5878 ms:

- GC overlap: zero; unioned QML ranges: 4.0414 ms.
- GUI scheduled running: 7.4230 ms; ready: 0.2560; waiting: 3.9088.
- Render scheduled running: 5.2408 ms; measured render work: 3.9146.
- Six GUI CPU samples include component incubation/finalization, nearby delegate
  preparation and two file-selector stacks.

One full stack reaches VectorSourcePin::loadComponent -> QQmlComponentPrivate::loadUrl
-> QQmlTypeLoader::getType -> QQmlFileSelectorInterceptor::intercept ->
QFileSelectorPrivate::selectionHelper -> QFileInfo::exists -> GetFileAttributesExW
-> the Windows filesystem/filter-manager path. Another file-selector sample is
resource lookup/type resolution. The first stack establishes synchronous GUI
filesystem access during generated SVG component loading; two samples do not
measure its total duration or identify the queried URL.

Source inspection finds VectorSourcePin loading a private generated Prepared.qml
from its revision's temporary directory with QQmlComponent::Asynchronous.
The mode does not prevent the initial GUI selector checks. The next step is to
time URL interception and identify which generated-component checks can be
avoided while preserving normal application/platform file-selection semantics.
Do not disable global selectors merely from this stack, and do not switch to
synchronous GUI compilation with setData to hide the file lookup.

A separate 11.0756 ms interval overlaps 5.7040 ms GC. GUI running is 10.1826 ms,
and samples include GC transition and nearby component creation. Another
10.5523 ms non-GC interval has GUI running 4.4937 ms and waiting 5.9376 ms;
not every longer native period is continuous GUI CPU work. No single cause
accounts for every outlier.

## Evidence and restoration

Compact native captures, clock joins, scheduling/sample summaries, selected
full stacks, diagnostic code and build identity are saved with SHA256 manifest
in profiler/experiments/correlated-qml-gc-2026-10-19. Raw XML/ETL/symbol CSVs stay
in profiler/traces and artifacts, outside the compact bundle. Observer overhead
precludes claiming a clean cadence improvement from these captures.

The normal embedded worktree application is restored with its unchanged binary.
Its launcher/child hashes match the prior normal build and stderr is empty.
WPR recording is stopped; no canonical app process is touched. No production
file change, commit or push is made by this diagnostic turn.
