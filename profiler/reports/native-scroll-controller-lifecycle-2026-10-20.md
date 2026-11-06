# Native scroll controller lifecycle

`GalleryScrollAnimation` replaces the diagnostic hook's role in the panel
motion API. It is now the panel's animation object, but its native path is
disabled by default. Normal operation uses its inherited GUI animation path.
The persistent native path has lifecycle and rendered regressions; it has not
yet had its own live performance capture. Earlier native-camera timing results
must not be attributed to this new controller. Constant 160 Hz remains unproven.

## Implementation

The controller derives from `QQuickNumberAnimation`, preserving the existing
typed references, target, property, from/to, easing, duration and running state
used by panel controllers. Their start entry points call
`GalleryPanelMotion.restartScroll()`, which routes to `retarget()`.

When explicitly enabled and eligible for native Masonry motion, one persistent
render-thread job receives target commands without restarting. The job owns the
shared surface matrix correction, individual-cell coverage range, delegate
culling, SVG presentation gates and branch clips. Initial and subsequent target
preparation use the existing bounded metadata look-ahead. GUI position updates
are coalesced through one pending queued callback. The diagnostic dummy target
and its property interpolator are not used by this controller.

The native transition returns an explicit `QQuickAnimatorProxyJob`; the owner
reports GUI threading because the returned job is already a proxy. This fixes
job-thread ownership at construction instead of reevaluating mutable layout
eligibility after initial cells have been prepared. Qt normally queries the
threading model after `transition()` returns. See the
[Qt 6.11.1 animation implementation](https://github.com/qt/qtdeclarative/blob/v6.11.1/src/quick/util/qquickanimation.cpp#L114-L134).

This does not eliminate Qt's GUI animation-driver ticks: its proxy continues
to track the native job. It removes the extra logical property animation and
per-input job restart. The
[Qt proxy implementation](https://github.com/qt/qtdeclarative/blob/v6.11.1/src/quick/util/qquickanimatorjob.cpp#L45-L77)
describes that GUI/native relationship. No performance benefit is claimed until
the real controller has been measured.

The ordinary GUI path remains available when native motion is disabled,
the renderer is software, the mode is not Masonry, or the target is ineligible.
There is no user-facing setting or wire-protocol change.

## Stop handoff regression

The first implementation committed the latest native position from a C++
`runningChanged(false)` observer. A repeated OpenGL test showed that this was
too late for the panel's persistence path: the position was 182.929420 at return
from `stop()`, then changed back to 167.292952 after queued work ran.

The controller now completes the handoff from the native job's GUI-thread
`writeBack()` step, before the proxy sends its stop notification. It freezes
the camera, commits the latest position, clears its owner-scoped layout state,
and schedules scene cleanup before the QML handler persists the position.
The source-reset cancellation path suppresses committing the outgoing pose.
Qt calls native job write-back before cancellation in its
[proxy stop implementation](https://github.com/qt/qtdeclarative/blob/v6.11.1/src/quick/util/qquickanimatorjob.cpp#L125-L145).

The regression explicitly stalls the GUI thread for 25 ms before stopping, so
native motion can lead its GUI shadow. The position must remain unchanged after
another 100 ms of event processing. It passes on D3D11 and OpenGL after the fix.
The old failure is preserved in `scroll-controller-stop-race-before-opengl.log`.

An earlier test also caught duplicate completion: one natural completion
produced two `finished` notifications because the controller emitted an extra
one. Removing that emission leaves one notification in the tested lifecycle.
The initial failure is preserved in `scroll-controller-before-d3d11.log`.

## Cleanup and boundaries

Cleanup restores branch clip rectangles as well as the shared surface
transform and ordinary culling. Clearing layout camera state checks the owner,
so old cleanup cannot clear a replacement camera's state. Late GUI callbacks
check whether their position update replaced/cancelled the active state before
trying to finish it.

Resize, mode/model changes, catalog reset, extent or layout-band changes,
DPR changes, window reassignment and hiding currently cancel native motion.
Cancellation is a safety boundary, not smooth transactional rebasing. The
controller still needs that rebasing for an uninterrupted user experience
during metadata-driven geometry changes.

The retarget path does not call `surface->update()` after preparing cells.
The persistent render job already advances frames, and preparation can cancel
the owner; dereferencing its state afterward was unnecessary and unsafe.

## Verification

The lifecycle tests use the panel's actual QML-created controller and its
`restartScroll()` entry point. They cover initial start, forward retarget,
reversal, one native job across retargets, natural completion, explicit stop
after the GUI stall, resize/mode cancellation, Grid GUI fallback, software
fallback with native requested, and catalog replacement without later pose
restoration. Three consecutive runs passed after the stop fix on D3D11,
OpenGL and software. Each hardware run contains two behavioral cases plus
setup/cleanup (four checks); software runs the fallback case plus setup/cleanup
(three checks). There are no skips or failures in those successful runs.
The final explicit-proxy version also passes the same set.

At DPR 1.75, the final target is 20.13 logical pixels, deliberately off the
physical grid. Every visible named Masonry caption in the regression has a
whole physical-pixel scene origin and unit scene transforms after settling.
The rendered native-rest image exactly matches a reference made by moving
the GUI layout away and back to the same position without native motion.
The capture was inspected, including the small file captions.

The selected preexisting native hit-testing/raster/SVG camera regressions and
the neighboring scene-order, nearby preparation and pixel-grid set pass on
both hardware backends. The fresh static import-isolation test passes with
three checks and no failures or skips. The rebuilt normal host passes the
Windows system-DLL import audit. Embedded-host extraction tests run with
`CGO_ENABLED=0`, `GOWORK=off` and the system Go cache.

This is selected coverage, not a full-suite or all-platform release claim.
The previously recorded Details visual-contract failure remains outside this
work. Tests do not yet establish native pause/resume, animation groups/loops,
sticky headers, cursor/chrome synchronization, smooth layout/source rebasing,
or complete hidden-window and scene-graph lifecycle behavior.

## Packaged worktree launch

The payload was regenerated and embedded into the worktree's Go launcher.
`D:\Code\f4-zoin\f4-zoin.exe --gui=qt` is running with launcher PID 81936
and Qt child PID 53956, without `F4_EXT_UI_PATH` or sibling-host routing.
The Qt window has a nonzero native handle and is responding. Its extracted
host SHA256 matches the rebuilt host:
`8703a1de3e458e719f28c9b9a6077c9078f6c8e0309f0e423a973acd51fe6cd5`.
Launcher SHA256 is
`6b04994c49e98b830bea168b92a2ad8226aa3f6056cdedf7104a759ea4553cd3`.
Native motion remains disabled by default in this running build.

The initial 30-second child-process observation timed out. Subsequent process
inspection found the expected child; it was verified in place without another
launch. This is a successful eventual-launch check, not a startup-latency claim.
The previous launcher is preserved in
`artifacts/f4-zoin-before-controller-20261020.exe`. The runtime receipt is
`artifacts/controller-live-identity.json`.

## Remaining work

Measure this controller directly in the real app, without enabling the old
`RtPanelHook`. Capture display timing and native pose continuity, initial
preparation, cancellation/restart counts and cold metadata behavior. Complete
the lifecycle and chrome work above, then enable native motion only after
those checks and repeated 160 Hz captures pass. The default GUI path keeps
the current app usable while this proceeds.

Evidence, source snapshots and executable identities are indexed under
`profiler/experiments/native-scroll-controller-lifecycle-2026-10-20/manifest.json`.
Executables and libraries are hash-only records; no build outputs are committed.

> AI assistance has been used to create this output.
