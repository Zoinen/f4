# Native preview staging integration — 2026-10-21

## Status

Concrete progress, not goal completion. The implementation is in the worktree and the complete native core/host has been rebuilt. The normal packaged `f4-zoin.exe` remains the previous verified build; no payload regeneration, normal-app restart, commit, or push was performed. Constant 160 Hz remains unverified. Do not promote this candidate merely because the full-rebuild counter reaches zero.

## Change and regression

`GallerySceneItem` has an internal publication mask independent of camera culling and retained-scene flags. `GalleryComponentHost` uses it for native scene roots and keeps the Qt hide-reference path for generic roots/software rendering. The uniquely identified `fallbackSourceColor` factory root now uses `GallerySceneItem`, with identical child bindings/geometry.

The checked-in `GalleryComponentHostStagingTest.cpp` initially checked the pending root only. That check passed the broken baseline and was insufficient. The extended assertion observes the outgoing ready surface during synchronization: the unchanged baseline fails because retirement manufactures a zero-opacity node. The integrated fix passes that assertion. The earlier artifact-only renderer probe independently recorded one opacity-triggered full rebuild before, zero after.

New motion regression proves pending and retired content stays hidden even with an active camera, retained-scene state and prepared-scene retention simultaneously enabled. Ready content becomes visible again. Its actual colored leaf has physical origin (14,14) and unit scene vectors at DPR 1.75.

The source-color factory regression exercises the actual image leaf in all five panel modes, asserts a native factory root, checks leaf scene-space origins/unit vectors at 175%, waits for a Ready image, checks its rendered red center and writes rendered images. Inspected Masonry and Details output. The unchanged captions remain outside the modified source-color root.

## Validation

- Full core and host rebuilt normally via CMake (no cross-layout object override).
- Staging fixture: 5 passed / 0 failed / 0 skipped on Direct3D, OpenGL, software.
- Motion fixture: 7 passed / 0 failed / 0 skipped on Direct3D and OpenGL.
- Existing thumbnail replacement/shared-factory plus new source-color factory cases: 5 passed including init/cleanup on Direct3D.
- Actual source-color factory across five modes: 3 passed including init/cleanup on OpenGL.
- Both new diagnostic hosts pass 4 embedded import-isolation checks and the Windows system-DLL audit.
- The complete Session test executable hit the existing >32767-character response-line issue. It was relinked with exactly the same arguments one per line; receipt records the original and replacement response hashes. The small standalone staging target builds normally, without the complete gallery dependency tree.

These are targeted regressions. The known unrelated full-suite failures were not rerun or declared fixed.

## Accepted rapid-scroll evidence

`real-scroll-native-staging-rebuild-rapid-20261021` passes workload/input gates: 187 entries, density 152, DPR 1.75, 160 Hz, all inputs active, 1848 cold / 1871 warm wheel events, capacities below limits. Cold/warm extents match the earlier equivalent rebuild/upload probe: 4075.142857 / 4093.142857 logical pixels. This is an observational comparison to the existing baseline, not a randomized same-session A/B experiment. The ownership traversal is absent from timing hosts.

Across the two 10-second blocks:

| Metric | Reference | Candidate |
| --- | --- | --- |
| Cold full rebuilds | 11 + 13 | 0 + 0 |
| Warm full rebuilds | 8 + 8 | 0 + 0 |
| Warm render-list rebuild flag 5 frames | 583 / 600 | 827 / 815 |
| Warm upload bytes | 95.83 / 99.24 MB | 128.48 / 127.09 MB |
| Warm preparation median | 0.628 / 0.684 ms | 0.909 / 0.870 ms |
| Warm preparation p99 | 2.064 / 2.207 ms | 1.887 / 2.012 ms |
| Warm preparation maximum | 3.059 / 3.138 ms | 2.309 / 3.360 ms |
| Cold preparation maximum | 5.619 / 3.998 ms | 4.749 / 2.858 ms |

All first full-rebuild causes were checked; none appeared in the candidate measurement windows. The extra upload work is ordinary merged text-mask, smooth-color, texture and shader-effect batches, not newly discovered SVG uploads in the warm blocks. Total upload calls rise as well. The candidate reduces the identified tail mechanism but increases steady work. This is not a clean performance win and is not yet promoted.

Evidence: `native-staging-rebuild-comparison.json`, `native-staging-rebuild-analysis.json`, `native-staging-rebuild-flags.json`, `native-staging-upload-materials.json`, original raw packets/PresentMon CSVs, build and focus receipts.

## Rejected minimal captures and interference

The minimal rapid capture delivered only 1261 wheel events, below the unchanged 1500 requirement; all its recorded inputs were active. Do not use it as qualifying rapid performance evidence or relax the gate. The exact reason for its low event rate is not established.

Two minimal middle captures fail focus checks, despite using the required quarter-height distance 296.5 for a 1186-high window. They are excluded from performance claims. A bounded foreground-owner observer identifies an Android plugin executable and a Git executable among other foreground owners. This proves interference, not which command or root cause created each window. Do not terminate unrelated workloads or change installed plugins based solely on that observation.

Only the exact diagnostic launcher's process tree was stopped. The normal worktree app remains alive and responsive. Standing user authorization permits future captures without readiness questions.

## Next actions

1. Investigate why ordinary render-list rebuilds/merged uploads become more frequent after eliminating full rebuilds. Probe invalidation reasons and root/order-span behavior; do not assume an owner cache or globally suppressed opacity is safe.
2. Refine or reject the candidate based on total frame cost, including steady work. Preserve the failing retirement regression and correct pixel geometry.
3. Obtain qualifying minimal cold/warm rapid and middle captures without interference; retain 160 Hz, focus, input-count, capacity, displayed-gap and 2-physical-pixel motion gates.
4. Independently address remaining cold SVG uploads and presentation/motion tails.
5. Run portable packaging/import/extraction gates and restart the worktree app only after a useful candidate is validated. The full goal is still active.
