# Delegate parent at construction: lifecycle and performance â€” 2026-11-01

The candidate establishes the final viewport parent and window by QML component completion, but it does not establish a repeatable performance improvement. Both arms miss four refreshes across two foreground-validated captures. Cold-entry synchronous construction remains the dominant large burst. This artifact-only candidate is not promoted.

## Prototype and regression

The control is a frozen copy of current MasonryLayout.cpp. The candidate adds parent=_viewport and visible=false to the initial property map passed to createWithInitialProperties. It preserves the ordinary post-creation ownership, scene-branch assignment and row-finalization path. There is no asynchronous creation, larger pool, rounding change, sampling change, source-format change or Go wire change.

Control host SHA-256: `3e9bfdd15e5074326aeee6e534af8cf2c4163924dc0cbafa8377c119b84e39d1`. Candidate: `9dd09d031e295015bdb9f9f6edb647faf8ed73ff2b54ac9ca72ddefc086be1e6`. Each links one corresponding newly compiled MasonryLayout.cpp object ahead of the same foreground-gated host response graph. No product source or normal embedded payload was replaced.

The new component-completion regression first fails on the unchanged control in Masonry, Grid, Icons, Columns and Details: completedWithParent is false. The final candidate passes all five rows, with the exact viewport parent captured during completion, a non-null window, and invisible status until commit. Final parenting is checked as a viewport descendant because Masonry subsequently moves the root into galleryMasonryRasterBranch. An intermediate test incorrectly required the final direct parent to remain the viewport and failed only that Masonry assertion; the captured completion parent and final ancestry now test the actual two-stage hierarchy. The unchanged control still fails the completion assertion in every mode.

The final candidate run passes 16 QtTest results including fixtures: the five completion rows, five pixel/layout rows (each exercises 13/17/24 px caption sizes), recycling, two parked-bitmap cases and SVG thumbnail resize behavior. Control passes 11 results for the shared presentation/source cases; its five intentional completion-regression failures are retained separately. Embedded-only QML import isolation passes three results. Both PEs pass GUI-subsystem and system-DLL audits. Existing generic-fixture font, PinchArea and minimal-controller QPointF warnings remain.

Actual visible Text/Image leaves are mapped into contentItem at the shown window's DPR 1.75. Origins and local unit vectors are asserted; named small captions and icon leaves are covered. Twelve of fifteen native captures match exactly in RGB and alpha. The three Details captures differ by one header pixel at (1059,39), consistent across the three sizes but still an unresolved exact-pixel difference; do not call all captures identical. The smallest Masonry captions and actual raster icon output were inspected.

## Foreground-validated A/B/B/A captures

All runs use initialized-app cold navigation from a private empty directory into actual Downloads on both panels, 192 rows, Masonry density 152, maximized native 3840Ã—2160 desktop, DPR 1.75, 160 Hz and 30 seconds of rapid wheel input. Private thumbnail caches are empty before QML and navigation. Native foreground and helper absence are checked before release; navigation and every input event retain independent native-foreground flags. No screenshot, build, tests or QML profiler run inside timing. Both arms retain the same native census instrumentation; these are diagnostic comparisons rather than a final uninstrumented rollout audit.

| Case | Arm | Display intervals | p95 ms | p99 ms | Max ms | >9.375 ms | Drops | Coverage holds | Excluded wheel pairs |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|
| cold-c41 | control | 4800 | 6.26370 | 6.30907 | 18.7573 | 2 | 0 | 0 | 4795 |
| cold-c45 | candidate | 4797 | 6.26342 | 6.30210 | 18.7488 | 3 | 0 | 0 | 4792 |
| cold-c43 | candidate | 4800 | 6.26240 | 6.28691 | 12.5070 | 1 | 0 | 0 | 4794 |
| cold-c44 | control | 4797 | 6.26480 | 6.30740 | 25.0035 | 2 | 0 | 0 | 4791 |

The candidate has an early preparation outlier of 12.8687 ms in cold-c45 versus 6.4113/6.0582 ms in the controls; its other run reaches 4.9270 ms. Later commit maxima are lower in these candidates (9.2411/9.7436 ms versus 11.8486/10.1130 ms), but the misses do not improve and the early/pre-input tails do not consistently shrink. No overall speedup is established.

## Keep the cold-entry interval

| Case | Navigationâ†’input ms | Cache frames at input | Pre-input commit elapsed union ms | Largest contained commit ms | First post-nav submitted present displayed ms |
|---|---:|---:|---:|---:|---:|
| cold-c41 | 669.3806 | 41 | 204.2349 | 38.8147 | 154.6873 |
| cold-c45 | 646.9440 | 41 | 188.2552 | 50.7924 | 227.7186 |
| cold-c43 | 653.4961 | 32 | 204.1999 | 39.1036 | 204.5715 |
| cold-c44 | 652.4655 | 12 | 199.0373 | 39.0780 | 193.8027 |

The elapsed unions avoid double-counting nested native commits; they are not CPU residency. The first displayed submission metric does not prove the displayed pixels already contain the new folder. Idle pre-navigation time is excluded. Cache population at input start varies, so different cache-before values are not a causal scrolling improvement. The largest candidate cold-entry commits remain 50.7924 and 39.1036 ms; the control values are 38.8147 and 39.0780 ms.

## Publication failure and storage handling

cold-c42 failed while copying its diagnostic host to the worktree before launch. Free space increased after the failed copy. Delayed release of the previously closed image is a plausible explanation, not a separately traced kernel cause. The 62,914,560-byte partial is hash-preserved on P and was never executed. No raw capture directory or performance result exists for c42. A later launch command found no generated capture script and launched nothing. The normal app stayed alive. The replacement first-candidate capture uses the fresh c45 ID, preserving the A/B/B/A order c41/c45/c43/c44.

Publication now waits for stage size plus 32 MiB of headroom, copies to a .publishing path, verifies SHA-256, then atomically renames it to the executable path. Each case has a unique executable filename. Closed images are copied/hash-verified to P and replaced with per-case symlinks; historical case receipts continue to resolve to their own bytes. The normal payload and other worktrees were not touched.

The bounded display lease timed out after the final capture finished and restored the prior 60 Hz mode without a registry change. The final packet file was saved at Unix 1793546844.0213575; the lease restoration receipt was written at 1793546849.4376295. All recorded input samples remained at 160 Hz. Every owned diagnostic Go/Qt pair is closed; normal PIDs 9252/106980 remain alive. No commit or push occurred.

## Next justified experiment

The parent-before-completion hypothesis is now tested; do not repeat it as a presumed latency fix. The measured large burst still constructs roughly a viewport of real rows synchronously. Next investigate bounded incubation of actual requested rows with their final initial metadata, using the existing QQmlIncubator lifecycle patterns in GalleryComponentHost. This is different from the already inconclusive anonymous-spare reserve: it must avoid constructing a placeholder then rebinding every property.

Before implementation, map synchronous slot acquisition and pool contracts, pending-row ownership and generations, cancellation on reset/delegate/mode/source changes, final geometry and snapshot refresh, readiness publication, and target-leading priority. A partial root must stay invisible and outside ready camera coverage. Preserve required forward coverage and keep every coverage hold visible in results. Do not conceal work by shrinking the viewport or delaying the benchmark's start. Compare pre-input and input windows, and test stale completions, destruction, source changes, all supported media and 175% visual leaves. A staged prototype may be evaluated without promoting it; current evidence does not yet prove its latency or correctness.

The full 192-entry/17-stop warm tour, quarter-height middle-button motion, zero accepted physical wheel-error samples, uninstrumented cadence and portable payload rollout remain open. Stable 160 Hz is not achieved.

> AI assistance has been used to create this output.
