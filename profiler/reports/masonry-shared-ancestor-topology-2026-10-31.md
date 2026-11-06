# Masonry: shared ancestor topology

## Outcome

Constant 160 Hz remains unachieved. The shared-topology prototype reduces repeated
ancestor traversal, improves the graph helper by roughly fourfold for rebuilds
and sixfold for path churn, and halves before-body p99 in the verified-warm pair.
Cold A/B/B/A does not show a reliable whole-frame delivery improvement. All cold
runs fail the original gates; both warm arms still exceed the motion limit.
Keep the implementation experimental, without a production rollout.

The preceding turn was progress: a bounded integer-set replacement passed
correctness checks but did not reliably improve scrolling. This turn changes the
graph representation itself. The rejected word-set and driver-threading flags are
not enabled. No product source, installed Qt archive, embedded payload, normal app,
commit or push changes.

## Shared representation

The original reverse graph stores a complete ancestor path for every preprocessing
entry and an integer set at every ancestor. The candidate stores one vertex per
unique ancestor, with parent/child/sibling indices and an optional entry index.
A new path stops at its first already-indexed ancestor. Invalidation walks the
indexed subtree and targets the same preprocessing entries. Structural retirement
collects affected IDs before erasing metadata; empty vertices are pruned upward.
No QSGNode pointer is dereferenced during cached subtree retirement.

Parent walks, collection and pruning are iterative. Scratch and vertex vectors
grow geometrically with explicit maximum capacities. The existing 16384-entry,
32768-ancestor and 131072-logical-link limits remain. Logical path depth is still
charged to the old link limit even though physical ancestry is shared, preserving
the existing capacity fallback behavior. Failed additions publish no partial
topology and remain retryable after a graph revision. Root exclusion, force
fallback, updater type checks, mutation handling and address retirement remain.

The candidate does not drop a dependency or preprocess callback, reduce image
quality, rasterize SVGs, change layout/alignment or introduce a setting. It uses
F4_PROFILE_SHARED_ANCESTRY in the diagnostic host. Original reverse sets are the
control; clean inherited-transform skipping and geometric snapshot storage are
enabled in both. Buffer prewarming is off, and ordinary D3D11 threading remains.

The same host serves both live arms, SHA256:
`377aede0ad96b241b36e1c5e7cdd3bee351dd27484226db6b8efeb876097b272`.
QSGNode notifications, QSGRenderer preprocessing and the host snapshot consumer
are rebuilt together with consistent dispatch definitions. Legacy set-capacity
fields are zero/not applicable for the topology, rather than a total-memory claim.
Actual topology vertices are 32 bytes on this build; other records, hash tables,
vectors, allocation headers and process resources are additional memory.

## Reproduction and correctness

A fixed tree has 384 preprocessing leaves and 2386 unique ancestors. Both variants
report identical answers and 9600 logical links. The original performs 9600 parent
reads, failing a regression budget of one read per unique ancestor. The shared
index performs 2386 parent reads and passes. All ancestor affected sets are checked
against direct scene-parent traversal, not just an implementation-generated count.

Final helper checks:

- Fourteen bounded cache checks pass, preserving known replacement, unrelated
  notification reuse, targeted block/opacity invalidation, reparenting, destroyed
  ancestor address reuse, callback mutation, root exclusion, force fallback,
  unknown updater rejection and repeated capacity fallback.
- Nine existing eligibility checks pass.
- Five differential QtTest results pass, including 10000 random activation and
  retirement steps with preprocessing nodes also used as ancestors, 100 subtree
  retire/reparent cycles, and an overlong path rejected without partial state.
  Queries compare affected node identities with the original graph. Audits verify
  parent/child/sibling consistency, entry identities, depth, logical link counts
  and all vector/count bounds.

The three previous set-capacity accounting assertions are replaced with topology
audits and live-vertex/vector-capacity checks; the functional scenarios and logical
link-capacity fallback are retained. This is a representation-specific memory
check, not a relaxed performance or invalidation gate.

Both policies pass the actual in-flight native mutation fixture (three QtTest
results each) and all 46 native renderer results at actual DPR 1.75, zero failure
or skip. Counters prove cache use, with no native graph failure or capacity bypass.
All 25 PNG files match exactly: 23 rendered captures and two fixture inputs.
Existing static-leaf scene-coordinate and unit-transform checks remain; the small
native caption capture was visually inspected. Three embedded import-isolation
checks and the new host's Windows system-DLL audit pass. No portable payload is
generated or claimed verified by these diagnostic checks.

## Helper benchmark

Two original/topology pairs use the same tree and checksum, 200 rebuild samples
and 2000 eight-path churn samples per run, with no builds or live capture during
measurement.

| Policy | Rebuild median ms | Eight-path churn median µs |
| --- | --- | --- |
| Original, first | 0.8741 | 18.3 |
| Shared, first | 0.2053 | 2.9 |
| Shared, repeat | 0.2177 | 3.0 |
| Original, repeat | 0.8717 | 18.5 |

The helper preserves the 9600 logical links and 2386 unique ancestors. It does
not prove a comparable whole-frame gain. Build and helper directories were
hash-verified into the named C: archive with original-path junctions to make room;
no running executable inside those folders was moved.

## Live cold A/B/B/A

Actual Downloads contains 192 entries including parent. Masonry density is 152;
desktop 3840×2160, refresh 160 Hz and actual DPR 1.75. Every process gets a fresh
isolated application cache, normal viewport loading and two ten-second bouncing
wheel blocks. All build/hash, input, foreground, display, feature-use and bounded
record guards pass. Inputs are 1769/1720/1735/1731, inactive inputs zero. Graph
failures and capacity bypasses remain zero. No screenshot, UI Automation query,
compiler, test or offline analyzer runs during gestures. PresentMon and the existing
native instrumentation remain enabled.

Paired values refer to the two directions; times are milliseconds and motion is
physical pixels. Coverage holds are zero throughout.

| Arm | Before-body p99 | Preparation max | Display max | Intervals >9.375 ms | Maximum motion |
| --- | --- | --- | --- | --- | --- |
| A1 reverse sets | 0.8005 / 0.5568 | 6.4806 / 2.7635 | 12.6658 / 12.4679 | 29 / 1 | 117.368 / 39.710 |
| B1 shared | 0.5843 / 0.5637 | 6.1579 / 5.3919 | 12.6563 / 12.6673 | 38 / 37 | 121.416 / 116.773 |
| B2 shared | 0.5983 / 0.6201 | 5.8566 / 6.8682 | 12.9044 / 12.8193 | 36 / 33 | 120.505 / 120.700 |
| A2 reverse sets | 0.8463 / 0.6908 | 6.2496 / 6.9723 | 12.6916 / 12.6857 | 26 / 42 | 123.385 / 130.831 |

The first direction's before-body p99 is lower for both candidates, but total
preparation and display results do not establish a stable delivery gain. Source
completion timing differs between runs; a cold second direction is not labeled
verified warm. The whole-capture preparation-observer maximum in B2 is 322.5 µs,
versus 42.5/36.7/65.4 µs in the other arms. Its measurement-window location is not
identified by that aggregate; it remains an observer-overhead limitation, not a
reason to discard the unfavorable run. These wall spans are not pure CPU time.

## Verified-warm pair

The existing full-folder tour displays all 192 entries under normal visibility
and waits for quiescent previews before both measurement windows. All 1836/1826
inputs are active, graph failures/bypasses zero and coverage holds zero. This is
one A/B pair, not a repeated A/B/B/A delivery claim.

| Arm | Before-body p99 ms | Preparation p99 ms | Display max ms | Long intervals | Maximum motion px |
| --- | --- | --- | --- | --- | --- |
| A1 reverse sets | 0.5186 / 0.5016 | 1.7514 / 1.7320 | 6.4325 / 6.4306 | 0 / 0 | 3.2499 / 3.0582 |
| B1 shared | 0.2604 / 0.2761 | 1.6406 / 1.8170 | 6.4541 / 6.3838 | 0 / 0 | 3.5169 / 2.2688 |

The warm traversal/preprocess phase is substantially cheaper, but both arms still
fail the two-pixel maximum-motion gate. Whole preparation does not uniformly
improve. No middle-scroll measurement or full-goal acceptance is claimed.

## Next evidence and disposition

The shared index is a tested component optimization, not a completed scrolling
fix. Preserve it as an experimental candidate. The next investigation should
follow the whole-frame critical path, including GUI synchronization, recording,
uploads/material work and submission, rather than infer a display improvement
from preparation-only helper timings. Existing scheduler/profile traces contain
both GUI and render-thread data and provide a starting point. Any new upload
strategy also needs comparison with the already-rejected queued-Dynamic route.

The full objective remains actual cold wheel, verified-warm wheel and quarter-
height middle scrolling at 160 Hz/DPR 1.75, zero displayed intervals above
9.375 ms, maximum motion error at most two physical pixels, zero coverage holds
and the original native middle-velocity accuracy. No gate is changed.

Raw locations, hashes, source and compact results are frozen under
`profiler/experiments/masonry-shared-ancestor-topology-2026-10-31`. Owned diagnostic
processes and loggers are stopped, the display hold is released and the normal
worktree Go/Qt application remains alive. The goal remains active.
