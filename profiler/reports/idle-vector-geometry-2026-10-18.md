# Readiness investigation and idle vector geometry

## Readiness hypothesis tested

The preceding attribution identified costly geometry uploads from surfaces
already marked ready. A possible explanation was asynchronous Shape work
arriving after the native gate's latched readiness. Temporary diagnostics
recorded CPU transitions, native readiness and scene geometry counts in the
actual Downloads workload.

The first window started before source assignment and was inconclusive for
most surfaces. The corrected window resets on source assignment and records
40 render observations after CPU completion. It observes 11 loaded surfaces,
440 observations total. All have nonzero geometry at first native readiness;
none changes node/vertex count after readiness during that window, and none
reports a CPU transition with nativeReady already true. This does not support
a readiness change for this reproduction. It does not prove vertex bytes or
all later animation/resize cases are unchanged, so it is not a universal
readiness correctness proof.

Evidence is local artifacts/readiness-confirm-transitions.log and
readiness-confirm-frames.json. These diagnostic runs write logs and request
extra observations; their timing is not used as clean cadence evidence. The
temporary C++ instrumentation is restored completely before the final build.
Its source is retained locally as GalleryVectorUploadGate-readiness-trace.cpp.

## Reproduced cache geometry reset and fix

GalleryVectorSceneCache previously bound idle dimensions to 64 by 64, DPR to
1, and crop/checkerboard flags to false. Releasing a 160 by 80 surface changed
these values even though its compiled source stayed cached. Reacquisition
changed them back.

The regression fails before the fix with actual QSizeF(64x64) versus expected
QSizeF(160x80). The cache now snapshots those presentation values before
detaching its host and uses them while idle. Active hosts still supply live
values; no rounding or per-frame traversal is added. The existing scene/lease
bounds and cancellation/revision behavior remain intact.

The expanded reuse regression verifies zero width/height changes across
release/reacquisition, preserved DPR and enabled crop/checkerboard flags,
subsequent live flag updates, surface/child identity, exact pixel equality,
and the actual surface leaf's whole physical origin/unit transform at DPR
1.75. The rendered cache-reuse capture was inspected. The resize capture's
small caption and thin blue outline were also inspected; the caption is
present and legible. This is local rendering evidence for these fixtures,
not a claim about every authored SVG.

## Verification

- Before: idle-vector-geometry-before-175.txt, 2 passes and 1 expected failure.
- Focused SVG selection with experimental gate: 26 passes, no failures in
  idle-vector-geometry-after-175.txt.
- Final expanded cache test and both swap paths with explicit gate availability:
  5 passes, no failures in idle-vector-geometry-final-175.txt.
- The same final selection on normal Qt, explicitly requiring capability
  absence: 5 passes, no failures in idle-vector-geometry-normal-175.txt.
- Complete focused SVG selection on normal Qt: 26 passes, no failures in
  idle-vector-normal-all-175.txt. The selection includes all five panel modes,
  surface resize, mixed source/revision/cancellation and swap behavior.
- Import isolation plus compiled-host loading: 4 passes, no failures in
  idle-portable-imports.txt.
- Windows DLL audit succeeds; no application-owned DLL or dynamic MSVC runtime
  remains (idle-portable-audit.log).
- Generated payload, extraction/materialization and embedded lookup Go tests
  pass with CGO_ENABLED=0, GOWORK=off and the system Go cache.

The full project suite is not claimed green. The parent native fixture still
contains experimental admission/retention rows requiring their corresponding
prototype libraries; they are not evidence for the normal renderer recipe.

## Clean scrolling results

A is the previous normal Qt control; B adds idle geometry preservation.
Neither uses the experimental upload-gate library. The guarded workload is
Downloads, 184 entries, density 152, extent 4161.142857, active window, DPR
1.75 and 160 Hz. Wheel events are 120 units every 30 ms, about four seconds
in each direction. No diagnostic logs, detailed observer, build, test or
sampler runs during the captures. State is restored before each fresh process.
Caption measurement count remains zero.

| Run | Down display p99 ms | Down maximum ms | Down intervals >9.375 ms | Up p99 ms | Up intervals >9.375 ms |
| --- | ---: | ---: | ---: | ---: | ---: |
| A1 | 24.9849 | 43.7237 | 44 | 12.5110 | 24 |
| B1 | 18.7257 | 25.0018 | 19 | 6.3986 | 6 |
| B2 | 31.2345 | 43.7816 | 48 | 12.5136 | 24 |
| A2 | 18.7575 | 43.7935 | 32 | 12.5051 | 14 |

The first pair improves and the reverse pair regresses. There is no reliable
scrolling gain established by these data. The change removes a reproduced
unnecessary geometry transition; it does not fix constant 160 Hz. No run is
excluded based on an unverified environmental explanation. PresentMon
display-change intervals are the headline, not the harness's native periods.
Local stems are idle-geometry-{a1,b1,b2,a2}-wheel-masonry; the compact combined
data is artifacts/idle-geometry-summary.json.

## Portable worktree rollout

The normal host links the integrated stable-interval Qt library, verified in
the host response file. The experimental admission and combined-retention
patches remain disabled. The host's resources/payload are regenerated and the
staged Go launcher is verified before terminating only this worktree's process
tree and replacing D:/Code/f4-zoin/f4-zoin.exe. The canonical checkout is not
touched. The new executable launches directly with --gui=qt and all diagnostic
host, tracing, backend, scale and order-padding overrides cleared.

New host SHA256:
7e630abdf37014ee1feb1ad023cfa4de4d57e1a217ed366854c2ab9b1f7e8568.

New launcher SHA256:
01086cab1fc4f83d6d61f3732cee217fccab2fe56464401f8782c28aa47db66d.

The running extracted child matches that exact new host hash, its title is
f4 [zoin_branch/cached-pixel-grid], and startup stderr is empty. Evidence is
artifacts/idle-geometry-normal-identity.json. No commit or push is made.
Cross-platform release gates have not been run in this Windows turn.

The remaining work is to explain and eliminate invalidation/recreation of
ready geometry and critical-path driver/resource stalls without hiding
incomplete images or allowing unbounded image latency. The 160 Hz objective
remains active and unfinished.
