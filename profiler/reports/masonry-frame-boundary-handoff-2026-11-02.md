# Frame-boundary delivery of completed rows

## Result

The two-root diagnostic queue can retain completed roots until its queued
notification runs. Delivering that already-pending notification before the next
frame's queue scheduling removes a verified handoff delay. It does not yet
establish an aggregate scrolling improvement or meet the constant-160-Hz goal.
The candidate remains diagnostic; the normal embedded application is unchanged.

The previous turn was progress: a source change, native regression tests and a
new live capture. This investigation adds a tested lifetime/occupancy join, a
negative/positive queue regression, and four further live captures.

## Evidence leading to the change

`profiler/coverage_queue_occupancy.py` joins an attributed queued blocker to its
exact readiness snapshot and owner, then examines the owner's recorded root
lifetimes at both synchronization and held-render time. Ready but unclaimed
roots consume slots. Taken, canceled and failed roots release them. Different
owners do not share capacity or serial identity. Missing/ambiguous snapshots
remain unmatched; overflow, orphan lifecycle events and excess occupancy fail
closed. Five new tests cover these conditions; the combined occupancy,
coverage-attribution and lifecycle suite passes **19 tests**.

In earlier `cold-l131`, all 72 queued-blocker occurrences saw both slots
occupied at their readiness snapshot. Twenty occurrences included already-ready
roots; these contributed 40 occupied-slot observations, with ready age median
11.085 ms and maximum 11.516 ms. In `cold-h91`, all 30 queued-blocker
occurrences also had full capacity. These are repeated occurrences, not unique
roots or continuous occupancy measurements. They do not prove queue priority or
CPU residency.

Inspection also rejected two premature interpretations of the QML trace:
Creating entries count recorded ranges, not uniquely identified object
instances; and `MasonryLayout` already supplies final presentation mode through
initial properties. No duplicate-construction or wrong-initial-mode fix was
made on that basis. The exploratory contained-range output includes unrelated
work that can run while asynchronous construction is suspended; temporal
containment alone is not an object-parent relationship.

## Diagnostic change and regression

The copied `RequestedRowIncubation.h` now delivers a pending completion callback
at `afterAnimating` before `startFrame`. The existing zero-delay delivery remains
as a fallback, but is coalesced by the same pending flag and generation check.
A `QPointer` guard prevents scheduling after a callback destroys the queue.
Root limit, priorities, cancellation policy, renderer and visual layouts remain
the same. All five translation units that include the helper were rebuilt
consistently; only this helper's handoff behavior differs from the control.

The fixture manually finishes two incubations without processing the queued
notification, then emits the frame signal. The old helper fails with zero
notifications where one is required. The candidate takes both ready roots and
starts the next pair during that frame, never exceeding two constructed roots.
It does not deliver the stale notification again. Destruction during handoff,
generation cancellation, priority replacement, bounded publication and failure
handling also pass: **9 passes, 0 failures, 0 skips**. The old-helper negative
has **2 passes and 1 expected failure**. An initial fixture compile error was
corrected before this negative run; it is not regression evidence. The existing
invalid-root test still emits the Qt warning about two items being created at
engine destruction; that warning is retained in the test log.

Control host SHA-256:
`e8c9f54a8ebd10a3892c871d7768c186ce7e2ebcf5e664fe7c44f7fcbb874972`.
Candidate host SHA-256:
`912bd855c5aa2c198a56ca77a8a98b9d4c2fa86b34cebbb7979310db6cf8772f`.
Both include the preceding grid deferral, two-root async preparation, stable
source identity, ready-first handoff and directional leading preparation budget.

## Four live captures

Order is control/candidate/candidate/control. All use the initialized-app folder
entry harness, both panels entering Downloads, 192 entries, Masonry density 152,
3840x2160 at actual 160 Hz and sampled window DPR 1.75. Each records 30 seconds
of wheel input. Native foreground is verified throughout sampled input, and the
Computer Use observer is absent before release and after capture. Each starts
with an empty app thumbnail cache and 48 cached frames before input. This is
not an OS-cache-cold or fully warmed-folder claim.

| Case | Arm | Display p99 | Display max | Intervals >9.375 ms | Coverage holds | Target roots started in first 1.5 s | Ready-to-take p95 / max |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| cold-m146 | Control | 6.293 ms | 6.463 ms | 0 | 14 | 90 | 6.563 / 15.970 ms |
| cold-m142 | Candidate | 6.299 ms | 6.419 ms | 0 | 51 | 86 | 2.958 / 8.169 ms |
| cold-m143 | Candidate | 6.290 ms | 6.502 ms | 0 | 2 | 86 | 2.757 / 3.058 ms |
| cold-m144 | Control | 6.284 ms | 12.507 ms | 1 | 42 | 91 | 13.548 / 14.707 ms |

Ready-to-take cohorts include target-owner roots that started during the first
1.5 seconds of input and were subsequently taken. Cohort sizes are retained in
`comparison.json`; failures/cancellations are not silently treated as samples.
All four have zero dropped presents and no orphan or unfinished queue requests.
The final candidate and final control each have one unmatched native motion
commit; the other two have zero. Excluded physical-motion pairs are
4,794 / 4,794 / 4,795 / 4,794. All four admit **zero** physical-error pairs.
Their render-signal maxima are 4.708 / 4.839 / 3.606 / 9.429 ms.

There are no ready-slot observations among the candidate's queued coverage
blockers; the final control has 36, median ready age 11.173 ms. This supports the
specific handoff mechanism. Aggregate holds vary too much to establish a causal
scrolling win. Input currently begins on content readiness rather than at a
fixed elapsed delay: navigation-to-input times are 545.628 / 542.205 / 544.508 /
426.552 ms. Wheel direction also follows observed position. These are material
limitations for comparing early backlog or exact input trajectories.

Two preliminary cases (`cold-m141`, `cold-m145`) failed the disk-headroom gate
before application launch. They were not restarted or included in the table.
Their failure receipts and unlaunched host publications remain available. Closed
diagnostic files were copied to the archive, hash-verified and replaced by exact
symlinks before new cases were used.

## Native/portable checks and cleanup

The candidate passes embedded-only QML import isolation (3 passes), Windows GUI
subsystem verification (3 passes), and the system-DLL import audit. All links
use `ci/msvc_link_response.py`. The first native-mode test link was invoked from
the wrong working directory; its failure log is preserved, and the corrected
link uses the CMake build directory.

The first native-mode test process reported actual DPR 1.0: 3 passes, 2 failures,
2 skips. That is not 175% evidence. A separate native-renderer run with explicit
`QT_SCALE_FACTOR=1.75` asserts effective window DPR 1.75 and passes all seven
selected tests, including moving-scope dirty leaves, camera culling, SVG and
scroll retargeting. The actual named text/raster leaves are checked for physical
origins, dimensions and unit transforms. Both settled captures are pixel-equal
to each other and RGB-equal to the previous grid baseline; small text and thin
raster strokes were visually inspected. This test run is distinct from the four
live 160-Hz captures.

All four capture process trees are closed by exact executable identity. The
normal Go/Qt processes are preserved. The temporary display lease is released
and restores its prior display mode. No new production payload was embedded or
promoted, and no commit or push was made.

## Next work

Use fixed early input and a fixed input schedule to distinguish throughput from
readiness-driven workload changes. Continue investigating row construction and
publication, while preserving the two-root bound; do not raise concurrency based
on this result. Resolve the wheel physical-motion measurement gap, then verify
the full workload, including cold entry, warm touring and middle-button motion.
Zero holds, no refresh spikes and actual physical-motion accuracy remain
unproven. A normal portable rollout is still pending those checks.
