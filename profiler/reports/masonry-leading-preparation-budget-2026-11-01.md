# Earlier directional preparation — 2026-11-01

The diagnostic candidate requests leading Masonry rows earlier without widening
the metadata window or increasing the two-root construction limit. This is a
partial improvement candidate, not an accepted solution for constant 160 Hz.
Both candidate runs still held the camera; one missed a refresh interval.

## Change under test

The existing planner moves trailing metadata budget ahead only when the input
target extends beyond the normal leading metadata margin. A short target can
therefore leave that budget behind the viewport despite asynchronous
construction requiring substantial lead time.

`RequestedRowLeadingBudget.h` spends the remaining trailing metadata budget in
the requested direction as soon as motion starts. It retains the ordinary
trailing delegate margin. Visible and delegate extents are unchanged; total
metadata extent is unchanged. Zero movement leaves the plan unchanged. The
candidate is applied after the existing `withMotionTarget()` calculation.

Only a diagnostic copy of `MasonryLayoutThumbnails.cpp` calls the helper, gated
by `F4_REQUESTED_ROW_LEADING_BUDGET`. The normal application is unchanged.
Both arms use the same binary, the ready-first prototype, stable source identity,
the same coverage probe, and unchanged construction/nearby-preparation budgets.

Host SHA256: `1d71c606b8a35379d875bf64a5c9540fa95cfc579403cf7dded94ed491a8ed48`.

## A/B/B/A measurements

Each case contains 30 seconds of rapid wheel input after initialized-app cold
folder entry: Downloads, 192 entries, Masonry density 152, 3840×2160 at 160 Hz,
actual DPR 1.75. Runtime input samples confirm entry count, density, DPR and
refresh rate. Native foreground and observer-absence admission passed. Source
acquisition uses an empty private gallery cache; OS file-cache coldness is not
claimed.

| Case | Arm | Held frames | Display p99 ms | Display max ms | Intervals >9.375 ms | Dropped presents |
|---|---|---:|---:|---:|---:|---:|
| cold-i101 | control | 26 | 6.305601 | 6.4875 | 0 | 0 |
| cold-i102 | candidate | 20 | 6.287705 | 6.3799 | 0 | 0 |
| cold-i103 | candidate | 35 | 6.286002 | 12.5086 | 1 | 0 |
| cold-i104 | control | 134 | 6.298807 | 12.4979 | 1 | 0 |

Each display distribution has 4,800 samples. Maximum recorded render spans
were 4.2622, 4.1418, 4.3618 and 3.9388 ms respectively. Elapsed render spans
are not a measurement of GPU residency.

Requests for rows 68–70 began at approximately 67.13 ms in the first control,
16.31 ms in the first candidate, 20.39 ms in the repeated candidate, and
92.76 ms in the repeated control, relative to input start. This verifies the
intended earlier-request mechanism in these captures. It does not establish a
reliable magnitude of perceptual improvement: the controls vary substantially,
and 20/35 candidate holds remain unacceptable.

All cases retained two constructed/unclaimed roots per layout, zero orphan
queue events and zero unfinished requests. All held frames matched geometric
coverage blockers and queue lifetimes. Maximum unique-blocker queue waits were
151.4064, 120.5174, 106.5029 and 362.603 ms respectively. These are elapsed
queue waits, not CPU time. Whole-capture queue percentiles include startup and
background work; the comparison JSON also provides blocker-only distributions.

## Limits retained

- Unmatched native commits: 0, 1, 1, 1. Their complete records remain in each
  motion-analysis JSON. Unmatched presents and display-pair gaps are zero.
- Every motion analysis still excludes 4,794 wheel-curve pairs and admits zero
  physical-error pairs. No optical or full physical-motion acceptance is claimed.
- Cache-before frame count was 48 and pending count zero in every run, but miss
  counts were 66/66/66/48. After-capture frame counts were 117/119/119/122.
  Decode/metadata timing is not identical; do not treat the last control as an
  isolated deterministic scheduler regression.
- Candidate forward/anchor/backward hold counts were 15/3/2 and 24/4/7;
  control counts were 19/5/2 and 90/41/3. No direction was removed from analysis.
- This experiment does not cover the full warm tour, middle-button motion,
  rendered pixel-grid verification, packaged-payload extraction or release
  acceptance. No static leaf geometry was changed by the planning helper.

## Regression checks and provenance

The negative test substitutes the unchanged window for the helper and fails
`shortMotionSpendsTrailingPreparationBudget` (775 versus expected 952 for the
leading window's start). The positive test passes that case, the stationary/
delegate-geometry invariant, and both existing motion-target tests: six QtTest
results including fixtures. The negative control has five passes and one
expected failure. The helper never changes visual or delegate geometry in
those tests and preserves the metadata extent for either direction.

The diagnostic link also passes selected resource-only QML import and Windows
GUI-subsystem checks (three QtTest results each), plus the system-DLL import
audit. Import-test warnings about PinchArea's overridden enabled property,
MS Sans Serif in the fixture, and an undefined QPointF assignment remain; this
is not a rendered-text quality or complete-suite claim.

Raw captures are under
`C:/Users/xs/AppData/Local/Temp/f4-current-cold-motion-20261031/cold-i101` through
`cold-i104`. Large closed files and binaries were copied to the profiling
archive on P:, hash-verified, then replaced with exact symlinks. No samples were
deleted. Comparison and relocation receipts are in
`artifacts/requested-row-leading-budget-20261101`.

All diagnostic processes were closed; the normal app remained alive. The
temporary display lease restored its prior mode. The candidate remains
experimental. Next, measure why row-construction throughput and synchronized
coverage still lag demand even after requests arrive earlier; retain the
bounded workload and coverage guard while doing so.

AI assistance has been used to create this output.
