# Masonry preparation bursts and bounded scene retention — 2026-10-21

Status: useful attribution, unsuccessful optimization candidate. Production application code is unchanged by this experiment. Constant 160 Hz without spikes remains unachieved.

## Instrumentation and accepted input

The previous bounds probe did not support a local-bounds cache. A new diagnostic splits the current production batch renderer's preparation into resource-batch acquisition, render-list rebuilding, cleanup, batching, sorting, vector-gate setup, uploads/admission, opaque and alpha material preparation, resource submission, and completion. It also records cumulative render-thread geometry-dirty notifications, recursive node additions/removals, and subtree-blocked notifications. These counters are render-thread totals, not a count of unique objects or newly constructed QML items.

All hooks are reversible against the SHA-pinned production renderer. There is no change to its rendering algorithm. Fixed pre-touched storage has a checked capacity; completed rows publish with release/acquire ordering. These are wall-clock stage timings, which can include scheduler preemption. They do not prove that each measured millisecond is CPU execution or a GPU wait.

Accepted captures use Downloads with 187 entries, density 152, DPR 1.75 and 160 Hz. Cold/warm extents match across arms (4075.142857/3623.142857 logical pixels). Middle scrolling moves the pointer one quarter of the window height. Rapid wheel input is requested every 10 ms. Focus, record capacity, native animation and source/build identity checks pass. Each cold/warm capture has two approximately ten-second input blocks. Builds, tests, and offline analysis do not run during capture input.

## New attribution result

`real-scroll-preparation-20261021` captured a 12.0714 ms preparation frame:

| Stage | Wall time |
| --- | ---: |
| Before batch-renderer preparation body | 1.1396 ms |
| Render-list rebuilding | 0.8137 ms |
| Batch preparation | 2.6391 ms |
| Upload/admission section | 6.0214 ms |
| Alpha material preparation | 1.0642 ms |

The corresponding frame records 547 recursive scene-node additions, no removals, two geometry-dirty notifications and seven blocked-state notifications since the previous preparation. Another 7.5987 ms warm preparation frame also adds 547 nodes and spends 2.8281 ms in uploads and 2.0404 ms in batching. Other boundaries produce larger counter changes. This identifies a useful visibility-boundary reconstruction/upload burst to investigate; it is not proof that one particular SVG, allocation, or driver call caused the whole tail.

Across this initial run, alpha preparation and the prefix before the batch renderer dominate aggregate time. Uploads and batching dominate several individual tails. An aggregate hotspot must not be substituted for tail attribution.

## Retained-prepared-scene candidate

The candidate changes only the diagnostic controller's render culling rectangle during native motion: extend it one viewport height above and below the actual visible rectangle. It keeps the existing prepared-delegate and motion-coverage bounds; the owning clip and vector presentation viewport remain unchanged. The same executable selects the ordinary rectangle or expanded rectangle through an environment flag. This is diagnostic code, not a production option.

The test asked whether avoiding some blocked/unblocked transitions would reduce repeated reconstruction. It reduced blocked notifications but increased total recursive additions and upload/material work. It does not establish persistent GPU residency for each prepared surface.

### Middle scrolling, both run orders

| Order / pass | Control preparation p99, blocks 0 / 1 | Expanded p99, blocks 0 / 1 |
| --- | --- | --- |
| Control first / cold | 2.2915 / 1.6860 ms | 2.9458 / 2.7058 ms |
| Control first / warm | 1.7528 / 1.6096 ms | 2.8080 / 2.7768 ms |
| Expanded first / cold | 1.9039 / 1.6871 ms | 2.6814 / 2.6936 ms |
| Expanded first / warm | 1.8140 / 1.6658 ms | 2.4897 / 2.9188 ms |

The candidate worsens preparation p99 in all eight comparisons. The first pair has four display intervals above 9.375 ms in control and none in the candidate. Reversed runs have none in either arm. Expanded middle scrolling stays below the existing 2-physical-pixel displayed-step regression ceiling in all eight blocks; two cold control blocks exceed it (13.1088 and 6.0559 px). Thus it has a potentially useful middle-motion observation, but no repeatable demonstrated missed-refresh advantage, and it consumes more CPU/frame headroom.

### Rapid wheel stress

| Pass | Control render-to-frame-end p99, blocks 0 / 1 | Expanded p99, blocks 0 / 1 |
| --- | --- | --- |
| Cold | 2.9934 / 2.6298 ms | 6.0794 / 5.3515 ms |
| Warm | 3.0832 / 2.9079 ms | 5.4364 / 5.6728 ms |

All rapid blocks have zero display intervals above 9.375 ms. This does not establish smoothness: maximum displayed curve-step error is 46.4348 px in control and 36.7203 px in the candidate. Expanded frame-end maximum reaches 8.8492 ms. Both fail the existing motion ceiling. The rapid pair was not reversed: the candidate is not being promoted and already fails the required motion gate while substantially increasing preparation cost.

## Decision and next target

Do not promote wider culling as the solution. Do not relax motion or cadence gates to accept it. Preserve it as a measured diagnostic tradeoff, not a production improvement.

Next isolate which uploads at a visibility boundary bypass or exceed the existing vector admission budget, including reintroduced previously ready batches and non-gated geometry. Existing admission occurs after render-list rebuilding and batch construction, so merely tightening its byte budget cannot bound the earlier work. A fix must reduce or distribute that reconstruction while preserving actual visible content, source priority, painter order and portable static rendering. Separate submission waits and clock/display-step deviations also remain unresolved.

## Verification and limits

- Probe unit tests: 4 passed, no failures/skips (stage/counter snapshots, incomplete records, bounded overflow).
- Actual D3D11 renderer tests at DPR 1.75: 44 drawing-order and 6 curve cases passed, no failures/skips.
- Both diagnostic hosts pass 4 embedded-import/compiled-host checks and system-DLL audits.
- Native tests cover the instrumented renderer; they do not establish every visual detail of the expanded-motion experiment. It has not passed production rollout, pixel-render comparison, or full viewer/panel gates.
- The initial 12 ms attribution capture and later controlled pairs ran at different times; their wall times must not be presented as a before/after improvement. Unrelated background workloads were not terminated.
- No commit, push, portable payload replacement or normal application restart was performed for this diagnostic. Only owned diagnostic launcher trees were stopped afterward.

Evidence, source, receipts and test logs are archived with SHA-256 manifest in `profiler/experiments/preparation-bursts-2026-10-21`. Executables, object files and raw traces are excluded.
