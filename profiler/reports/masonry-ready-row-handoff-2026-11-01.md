# Masonry ready-row handoff — 2026-11-01

Ready-first acquisition eliminates the observed pool-driven discard of completed rows. In two foreground-validated rapid-wheel captures, coverage-held samples fall to 35/22 from 77/117 in the controls. One refresh miss occurs in each arm across its two runs. This is progress, but the remaining holds and unverified physical-motion workload prevent a constant-160-Hz claim or normal-app rollout.

## Change and regression

`GalleryDelegatePool` gains an acquisition overload accepting an unclaimed prepared item and its component key. It registers and uses that item before considering generic free slots. Null retains ordinary pool/factory acquisition. Registration preserves the component identity used by retirement, trimming and subsequent reuse. QObject/visual ownership remains with the caller; the pool does not publish visibility.

Two new pool regressions first fail with the old acquisition behavior: the free spare wins over the ready row, and a ready row with no factory is rejected. With the implementation in place, both pass alongside the existing retention, spare-bound and native-component-identity cases. The existing target-leading fast-scroll materialization regression also passes. The selected pool run has eight QtTest results including setup/cleanup, zero failures and no skips.

The diagnostic Masonry bridge checks whether a requested row is ready, validates its current source identity, takes it, establishes the existing viewport ownership/scene-branch watchers, and passes it to the pool overload. Normal identity/visual/geometry commit still precedes unmasking in finalization. An unrelated/stale identity cannot bypass `takeReady()` validation. Pending duplicates satisfied by a spare can still be canceled, but already-completed matching work is preserved.

Identity calculation is shared between ordinary factory acquisition and the ready-first path to prevent different keys for the same row. The source-identity correction, two-root per-panel construction bound, existing target-leading/metadata windows, static layout and pixel-rounding behavior are unchanged. Readiness checks avoid constructing the full identity when the row has no ready request.

The generic pool overload and its tests are in the working submodule sources. The Masonry routing remains in diagnostic copies, gated by `F4_REQUESTED_ROW_READY_FIRST`; it is not wired into the normal embedded application. Previously dirty component-key getter changes were preserved.

## Recycling coverage repaired

The existing geometry-before-visibility test previously navigated after fixed 50 ms waits and could observe no recycled identity changes under asynchronous construction. It now waits for actual visible row assignments before installing observers and before each navigation step. Its strict `reopened > 0` and premature-geometry assertions remain. The native run passes with actual recycling exercised; no performance conclusion is drawn from these functional waits.

The selected native regression group has 15 QtTest results including fixtures, zero failures and no skips: source-identity contracts, presentation-only delta preservation, suspended reset, reversal/mode changes, five-mode physical-pixel checks, parked bitmap cases, actual recycled identity/geometry ordering and SVG resize behavior. This is a selected regression group, not the entire repository test suite.

## Foreground A/B/B/A capture

Both arms use the same diagnostic binary and trace graph, asynchronous construction and stable source identity. Only ready-first differs. Workload: actual Downloads, 192 rows, both panels in Masonry density 152, native 3840×2160, DPR 1.75, 160 Hz, initialized-app cold entry followed by 30 seconds of rapid wheel input. Private thumbnail caches are empty before QML/navigation; OS file-cache coldness is not claimed. Computer Use is disconnected before timing, and native foreground checks pass at admission, navigation and every sampled wheel event. The actual GC budget is verified at 5000 μs.

Host SHA-256: `84beaa7cdd78c60fb02ac172e06bf0a79d1da5141ebcaca61bcee316db52b434`.

| Case | Ready-first | Display intervals | p99 ms | Max ms | >9.375 ms | Drops | Coverage-held samples | Ready roots discarded by pool |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| cold-g81 | off | 4800 | 6.28981 | 6.5040 | 0 | 0 | 77 | 23 |
| cold-g82 | on | 4800 | 6.30051 | 12.5183 | 1 | 0 | 35 | 0 |
| cold-g83 | on | 4800 | 6.30650 | 6.5848 | 0 | 0 | 22 | 0 |
| cold-g84 | off | 4800 | 6.29521 | 12.4952 | 1 | 0 | 117 | 45 |

Ready-first removes the measured discard mechanism in both runs and reduces coverage-held samples in both comparisons. Display p99 is slightly higher in the candidates, and total missed-refresh count is equal across arms. Do not describe this as an across-the-board cadence improvement or as smooth motion acceptance. Held samples are not counts of separate visible pauses.

All queue traces have zero overflow, orphan events and unfinished lifetimes. Both owners remain bounded at two constructing/unclaimed-ready roots. Ready-to-take maximum wait drops from 40.9848/40.6436 ms in controls to 15.1717/18.7124 ms in candidates. This is elapsed waiting, not CPU execution time. Pending (not ready) requests still have pool-satisfied cancellations, and bulk/identity cancellations remain.

## Cold-entry interval remains visible

| Case | Navigation→input ms | Largest contained pre-input commit ms | Pre-input commit elapsed union ms |
| --- | ---: | ---: | ---: |
| cold-g81 | 439.5293 | 4.2045 | 63.8565 |
| cold-g82 | 545.8844 | 4.1261 | 60.5724 |
| cold-g83 | 431.9208 | 4.7668 | 59.0670 |
| cold-g84 | 544.5332 | 4.1124 | 64.5979 |

The large synchronous cold-entry commits do not return in this experiment. Input is still readiness-driven, with 48 cached frames and zero pending cache reads in each case; that is not full-folder warmth. Fixed-early-input behavior is outstanding. All pre-input attribution, excluded wheel pairs, held samples and raw presentation data are retained.

The final held sample occurs at 1336.5386/1346.0601 ms after input start in candidates, versus 874.1979/1683.6612 ms in controls. A diagnostic grouping using a 12.5 ms gap is retained separately; its groups are not asserted to equal perceptual pause episodes. The next investigation can focus on the initial traversal without assuming later workload acceptance.

## Rendering and portable-host gates

At actual DPR 1.75, named Text/Image leaves in five modes at 13/17/24 caption sizes pass scene-origin and unit-vector checks. Rendered Segoe UI captions contain 323/566/1250 qualifying glyph pixels in the first-caption region; the smallest native caption and raster icon were inspected. These checks preserve the earlier correction that geometry alone must not count an unrendered font as visual proof. No new rounding or UI layout arithmetic was introduced.

The exact diagnostic host passes embedded-only `qmlImportsWithoutInstalledQt`, actual-host `executableUsesWindowsGuiSubsystem` and the Windows system-DLL audit. Links use `ci/msvc_link_response.py`. No packaged payload/Go extraction release was built, so this is not a completed portable release.

Closed test/host binaries were hash-verified into P: archives and replaced by symlinks to preserve evidence and disk headroom. Negative-test provenance retains the compiled pre-fix pool copy even though the live source now contains the implementation. All four owned capture pairs are closed, the temporary display lease restored its prior 60 Hz mode without registry changes, and normal pair 9252/106980 remains alive. No commit or push occurred.

## Remaining work

Trace the first ~1.35 seconds of candidate input against requested-row start/ready/take/publication and render-scene coverage. Determine which needed rows are still queued, unmaterialized, constructing, waiting for claim, or not yet present in the prepared scene. Preserve target-leading coverage checks and the two-root bound while identifying the next bottleneck; do not hide stalls by weakening camera readiness.

The wheel analyzer still excludes wheel-curve pairs and admits no physical-motion error samples. Fixed-early entry, the full-folder warm tour, quarter-height middle scrolling, physical-error acceptance and a normal embedded-payload rollout remain required. The full goal is active and unachieved.

Sources, tests, captured images, commands and raw-data hashes are frozen beside [the experiment manifest](../experiments/masonry-ready-row-handoff-2026-11-01/manifest.json). See [the preceding queue/source-identity findings](masonry-row-queue-and-source-identity-2026-11-01.md) for the discarded-work diagnosis.

> AI assistance has been used to create this output.
