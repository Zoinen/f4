# Masonry delegate allocation and recurring preparation — 2026-10-18

## Capture contract

Diagnostic full-translation-unit QtQml allocation hooks plus scoped gallery phase timings. Normal QtQuick buffer-fit library, default 5000 us GC budget, Direct3D 11, DPR 1.75, 160 Hz, Downloads folder with 184 entries. Explicit 30 ms wheel injection: first capture 267 events, second capture 268. These are attribution captures, not clean before/after performance comparisons. Nested phase times and allocation counters are inclusive and must not be summed. Allocation requests are managed-data/string requests, not OS heap calls. Event overflow was zero.

Raw captures: `artifacts/modes-profile/delegate-alloc-wheel-masonry.json` and `delegate-alloc-stats-wheel-masonry.json`. Compact second-capture totals and nearby state frequencies: `artifacts/delegate-allocation-summary.json`.

## Verified findings

First capture recorded 1035 nearby-preparation callbacks; 907 had zero managed-data/string requests. Inclusive nearby preparation time was 372.5532 ms. There were 106 component creations and 208 pool acquisition calls, so reuse exists; repeated component creation is not the only source of work. Total managed-data requests were 69848 and string requests 37191. No conclusion about OS allocation count follows from those counters.

Second capture recorded a stable state with 120 active delegates, 46 nearby indexes, 74 obsolete indexes, zero prepared rows and pending=true 426 times. Another state with 139 active delegates, 67 nearby indexes, 72 obsolete indexes, zero prepared rows and pending=true occurred 392 times. These 818 callbacks performed no row preparation and continued at the timer cadence.

Source explains the mismatch: `retireInactiveDelegates` excludes `retainedVectorSceneIndexes()` from retirement debt, while `prepareNearbyDelegates` counts every active index outside nearby as obsolete. When the ordinary offscreen working set has settled at 64 rows, retained vector scenes can keep the second count above 64 forever. `pending = obsolete > 64` then rearms the 8 ms timer despite no remaining retireable debt. This is a completion-condition bug, not evidence that vector retention should be removed.

Retirement and release run before the 2 ms preparation budget starts; retained-vector processing runs afterward. Therefore that budget does not bound the entire callback. One first-capture visual binding scope took 6.2372 ms; no simultaneous scheduler trace was collected, so elapsed scope duration alone does not establish CPU execution or GC causality.

## Next implementation and verification

Add a regression reproducing offscreen vector retention alongside a settled 64-row ordinary working set, and assert nearby preparation stops rescheduling while retained scenes remain intact. Make the completion predicate use the same exclusions as retirement. Verify near-row preparation, reversal, distant jumps, vector retention and actual-wheel capture afterward. Preserve bounded retention and all normal rendering/GC settings. Then compare clean repeated display-cadence captures; eliminating this timer bug alone does not prove constant 160 Hz.

No production fix or pixel-grid migration was made during these captures.

AI assistance has been used to create this output.
