# Threaded animation startup fix in the portable build

## Implemented

The Qt source recipe now finishes both queued timer-registration stages after
starting new render jobs, before their first commit. The work is guarded by
both newly started jobs and a render thread distinct from the window's GUI
thread. The basic loop retains its existing event ordering. Steady frames do
not deliver registration events. Only the specific render-thread unified timer's
MetaCall events are delivered; no general application event loop is pumped.

The strict, atomic animation-clock recipe patch now includes the animator
controller source. Recipe tests validate all three files, source drift, generated
recipe self-containment, refresh/idempotence and the thread/start guards. The
local Quick archive replaces exactly one controller object and retains the
preceding precise clock, buffer pool and upload policy. Qt remains 6.11.1.

The production controller still uses the existing finite curve. The Hermite
velocity-preserving experiment is not part of this build. Retarget velocity
discontinuities and broader workload coverage remain work toward the original
constant 160 Hz without spikes goal.

## Functional verification

- 36 Python recipe checks pass.
- Main native suite: 22 passed/backend on D3D11/OpenGL, none failed/skipped.
- Final focused checks after extending fixture geometry assertions: 4 passed/backend
 in threaded mode and 6 passed/backend in basic mode, none failed/skipped.
- The direct first-sync registration regression passes. Generic OpacityAnimator
 stop/restart sequences finish at the correct property value and rendered color.
 The fixture checks actual scene origins, extents and unit transforms at 175%.
 Its rendered image was inspected. Existing native leaf/rest-render checks pass.
- Portable QML import isolation, Windows system-DLL import audit, embedded-host
 extraction tests and CGO-disabled launcher build pass. Existing duplicate
 plugin-object linker warnings remain. Source diff checks pass with the existing
 PIXEL_GRID.md line-ending warning; its preexisting non-UTF8 byte is preserved.

An initial generic-animator test incorrectly required an interrupted intermediate
value to be written immediately to the QML property. This failed in basic-loop
OpenGL against both old and new Quick libraries, and still failed with a wait.
[Qt's Animator contract](https://doc.qt.io/qt-6.11/qml-qtquick-animator.html)
guarantees property update on completion. The final test checks accepted stop,
successful restart, completed property value and actual rendered output. The
failed checks and old-library comparison remain in the evidence.

## Production-path performance capture

Downloads186/Masonry152, DPR 1.75, 160 Hz, 30 ms wheel events with boundary reversals;
four 10-second input windows. Same production library and controller as the host,
with bounded diagnostic material/phase probes and no general frame recorder.
No builds, tests, analysis or UIA run during input windows.

After space recovery, the complete capture has 12 recorded jobs, no repeated
zero-time commits and no committed frames with an unavailable precise sample.
All recorded Hermite ratios are -1, confirming the production curve path.

| Window | Max display interval ms | Motion error p95 px | Max motion error px | Missed refreshes |
|---|---:|---:|---:|---:|
| Cold 0 | 6.3783 | 0.1025 | 0.8451 | 0 |
| Cold 1 | 6.4148 | 0.1028 | 0.8534 | 0 |
| Warm 0 | 6.3766 | 0.1384 | 0.9970 | 0 |
| Warm 1 | 6.4463 | 0.1167 | 0.9632 | 0 |

The motion metric covers unchanged curve segments (roughly 1300 pairs/window),
including GUI base-translation changes when the composite mapping is stable.
It excludes genuine retarget/geometry seams. Presentation brackets are complete.
The p95<=0.5px, maximum<=2px, zero-missed-refresh guard passes this capture.
These are regression ceilings, not a definition of full goal completion.

## Failed/invalid captures retained

The first production-path capture has no startup repeats across 18 jobs but fails
the pacing guard: one cold display gap is 18.734 ms, with joined camera-step error
87.879px. Before that gap, preparation reaches 7.0954 ms, followed by a frame with
13.4604ms pacing/sync and no GUI synchronization. Its cause is still unproven;
the new startup-registration body does not execute in those steady frames.
This failure is retained and is not erased by the passing recovered run.

The following repeat exhausted workspace space and produced an empty warm JSON;
it is invalid as a full capture. Its interrupted PresentMon process left our
named ETW session alive. A subsequent attempt was rejected before input because
that session already existed. The session was queried and stopped by its exact
task-owned name; failed outputs were preserved. Four old scheduler CSVs were
losslessly compressed and decompressed SHA256 equality verified. No evidence
was discarded. The fresh capture began with 1.79 GB free disk and about 64 GiB free
memory; before/after health snapshots are recorded.

Thus the startup defect is fixed, while occasional late spikes remain a known
open part of the goal. No cross-platform runtime performance claim is made from
these Windows tests.

## Deployment and evidence

After packaging passed, only the exact worktree launcher tree was replaced.
`D:\Code\f4-zoin\f4-zoin.exe --gui=qt` is responsive. The extracted cached host
matches the rebuilt host; no host or frame-latency override is active. The
canonical checkout was untouched. Nothing was committed or pushed.

- Launcher SHA256: `a614afe36d4e61ab3a7d02aa5ad9d4bd74b148ccdd8e483f3ea7757b53d81e87`
- Host SHA256: `914500ac514408f563e2cbfc38e829703abba442dd25244e7842204c0626a531`

Verified compressed evidence is indexed by
`../experiments/threaded-start-production-2026-10-20/manifest.json`.
Next investigate late preparation/pacing spikes and safely integrate velocity
continuity, retaining reversal, bounds and deadline behavior. The goal remains active.
