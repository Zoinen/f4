# Preserve running scroll segments for identical wheel requests

## Finding

Reanalysis of the final finite-clock capture separates actual retargets from
ordinary GUI synchronization. Changing the base translation alone does not
invalidate a movement comparison when `renderedY + visualY` stays stable. The
expanded comparison covers 1263–1264 pairs per 10-second window, rather than
roughly 470, and still has motion-error p95 below 0.11 physical pixels.

Across four windows, 85–86 retargets per window have an identical destination.
They still reset the OutSine curve's origin and deadline. Reconstructed curve
poses remain exactly continuous, but the analytic instantaneous speed drops by
23.27% at the median and 33.18% at p95. These are calculated derivative changes,
not measured display intervals. Scaled to one 6.25ms refresh, their p95 speed
discontinuity is equivalent to 9.30–11.69 physical pixels of movement. Actual
changed-target requests also have speed discontinuities; those remain separate.

## Change and regression

The native retarget method now returns without restarting the segment when
destination, duration and easing all match the current command. It also avoids
redundant viewport preparation for that request. A changed destination, duration
or easing retains the existing retarget behavior. Continuous-mode exit and
native eligibility checks retain their existing order.

The new hardware regression repeatedly resends the same 200ms command every
30ms. Before the fix, it remains running at the test's 600ms cutoff (observed
615/619ms), failing on OpenGL/D3D11. Afterward it finishes at the polling
observation times of 220/247ms. A separate step in the same test changes the
duration with an identical destination and proves that request still takes
effect. Existing changed-target job-retention and first-handoff tests also pass.

## Live capture

Downloads186, Masonry152, DPR1.75, 160Hz, wheel requests every 30 ms with boundary
reversals. Two 10-second windows in each cold/warm stage: 40 seconds. The baseline
is the preceding finite-clock capture, reanalyzed without modifying its evidence.
No builds, tests, analysis or UIA observation run during input windows. Bounded
renderer/material probes remain enabled; the general frame recorder is disabled.

| Window | Identical-target resets before | After | Display misses after | Max display interval ms |
|---|---:|---:|---:|---:|
| Cold 0 | 85 | 0 | 0 | 6.3820 |
| Cold 1 | 86 | 0 | 0 | 6.3467 |
| Warm 0 | 86 | 0 | 0 | 6.4342 |
| Warm 1 | 86 | 0 | 0 | 6.4061 |

The zero-identical-retarget gate fails on the baseline and passes afterward.
Reconstructed position residual at changed-target seams is zero in these data;
there are no excluded geometry/curve transitions in the seam analysis. The
changed-target derivative discontinuities remain, with median equivalents around
3.8–4.0px per refresh and p95 around 9.5–10.7px. They are not fixed by this change.

The expanded displayed-motion comparison covers 1297–1303 unchanged-segment
pairs/window after the fix. Its p95 error is 0.114–0.137 physical pixels, maximum
1.704px; zero presentation brackets are missing. Its p95<=0.5px gate passes,
also requiring zero missed refreshes. The different boundary traversal pattern
means these p95 values are not a statistically isolated speedup comparison.
The concrete improvement is removal of unnecessary speed resets and postponed
completion, not a claim that every remaining source of roughness is solved.

## Verification and deployment

- Primary native suite: 18 passed per D3D11/OpenGL, none failed/skipped.
- Retarget/handoff suite: 4 passed per backend, none failed/skipped.
- Consumer linked without the optional precise clock: 5 passed per backend,
  including identical-command deadline and changed-target/handoff behavior.
- Existing 175% resting-leaf scene-origin/unit-transform and exact restored-render
  comparisons pass. No static layout was changed.
- Portable QML import isolation, system-DLL audit, embedded-host extraction tests
  and CGO-disabled packaging/build pass. Existing duplicate-plugin object warnings
  remain. Source diff checks pass with the existing PIXEL_GRID.md line-ending warning.

The worktree launcher was replaced after the package passed. Its responsive
extracted Qt host matches the rebuilt host; host and frame-latency overrides are
absent. The canonical checkout was untouched. Nothing was committed or pushed.

- Launcher SHA256: `239efedb1cbe135d50b972c9771885279472971ed982a85eb3cc3877a6a20e38`
- Host SHA256: `c27b27dad94a944219a9716131e71ff184b7773ba63a6700dd804512226fa81b`

Verified compressed evidence is indexed by
`../experiments/identical-wheel-retargets-2026-10-20/manifest.json`.

The 160 Hz goal remains active. Next investigate velocity continuity when the
destination genuinely changes, while preserving bounded response, reversal
behavior and the existing preparation limits. Startup and broader workload/view
coverage also remain unproven by this specific capture.
