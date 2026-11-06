# Nearby preparation paced by completed frames — 2026-10-19

Status: rejected isolated experiment. Constant 160 Hz without spikes is not
achieved. Production source, Qt SDK, dependency patch, embedded payload and the
normal running worktree application are unchanged by this experiment.

## Hypothesis and implementation

The preceding native scheduling capture found delegate acquisition, QML creation,
incubation and scene-graph work in long producer intervals, including intervals
with no GC overlap. Independent 8 ms preparation timers can collide with frame
work. Test whether completed-frame pacing and a smaller row admission limit help.

A diagnostic copy of MasonryLayoutDelegateCommit.cpp replaces the nearby timer
with one persistent native QObject scheduler per layout. When exposed, it admits
a pending callback through a queued frameSwapped signal and requests another
frame if further work remains. Without an exposed window it retains an 8 ms
fallback. Each nearby callback prepares at most one new row instead of two;
the existing 2 ms loop timer, viewport plan, visible-row commits, recent-row pool,
16-vector retention and thumbnail priorities remain intact. The experiment
changes nearby scheduling for its existing Masonry/Columns callers; performance
measurement exercises Masonry only.

The helper uses a separate sidecar without changing the installed Qt ABI or the
MasonryLayout class layout. The modified object is placed into a separate gallery
library and linked into f4-qt-host-frame-nearby-clean.exe. The normal library and
host remain untouched. This experiment combines pacing and row-count changes;
it does not isolate their individual effects.

## Correctness receipts

A standalone lifecycle regression first failed: a frame signal queued from the
old window could admit work after the item moved to another window. A window
generation check corrects that case. The waiting subprocess receipt changes from
exit 1 / passed=false to exit 0 / passed=true. It verifies deferral, one callback
per supplied frame, chained progress on a subsequent frame, idle frames, window
replacement and destruction cancellation/registry cleanup. This deterministic
synthetic test does not prove real-display frame pacing or smoothness.

Selected native mode checks pass at DPR 1.75 on Direct3D 11 and OpenGL: 44 results
per backend including fixture setup/cleanup, zero failures. They cover nearby
preparation and settling, retained SVG scenes, thumbnail reuse/replacement,
coherent row commits, mode geometry/navigation, scene branches and actual leaf
pixel-grid checks. Rendered captures include small captions, columns, retained
vectors and Details previews. This is not a full repository test-suite claim.

## Clean comparison

Six separate fresh-process captures run in A1/B1/B2/A2/A3/B3 order. A is the
current bounded-Details clean baseline, B the frame-paced prototype. Each has
two 10-second boundary-bounce wheel windows, 30 ms wheel input spacing, Downloads
186 entries, density 152, extent 4101.142857 logical pixels, actual DPR 1.75 and
160 Hz display. Both use the original Qt collector with a 2 ms budget. There are
no native per-frame probes, object census, caption measurements, model-change or
snapshot-only signals. Input/motion validation confirms active scrolling and
trajectory reversals. No builds, tests, UI Automation or offline analysis run
during acquisition. The normal worktree application remains alive alongside
both diagnostic arms.

A long display interval is >9.375 ms, halfway between one and two 160 Hz refresh
periods. This is displayed frame cadence, not exclusive CPU duration.

| Run | Long display intervals | Maximum interval (ms) | Block p99 (ms) |
| --- | ---: | ---: | --- |
| A1 | 11 | 12.6183 | 6.3937 / 6.3467 |
| B1 | 17 | 18.7597 | 6.4616 / 6.3607 |
| B2 | 13 | 12.5153 | 6.3496 / 6.3816 |
| A2 | 10 | 18.7481 | 6.3945 / 6.3031 |
| A3 | 22 | 18.7716 | 12.4753 / 6.3639 |
| B3 | 33 | 24.8301 | 6.8950 / 12.3948 |

Totals across 60 seconds per version: A=43 long intervals, B=63. There is no
measured benefit supporting adoption. The variation between fresh processes
prevents treating this small trial as a general statistical estimate of the
regression magnitude. In particular, callback pacing does not bound the cost of
one synchronous row creation, queued incubation, GC, or subsequent scene work.

## Decision and next action

Do not merge or deploy this scheduler. Preserve the original nearby preparation.
The result rejects this combined pacing/one-row proposal as a demonstrated fix;
it does not show that all deadline-aware admission designs are ineffective.

Next investigate separating synchronous top-level delegate construction from row
admission. Any asynchronous shell/pool prototype must preserve required QML
properties, final row identity/geometry publication, cancellation, mode changes,
viewport priority, bounded retention and eventual prefetch progress. Measure
actual construction/readiness cost and first-visible latency as well as cadence;
do not trade missed frames for permanently absent or delayed previews without
reporting that cost. The independent GC/rendering spikes remain unresolved.

The diagnostic panel was restored before stopping only the exact-path
artifacts/f4-zoin-frame-nearby.exe process tree. The normal worktree application
remains on host SHA-256
509c4b470b89b3171bb4f8f7e31faf9d9fc9011800c2e1bc1bdebfb50aa893ca.

Compact sources, regression receipts, captures, clean measurement JSON, comparison
summary and SHA-256 identities are preserved in
profiler/experiments/frame-paced-nearby-preparation-2026-10-19. Executables,
libraries, raw CSV/ETL and build outputs are excluded.

> AI assistance has been used to create this output.
