# Stable renderer intervals and batch retention: checkpoint

## Status

The candidate reduces renderer work and improves one matched clean capture,
but constant 160 Hz is not achieved. It remains an artifact-only experiment;
the portable application and dependency recipe retain the committed renderer.
The source patches and native regression fixture are preserved under
`profiler/experiments/stable-renderer-intervals-2026-10-18/`.

## Mechanism and verification

The earlier incremental clip experiment still invalidated every descendant
batch of a tagged root. The new prototype records prior element orders and root
ranges, rebuilds the lists, and retains a batch only when its membership,
ordering, root range, and global depth normalization remain unchanged. Ordinary
dirty flags remain authoritative. It does not transfer GPU buffers between
batches or cache foreign resource ownership.

Each root owns its free interval. Exhausted descendants borrow from ancestors;
ordinary additions and removals debit/refund only their owning root. Removing
an assigned clip returns its interval. A transient receipt ledger handles
clips created and removed before their first render-list assignment. Unsupported
states retain the full-rebuild fallback. Partial rebuilds preserve the existing
global range rather than incrementing its end.

The native tests exercise merged and unmerged geometry, insertion before and
after an existing root, same-size geometry mutation, material changes,
overlapping alpha, clip growth/removal, and a clip canceled in the same sync.
They assert actual root/element order bounds, unchanged uploads where required,
rendered pixels, and scene-space origins and unit transforms at actual DPR 1.75.
The clean library with no custom test hooks passes eight results:
`artifacts/stable-native-diagnostics-175.txt`. Upload counts use Qt's built-in
renderer diagnostics. Earlier baseline fixtures failed on unrelated reuploads;
the final built-in-diagnostic version has not been rerun against baseline.

An intermediate version immediately recycled removed clips' batches and crashed
at shutdown through double retirement. The saved candidate uses deferred
invalidation and passes the native test including teardown. This does not
establish long-run or general scene-graph correctness.

## Instrumented work counters

Downloads contains 184 entries, density 152, initial extent 4161.142857, actual
DPR 1.75 on a 160 Hz screen. Wheel delta 120 repeats every 30 ms for four
seconds in each direction. The corrected harness sends the first event
immediately and does not restore stale contentY before reversing.

The final instrumented candidate,
`pending-intervals160-p512-wheel-masonry`, records zero full rebuild frames,
35,744,880 unmap-upload bytes and 2,314 buffer creations. Its maximum drawing
order is 56,467. The earlier incremental candidate with padding 128 recorded
six full rebuilds and 123,011,804 bytes. These padding policies differ; this
comparison describes diagnostic progression, not an isolated timing result.
The explicit 512-order override is not a product default.

Eliminating full rebuilds does not eliminate cold SVG costs. One new-scene
frame spends 11.7762 ms preparing, including 8.724 ms uploading and 7.455 ms
unmapping 10,710,888 bytes; 303 buffer creations take 3.41 ms. Tagged-list
reconstruction peaks around 0.54 ms. Some long intervals also contain time
outside measured sync/render stages, still requiring attribution.

## Clean matched display comparison

Both fresh processes use the original default padding, the same workload,
basic native frame hooks and PresentMon. There is no source-site observer,
test logging, UI Automation observer, CPU sampler or compilation during capture.
Capture names under `artifacts/modes-profile` are
`stable-baseline-clean160-default-wheel-masonry` and
`stable-intervals-clean160-default-wheel-masonry`.

| Display interval | Baseline | Candidate |
|---|---:|---:|
| Down median | 6.2504 ms | 6.2500 ms |
| Down p95 | 6.3067 ms | 6.2709 ms |
| Down p99 | 18.7615 ms | 12.5025 ms |
| Down maximum | 37.5311 ms | 24.9949 ms |
| Down intervals over 9.375 ms | 27 | 17 |
| Up p99 | 6.3153 ms | 6.2833 ms |
| Up maximum | 18.7408 ms | 12.4991 ms |
| Up intervals over 9.375 ms | 3 | 2 |

One pair suggests improvement but does not establish a robust performance gain.
The target budget is 6.25 ms; cold downward scrolling still misses it.

## Outstanding gates

The full SVG/navigation/VFS suite, long-run depth/reservation audits, arbitrary
clip reparenting and stencil coverage, all-platform builds, strict dependency
patch integration, import isolation and portable rollout remain unfinished for
this candidate. No claim is made that those gates passed. First-time SVG GPU
uploads and the remaining unmeasured scheduling/incubation gaps are the next
performance investigation, rather than adding more per-frame pixel rounding.
