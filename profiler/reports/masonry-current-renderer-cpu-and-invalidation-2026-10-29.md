# Masonry: current renderer CPU, waits, and cache invalidation

The constant-160-Hz goal remains **unachieved**. Two new captures identify a
remaining CPU target without changing cache semantics: the preprocessing cache
still repeats many ancestor walks, and almost all recorded invalidations come
from topology and subtree-block changes. Forced updates are rare. A separate
long upload wait also remains; fixing CPU invalidation alone cannot yet be
claimed to solve every spike.

## Current-code scheduler capture

Case `real-scroll-current-renderer-cpu-20261029-wheel-masonry`, Qt PID 17624,
render thread 13492, GUI thread 122760. Actual Downloads, 191 entries, Masonry
density 152, 3840×2160 / 160 Hz / actual DPR 1.75. Two ten-second rapid-wheel
blocks, initially empty isolated application cache, normal thumbnail workers.
No serial video-conversion lane or context-prewarm intervention is enabled.
Preprocessing eligibility, vector-owner resolution, AV private-heap and decoder
device reuse remain enabled, with the existing resource bounds.

The host is relinked from the exact current preprocessing-eligibility response
file solely to obtain a matching symbol map. Its `.text` and `.pdata` hashes
match the existing baseline executable exactly. The captured executable and map
are pinned by the build receipt; PE timestamps, image base, code/unwind sections,
and runtime-function bounds are checked before resolving application symbols.
ICF aliases remain explicit. No installed Qt library or production payload is
modified. Windows system-DLL audit passes.

The owned kernel trace captures
`PROC_THREAD+LOADER+CSWITCH+DISPATCHER+PROFILE`, with
`Profile+CSwitch+ReadyThread` stacks. Ownership is checked before stopping its
session. No tests/builds/UI Automation/offline analysis run during gestures.
The normal worktree app is preserved.

- Original workload guards pass: 1,525 active wheel events, correct display,
  content, recording bounds and resource bounds.
- 12,398,136 ETW events, 3,604,425 context switches, 1,767,380 ready events.
- Zero lost events/buffers, schema errors or ProcessTrace errors. The existing
  published-schema ReadyThread fallback is retained and disclosed.
- ETL: 1,025,245,184 bytes; verified lossless gzip: 200,854,315 bytes.
- SHA256: `22ad147a0d830668c91ac4071031c483dc2ae02eb75a57adc054e49a91ad0a24`.
- Stack reader exports 163,381 stacks and 23,968 profile records. Raw profile
  prefixes independently agree with all 72 selected xperf-decoded references.

## Preparation and waits

Across 3,170 measured preparations:

| State | Total ms |
| --- | ---: |
| Scheduled | 3,901.6968 |
| Runnable, waiting for CPU | 4.7997 |
| Blocked | 9.0614 |
| Unknown | 0 |
| Wall | 3,915.5579 |

Scheduled time includes interrupts/DPC execution on that CPU. Their separate
durations are not captured here; it is not a measurement of pure application
instruction time. The CPU samples independently establish substantial renderer
work. Of 3,872 preparation profile samples, one lacks a matching stack fragment;
its self IP is retained, while its inclusive attribution is unavailable.

Selected self sample counts (counts are not invocation counts or CPU ms):

| Function | Samples |
| --- | ---: |
| `QSGNodeUpdater::isNodeBlocked` | 402 |
| Batch updater `visitNode` | 287 |
| `Renderer::updateMaterialDynamicData` | 168 |
| `QRhiBufferData::assign` | 149 |
| `Renderer::prepareRenderMergedBatch` | 104 |
| Batch updater `visitTransformNode` | 104 |

All 402 `isNodeBlocked` self samples have `QSGRenderer::preprocess` as caller.
It remains the largest sampled self hotspot despite the existing cache.
Accumulated wall scopes: before renderer body 1,146.73 ms, alpha materials
1,208.77 ms, uploads 694.65 ms, render lists 365.64 ms, gate setup 135.20 ms.

The longest preparation is **13.1281 ms**, all scheduled, with uploads 5.9080 ms,
before-body 3.9779 ms and render lists 1.5475 ms. Its samples include ancestor
blocking checks, native scene-subtree blocking, a NVIDIA/D3D11 geometry Map path,
and shader-resource-binding updates. Do not attribute the entire scheduled span
to one sampled function.

A distinct **8.4558 ms** preparation contains **7.9918 ms blocked** in the
geometry-buffer Map path (`F4GeometryReuse::map`, called by the dynamic-buffer
full update during batch upload). ReadyThread CPU context independently agrees
with the stack payload: an NVIDIA worker in the same process readies the render
thread. A separate 4.125 ms uniform-buffer Map wait is readied from a System
thread with dxgmms2 frames. Private NVIDIA functions remain offsets; exact lock
objects and GPU execution time are not established by these stacks.

The intrusive run still fails the original gates:

| Block | Display max, ms | Intervals >9.375 ms | Max motion error, physical px | Coverage holds |
| --- | ---: | ---: | ---: | ---: |
| 0 | 25.2922 | 14 | 261.5051 | 0 |
| 1 | 18.3574 | 14 | 116.7518 | 0 |

These are attribution measurements, not a clean optimization comparison.

## Invalidation-reason probe

Case `real-scroll-preprocess-invalidation-reasons-20261029-wheel-masonry` uses
the same scene/cache semantics with six aggregate reason counters added. Each
notification increments only thread-local integers; scopes aggregate into
atomics once after preprocessing. There is no per-notification clock call,
allocation or atomic increment. Flags in a combined notification each count,
so flag totals need not equal notification totals.

Counter regressions cover independent flags, combined flags, ignored
matrix/geometry/material flags, and transfer/reset at scope exit. The executable
passes. Native mutation tests and embedded-QML import isolation also pass with
the new renderer/node objects at actual DPR 1.75. The native mutation fixture's
output paths are isolated from the original baseline evidence. The diagnostic
host passes the system-DLL audit. No static UI layout was changed.

The live capture passes the unchanged workload guards and records:

| Reason | Flag occurrences |
| --- | ---: |
| Subtree blocked | 19,005 |
| Node added | 18,980 |
| Node removed | 18,248 |
| Opacity | 102 |
| Forced update | 11 |
| Use-preprocess flag | 0 |

Overall: 56,263 invalidating notifications, 3,298 preprocessing passes, 413,335
cached results, 690,398 original ancestor queries, peak 404 entries, no bypasses
or mid-pass invalidations. These totals include startup and loading; they do
not independently correlate a specific dirty flag with the longest frame.

The result rejects the earlier suspicion that forced updates frequently erase
this cache. It supports investigating a bounded reverse dependency table:
subtree/opacity notifications invalidate only preprocessing leaves depending
on that ancestor, while unrelated geometry insertion/removal need not invalidate
every leaf. Topology changes affecting known dependencies, source/root changes,
deleted nodes, address reuse and arbitrary updater overrides require explicit
fallbacks and native regressions. Retain global invalidation where locality
cannot be proved. No selective-invalidation implementation is claimed here.

## Evidence and state

Frozen compact evidence:
`profiler/experiments/masonry-current-renderer-cpu-and-invalidation-2026-10-29`.
Large ETL/exports/map and diagnostic binaries remain at externally hashed paths.
Inactive binaries and completed raw text packets were NTFS-compressed with
before/after SHA256 equality; evidence was preserved rather than deleted.
Only the diagnostic launcher's exact executable tree was stopped; the normal
worktree app PID 43936 was verified alive. No commit or push was made.

The next implementation target is dependency-specific cache invalidation with
mutation/ownership correctness proven first. Upload/driver waits remain a
separate target. Cold, verified-warm rapid-wheel, and quarter-height middle
scrolling still require the original zero-miss, motion and coverage gates.

AI assistance has been used to create this output.
