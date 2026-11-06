# Vsync-preserving frame-pacing experiment

## Status

The 160 Hz goal is not achieved. Native configuration/fidelity checks pass, but
representative foreground captures are blocked by Windows refusing to activate
the diagnostic window. No production default, Qt library, embedded payload,
commit, or push was changed.

## Safe experiment selected from the synchronization evidence

Qt begins the graphics frame before item synchronization because item updates
and synchronization callbacks can perform graphics resource work. Moving sync
ahead of frame initialization would violate that ordering. See
[Qt's threaded render loop](https://raw.githubusercontent.com/qt/qtdeclarative/v6.11.1/src/quick/scenegraph/qsgthreadedrenderloop.cpp).

The D3D11 backend supports `QT_D3D_MAX_FRAME_LATENCY=0`, which disables its frame
latency waitable object. It does not mean zero queued frames. The default header
value is 2; nonzero values use the waitable object. See
[Qt's D3D11 implementation](https://raw.githubusercontent.com/qt/qtbase/v6.11.1/src/gui/rhi/qrhid3d11.cpp).

This provides a configuration experiment without altering Qt's synchronization
or resource-access contract. It may change where backpressure occurs and how
many frames are queued. GUI responsiveness, displayed frame timing, and input
latency must all be measured before considering a default change. A hypothesis
about moving a wait is not evidence that the wait or frame spikes disappear.

## Native verification

The test creates a real threaded D3D11 QQuickWindow and examines its actual
`QD3D11SwapChain` and DXGI description on the render thread. It tests values
2, 0 and 1 in separate processes. All nine final QtTest results pass.

For every captured frame:

- Swap interval remains 1.
- The tearing flag is absent; buffer count remains 2.
- Values 1 and 2 have the frame-latency waitable object and DXGI flag 64.
- Value 0 has no waitable object and DXGI flags 0.
- `updatePaintNode()` and the render callback execute inside an initialized
  recording frame.

At actual DPR 1.75, the test checks the visual leaf's scene origin and unit
vectors, red fill, one-physical-pixel white stroke, and neighboring blue pixel.
Resize and hide/show preserve the configuration. Corresponding rendered PNGs
are byte-identical across all three settings. The initial image is 448×224;
resized and restored images are 560×280. The rendered thin stroke was inspected.

The initial fixture missed a stale resize grab: its first post-resize image was
still 448×224. Those initial images, dimensions and results are retained. The
final check waits for a grab matching the current window's physical size before
sampling pixels. All final processes pass this stricter check.

These checks cover the pacing configuration and a native scene, not application
scrolling performance, SVG fidelity, every lifecycle edge case, or frame latency.
All 57 observed native test frames had a different foreground owner; they are
not promoted into foreground performance evidence.

## Capture blocker, independently verified

The desktop is unlocked: the input desktop is `Default`, WTS session state is
active, and the SDK-defined session flag is `WTS_SESSIONSTATE_UNLOCK` (1).
This does not make the diagnostic window the foreground window.

The same condition has prevented representative measurements across three
consecutive goal turns:

1. `real-scroll-overlay-b2-20261029-wheel-masonry` recorded Qt active flags, but
   the native foreground observer recorded ShellExperienceHost in both windows.
2. The explicit-activation attempt in the overlay-focus experiment failed twice
   with `failed to activate captured window`; its handshake timed out.
3. This turn's `real-scroll-frame-pacing-a1-20261029-wheel-masonry` selected the
   unique current diagnostic window, obtained a fresh screenshot, and attempted
   to focus its title bar through the supported Computer Use API. It returned
   `failed to activate captured window`. The activation handshake timed out
   before queuing any scrolling gesture.

The test thresholds and native foreground requirement remain unchanged. There
is no accepted control/candidate scrolling pair for the new pacing option and
no claimed speedup. Further speculative default changes would not resolve this
missing verification. Resume representative captures after F4 can be brought
to the foreground; the user need not remain at the computer during recording.

## Preserved state

The owned diagnostic launcher tree was stopped by its exact executable path.
The normal worktree application, `D:\Code\f4-zoin\f4-zoin.exe`, remains alive
at PID 43936.

`profiler/experiments/masonry-frame-pacing-contract-2026-10-29/manifest.json`
pins 57 compact files and 7 external binaries/objects/headers. Evidence includes
the native test and build scripts, initial and corrected captures/results,
configuration snapshots, guarded application capture scripts, activation failure
and observer records, and the unlocked-desktop check. The capture scripts keep
the same host, bounded-cache settings and overlay fast path in both arms and
vary only the pacing configuration. Native foreground verification is required
before their gesture handshake can be released.
