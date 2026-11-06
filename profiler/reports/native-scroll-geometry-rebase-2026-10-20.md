# Preserve native scrolling across Masonry geometry updates

The repeated cold geometry cancellations are fixed in the experimental native
controller. Constant 160 Hz across the application remains unverified; native
scrolling is still disabled by default in the ordinary worktree app.

## Reproduction and implementation

`nativeScrollSurvivesMetadataRewrap` first reproduced the cancellation on both
D3D11 and OpenGL, at actual window DPR 1.75. A cached decoder-metadata batch
changed aspect ratios during native scrolling. Both runs failed specifically
because the active native trajectory had been cancelled, after verifying that
the layout geometry had changed. Logs are `metadata-rewrap-before-*.log`.

`MasonryLayout::rewrap()` now brackets the complete geometry transaction with
`rewrapAboutToBegin` and `rewrapFinished`, including extent and viewport-anchor
restoration. Nested rewraps share the outer boundary. The controller adopts the
committed native pose while the old geometry still exists, then translates the
trajectory by the layout's restored anchor offset at transaction completion.
It preserves the native job and animation clock. The endpoint is clamped to
the new extent and published to both QML's planned target and bounded delegate
preparation.

An accumulated geometry offset maps render-job coordinates to the current
GUI layout. Render publication and GUI rebasing share the state mutex; queued
GUI callbacks therefore consume current-space poses rather than restoring old
coordinates. Render synchronization translates the running segment and rebuilds
its coverage/culling snapshot. Changes outside a rewrap transaction still cancel
safely; source/model replacement, mode, resize, DPR and hiding guards remain.

The extended passing regression starts well down the folder, stalls GUI work
for 25 ms while the native camera advances, then changes aspect ratios above
the viewport. It asserts a leading entry's viewport phase, adoption of the
advanced native pose, one job with no false running-state change, the rebased
endpoint and successful completion. At rest, each named caption leaf is checked
for whole physical-pixel scene origin and identity unit-vector transform.
The rendered native result equals the ordinary GUI reference exactly. The
small captions in the saved 175% capture were also inspected.

## Repeated live measurement

The same diagnostic executable ran native, GUI, native repeat, GUI repeat.
Each process supplied two 10-second cold and two 10-second warm blocks:
80 seconds per path. Conditions match the direct-controller baseline report:
Downloads, 186 entries, Masonry density 152, 160 Hz, DPR 1.75, 2194 x 1186
logical window, wheel events every 30 ms, bouncing direction, default 5000 us
GC budget, minimal recorder and diagnostic pixmap-cache cap of 2 MiB.
No build, test, UI Automation observation or analysis ran during input windows.

| Capture | Cold long intervals, blocks 1 / 2 | Warm, blocks 1 / 2 |
| --- | --- | --- |
| Native | 0 / 0 | 0 / 0 |
| GUI | 2 / 0 | 1 / 2 |
| Native repeat | 0 / 0 | 0 / 0 |
| GUI repeat | 2 / 1 | 1 / 1 |

Long means a displayed interval greater than 9.375 ms, rather than a count of
lost refreshes. Native has zero such intervals over 80 measured seconds; GUI
has ten. Native block p99 values range from 6.276 to 6.405 ms and the worst
displayed interval is 8.550 ms. GUI's worst is 12.516 ms. This bounded workload
is encouraging evidence; it is not proof of invariant 6.25 ms timing, nor of
the full goal across gestures, folders, renderers or operating conditions.

Both native processes used one job per input block: two cumulative jobs after
cold capture, four after warm. No cancellation event was recorded. The first
cold block exercised repeated real rewraps on job 1. All measured native blocks
had zero coverage-limited or unready-anchor frames. This supports retaining
existing preparation bounds. Native commit gaps still reached 9.016 ms, so
the controller trace also does not establish perfect cadence.

The probe remains a source-instrumented copy of the production controller,
with a pre-touched fixed buffer and post-measurement JSON export. Rebase events
were added to the GUI-side event list. Its private-state addition and per-frame
scope recording have nonzero overhead. Results describe that instrumented
host, not exact uninstrumented production latency. Per-stage SHA sidecars and
PresentMon PID validation identify the actual processes and source revisions.

## Verification and remaining work

- D3D11 and OpenGL metadata regressions pass with zero skips.
- Existing retarget/stop/fallback checks pass on D3D11, OpenGL and software.
- The rebuilt static host passes system-DLL import auditing and the embedded
  QML-only `qmlImportsWithoutInstalledQt` regression.
- The compressed payload was regenerated and embedded in a `CGO_ENABLED=0`
  launcher; embedded-host tests passed using the system Go cache. The worktree
  app was restarted directly with `--gui=qt`, with no host override. Its
  responding Qt child has SHA-256
  `caa5eb50eaeef6a44d6be5097516f737884ba97f84bee1b5a3a6a957c16d8662`,
  matching the freshly built host. The launcher is
  `1cec8c15931476535ff3e27c63a24a47557295831b2e6a36359e7644b5a76936`.
- Normal native scrolling remains off; no default enablement or 160 Hz completion
  claim is made. The earlier unrelated Details pixel-contract failure is not
  resolved by this change and no full-suite pass is claimed.

Next, verify multiple geometry transactions and endpoint shrink while the
camera leads GUI state, mixed SVG/raster culling during those transactions,
cursor/chrome and sticky-header synchronization, and middle-button scrolling.
Attribute the remaining cadence variation using the production controller;
repeat broader workload measurements before default rollout.

Evidence is preserved separately from the immutable baseline under
`profiler/experiments/native-scroll-geometry-rebase-2026-10-20/manifest.json`.
Capture prefixes are `real-scroll-rebase`, `real-scroll-rebase-gui`,
`real-scroll-rebase-repeat` and `real-scroll-rebase-gui-repeat`.
No commit or push was made.

The initial launch observer searched only immediate children and timed out.
The existing launcher had relaunched a same-path Go child, which owned the Qt
process. The same process tree was inspected and verified without restarting;
the observer now also follows that relaunch. A preceding backup-identity check
stopped before terminating the old app; a separate matching backup was then
created for this deployment. Neither observation/setup issue is a measured
scrolling sample.

> AI assistance has been used to create this output.
