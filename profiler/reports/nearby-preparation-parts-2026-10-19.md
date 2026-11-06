# Nearby preparation phase costs — 2026-10-19

## Result

This iteration is diagnostic progress toward constant 160 Hz; the goal remains
unachieved. No production scheduling limit or behavior is changed. The verified
lazy-video-badge portable build is restored and startup stderr remains empty.

The existing native phase recording already covers retirement, release,
retained-vector lookup and pixel-grid work. An offline breakdown of the preceding
lazy-badge trace shows that the largest GC-free warm nearby preparation call
spends 3.1030 ms in its row loop, versus 0.1219 ms before and 0.0576 ms after.
Retirement takes 0.0955 ms, release 0.0026 ms. The earlier suggestion that work
outside the timer dominates is not supported for that call. The timer still
does not bound the whole function or predict the cost of the next row.

## Method

Diagnostic copies of MasonryLayoutDelegateCommit.cpp add bounded native scopes
around bindDelegateRowIdentity, bindDelegateRowVisual, applyDelegateRowLayout,
finalizeDelegateRow, hiding and retained-vector maintenance. A second version
adds scopes inside bindDelegateRowVisual for setVisualRow and index notification.
The diagnostic library is linked into a dedicated host; production source and
the installed Qt SDK are unchanged. Events use native steady-clock nanoseconds,
are serialized after measurement and have a 131,072-event cap.

Two cold/warm captures use the same live process/engine within each capture:
Downloads 186 entries, density 152, 160 Hz, actual DPR 1.75, two four-second
boundary-bounce wheel windows, 30 ms wheel interval, 5 ms GC budget. Frame
recording, object census and UI Automation are disabled; builds/tests/offline
analysis do not overlap acquisition. The first capture includes WPR scheduling;
the second uses only the native scopes/GC observers and PresentMon. These are
instrumented phase measurements, not clean before/after cadence comparisons.

## Measurements

| Capture / interval ending QPC ns | Whole nearby ms | Before loop ms | Binding ms | Layout ms | GC union ms |
| --- | ---: | ---: | ---: | ---: | ---: |
| Parts cold / 146788495707500 | 8.2023 | 0.0341 | 7.3491 | 0.6954 | 6.4479 |
| Parts warm / 146807036310900 | 8.3400 | 0.0646 | 7.6044 | 0.4897 | 6.4571 |
| Parts warm / 146803683340000 | 5.9706 | 1.2779 | 2.1935 | 2.2098 | 0 |
| Notify cold / 146943471952700 | 3.9486 | 0.0591 | 3.1260 | 0.6159 | 0 |
| Notify warm / 146951810580700 | 4.1659 | 0.7280 | 2.1295 | 1.0451 | 0 |

The 8 ms calls contain GC nested inside binding; attributing their complete
duration to row binding itself would be incorrect. Another notify warm call
lasts 8.5163 ms, but 6.5664 ms is in release and 6.0771 ms overlaps GC. Moving
or shrinking only the row loop would not remove that spike.

The GC-free notify cold 3.9486 ms call spends 2.5504 ms publishing a snapshot
and 0.5679 ms notifying indexes; it prepares one row. The notify warm 4.1659 ms
call spends 0.9396 ms in snapshot notification and 1.1767 ms in index notification
across two prepared rows. Another GC-free warm 3.5530 ms call has snapshot
1.5375 ms, index notification 0.5578 ms, and layout 0.6521 ms. Costs vary by
recycled row and source type; these are observed examples, not constant costs.

One GC-free cold row acquisition takes 4.3984 ms by itself. Creation and reuse
therefore also need distinct treatment; a two-row cap cannot guarantee a frame
deadline. Nested scope durations overlap: do not sum parent and child scopes.
These scope durations alone do not prove exclusive scheduled CPU time.

## Source-level implication and next experiment

GalleryDelegateItem::setVisualRow toggles visualIdentityChanging around snapshot
publication. GalleryEntryDelegateBase uses that flag to deactivate/reactivate
its preview. In the layout commit sequence this reopening occurs before
viewSourceIndexesChanged is notified and before applyDelegateRowLayout publishes
the final geometry. Recycled rows can therefore reactivate preview bindings,
then process another index notification, then respond to layout changes.
The identity guard is required to prevent displaying pixels belonging to the
old entry and must not simply be removed.

Next establish a failing regression that records preview reactivation against
final row identity, indexes and geometry, then evaluate a coherent row-commit
boundary that keeps the preview guarded until those values are installed.
Preserve standalone setVisualRow behavior and same-entry updates; do not clear
ready thumbnails or disable useful nearby preparation. Measure notification
counts and GC-free phase costs before running a clean alternating timing trial.
Atomic GC and render-thread resource work remain independent bottlenecks.

## Evidence limitation

The first phase host and map were overwritten when adding the notification
scopes. Its launch SHA is 7ebf674ed39445898060382a43ffac5242ef33b0f2c38ef1d8999585537fccbb;
the notification build SHA is
94a2217811cca9b4e0967e8b797b99e4759ea3281e1b9e3dca56a84ccabd9877.
The later scheduling-symbol pipeline correctly rejects the mismatched first
map (computed image base fails alignment). No symbolized stack attribution
from that failed pipeline is accepted. Recorded native phase intervals remain
direct evidence from their respective captured binaries. Keep future diagnostic
variants under distinct immutable host/map names before capture.

Raw ETW and PresentMon CSVs stay in artifacts. Compact source snapshots,
generation/analysis scripts, phase JSON and launch hashes are archived in
profiler/experiments/nearby-preparation-parts-2026-10-19 with a SHA-256 manifest.
No new portability claim or full-suite success is inferred from this diagnostic.

AI assistance has been used to create this output.
