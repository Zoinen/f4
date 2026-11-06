# Incremental sweep: cached internal-class lifetime regression

Status: isolated prototype only. The normal worktree application still uses the
original Qt collector. The 160 Hz goal remains active and unachieved.

## Reproduction and evidence

The earlier object-heavy standalone test used few distinct shapes. A new shape
test creates 12,000 distinct property layouts. With ephemeral property-name
strings it passes both original and experimental collectors. Retaining those
property-name strings separately exposes a deterministic failure in the old
incremental-sweep prototype while the original collector passes.

`IncrementalSweepCachedShapeRegression.cpp` keeps all property keys alive, drops
most objects, then creates objects with those same layouts between sweep slices.
It verifies kept/new graphs and allocator free-list accounting, forced completion,
interrupted shutdown and two independent engines. The old prototype returns
`0xc0000005` during `QV4::MarkStack::drain` and produces no successful result JSON.
Use a waiting process API for these GUI-subsystem harness executables: PowerShell
invocation alone can return before termination and is not an exit-code receipt.

A full 257 MB dump was captured separately. The popped heap object's first two
words contain a free-list next pointer and a two-slot free extent. The mark stack
has reached already-reclaimed storage. Raw dumps remain in `artifacts/` and must
not be committed or included in the experiment archive.

Qt's [InternalClass implementation](https://raw.githubusercontent.com/qt/qtdeclarative/v6.11.1/src/qml/jsruntime/qv4internalclass.cpp)
caches class transitions, returns an existing cached class on lookup, marks its
parent/name map, and removes transition-cache entries during class destruction.
The original atomic sweep finishes regular-object and class cleanup before JS
resumes. The prototype yielded after regular storage reclamation but before
unused transition entries were removed. A cached class could then be reused with
its managed name-map backing storage already freed.

This is a demonstrated standalone lifetime bug. The earlier live-app crash had a
similar mark callback failure, but its small minidump lacks the required object
memory; identical root cause for that particular crash is not yet established.

Three short cold debugger runs were stopped before completing both windows and
do not count as successful reproductions. Three extended cold debugger runs did
complete both windows without reproducing the earlier live crash. Their debugger
overhead means their timing is not performance evidence.

## Isolated correction

Before any regular sweep can reclaim storage, walk the initial internal-class
chunks and remove weak transition lookups whose target is unmarked. Clear the
detached child's parent pointer so its later destructor cannot remove a replacement
cache entry. Process one chunk at a time with deadline checks. Defer identifier
table sweep until pruning finishes and the usual mark-stack redrain has protected
objects reached by intervening JS work.

Allocation quarantine, deferred chunk decommit, free-bin rebuilding, forced
completion and normal mark-barrier redrain remain in place. The installed Qt ABI,
SDK libraries and production dependency patch are unchanged. The correction is
in `artifacts/IncrementalSweepCachePruneExperiment.h`, built into separate libraries
under `artifacts/qt-qml-incremental-sweep-pruned*`.

## Verification so far

- The previously crashing cached-shape regression passes. Non-forced sweep:
  177 slices, longest measured complete step 0.2112 ms in one standalone run.
- Cached-shape test with incremental-GC cross-validation passes: 178 slices,
  longest step 0.3454 ms. This is not a live UI latency measurement.
- Original 60,000-object graph test with cross-validation passes: 394 slices,
  longest step 0.1513 ms, plus forced completion, shutdown and two-engine cases.
- Selected Qt/QML checks with cross-validation pass on each of Direct3D 11 and
  OpenGL at DPR 1.75: 88 results per backend, including fixture setup/cleanup.
  This is not a claim that the entire repository test suite is green.
- First live diagnostic input capture completes: Downloads 186 entries, Masonry
  density 152, actual 160 Hz/DPR 1.75, two 10-second boundary-bounce wheel windows,
  2 ms GC budget. Within those windows, DoSweep max 2.0425 ms (10 steps), mark drain
  max 2.8395 ms (15 steps), and sweep redrain max 0.0361 ms. Cumulative startup
  events are excluded from those measured-window statements. No frame-delivery
  claim can be made from this phase-only run.

## Clean frame-delivery comparison

The completed A1/B1/B2/A2/A3/B3 comparison uses the original (A) and corrected
prototype (B), both with a 2 ms GC budget. Each run has two 10-second wheel windows,
186 entries, density 152, DPR 1.75 and a 160 Hz display. Frame probes, object census,
caption measurements and model-change signals are absent; input/motion receipts
verify active scrolling. The normal worktree application stays alive alongside the
separate diagnostic launcher throughout both arms.

| Run | Display intervals >9.375 ms | Maximum display interval (ms) | Block p99 (ms) |
| --- | ---: | ---: | --- |
| A1 | 3 | 12.4985 | 6.2971 / 6.3068 |
| B1 | 9 | 18.7267 | 6.3258 / 6.3302 |
| B2 | 15 | 18.7644 | 6.3963 / 6.4281 |
| A2 | 13 | 25.1219 | 6.4591 / 6.3796 |
| A3 | 12 | 18.7264 | 6.4125 / 6.3870 |
| B3 | 16 | 12.5655 | 6.4044 / 6.3732 |

Across 60 seconds per version, A has 28 long display intervals and B has 40.
The prototype does not demonstrate a smoothness improvement and is not deployed.
The lifetime correction fixes the demonstrated standalone failure; that correctness
result must not be mistaken for achieving the frame-delivery goal.

## Scheduling attribution capture

A separate diagnostic capture records native GC phases, sampled stacks, scheduling
and PresentMon in the same PID/engine for cold and warm runs, each two 10-second
input windows. Both complete without crashing. The cold run has 23 long display
intervals (maximum 24.7996 ms); warm has 2 (maximum 12.4797 ms). These instrumented
runs are diagnostic evidence, not a clean A/B performance comparison. The scroll
extent changes from 4101.142857 to 3649.142857 logical pixels between captures, so
cold and warm are not identical settled-layout workloads.

Some early misses overlap GC, including multiple steps totaling over 5 ms within
one producer interval despite the 2 ms per-step budget. Many misses, including
both warm misses, have no GC overlap in their surrounding producer intervals.
Native scheduling/sample attribution is required before choosing the next change.

Scheduling export initially filled the workspace disk. The incomplete export was
removed; six old generated CSV exports were gzip-compressed and independently
SHA-256 checked against decompressed contents before removing their uncompressed
copies. Original ETL traces remain intact. Export/analysis is outside capture time.

## Native scheduling result

The completed offline pipeline aligns 152,361 cold and 100,855 warm context
switches to raw 10 MHz QPC, with 900 ns offset spread in each trace. Both ETL
metadata records report zero lost events/buffers. Displayed misses match exact
producer timestamps within 1 microsecond. Host-only nearest-map symbols preserve
ICF aliases; samples are sparse and do not establish exclusive function costs.

The cold 19.4632 ms producer interval has zero GC overlap, 17.3903 ms of scheduled
GUI work and 10.0392 ms of scheduled render work. These overlap; they must not be
summed as serial frame time. GUI samples include createComponent,
prepareDelegateRow/popBrickItem, property capture and incubation. Render samples
include node traversal, bounds, batching and buffer work. Another 18.9368 ms
interval has 2.1274 ms GC overlap but 17.5468 ms GUI running time; GC alone cannot
explain it. Both threads have very small ready-but-unscheduled totals in these
examples, so prolonged CPU starvation is not supported for those intervals.

For the first warm miss, the corresponding producer interval is 7.7163 ms:
GUI running 5.3750 ms, render running 3.5733 ms, zero GC. Nearby preparation and
incubation appear in sparse GUI stacks. For the second warm miss, its producer
interval is 7.8674 ms: GUI running 1.7614 ms, render running 4.8094 ms, zero GC.
Display latencies are 13.1041 and 13.8679 ms, GPU-active durations 0.4616 and
0.4794 ms. These are not proof of a single driver or application function causing
the misses. Producer interval, scheduled CPU time, display latency and GPU-active
time measure different things.

The evidence rejects incremental sweep alone as a sufficient fix. Next isolate
remaining delegate admission/creation and scene-graph mutation costs with the
original collector as the control. Coordinate preparation with frame deadlines
rather than independently budgeting each callback; preserve useful prefetch and
vector/thumbnail correctness. Any candidate needs clean alternating runs and
175% geometry/rendering checks before promotion.

The diagnostic panel was restored and only its exact-path launcher tree stopped.
The normal worktree launcher PID 22772 and its cached host PID 42012 remain alive;
no canonical checkout process was stopped.

## Pending verification

Longer live runs, further lifetime/weak-cache scenarios and all remaining frame
spikes still need investigation before any production rollout. The installed SDK,
production dependency patch and normal running application remain on the original
collector. The 160 Hz goal is unachieved.

> AI assistance has been used to create this output.
