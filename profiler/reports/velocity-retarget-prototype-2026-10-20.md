# Velocity-preserving retarget prototype and startup outlier

## Prototype

The preceding capture contains a speed discontinuity whenever a genuinely new
destination restarts the OutSine segment. A local interpolation probe evaluates
the captured outgoing derivative and remaining distance. It does not claim a
closed-loop UI replay. Unrestricted cubic Hermite interpolation overshoots for
negative initial slopes or sufficiently large approach speeds; synthetic and
captured reversal cases demonstrate this.

The diagnostic-only controller uses a cubic with preserved incoming speed,
zero final speed and the existing duration when its normalized initial slope
`r = velocity * duration / distance` is in [0,3]. Its normalized position is
`3p² - 2p³ + r*p*(1-p)²`. The derivative is nonnegative throughout [0,1]
for this slope range, so it remains between its endpoints. Other slopes retain
the existing behavior. Initial jobs and non-OutSine commands are unchanged.

The cubic is expressed through Qt's BezierSpline with linear time coordinates.
This keeps the existing geometry-rebase inversion in the prototype. Production
integration should still consider a direct polynomial implementation and measure
its cost; no performance claim about Qt's curve evaluator is established here.

## Checks and capture

- Numerical checks verify endpoints, endpoint derivatives and monotonic bounds
  for representative slopes, and demonstrate overshoot outside the safe range.
  Completed OutSine segments use exact zero derivative in the probe rather than
  treating floating-point cos(pi/2) residue as a real reversal.
- The prototype is linked separately against a core archive with the original
  scroll object removed. No duplicate-symbol suppression is used.
- Hardware regression suite: 15 passed on D3D11 and 15 on OpenGL, no failures or
  skips. Covers retarget/reversal, identical-command deadlines, first handoff,
  precise finite stepping, metadata rewrap, shrinking extent, interruption and
  hide/minimize. These are existing integration checks, not exhaustive new-curve
  coverage.
- Downloads186, Masonry152, DPR 1.75, 160 Hz, 30 ms wheel input with boundary reversals;
  four 10-second input windows. Bounded material/phase probes enabled, general
  frame recorder disabled. No builds/tests/analysis/UIA during input windows.

Of 1164 captured changed-target seams, 1155 use the preserving cubic and 9 use
fallback behavior. Reconstructed position residuals are below 8e-13 physical pixels;
the preserving seams' calculated speed discontinuity is below 1e-14 physical
pixels per 6.25 ms refresh. Fallback seams still have substantial speed changes.
These derivatives come from the recorded curve parameters; they are not direct
finite-difference measurements of displayed velocity.

No display interval exceeds 9.375 ms; maxima are 6.3761,6.3585,6.3996,6.4362ms.
Unchanged-segment displayed step-error p95 is 0.105–0.148physical pixels. The
existing p95<=0.5px gate passes. That gate alone is insufficient.

## Reason it was not promoted

Three windows contain displayed camera-step errors around 13.6–13.7px. Detailed
joining of actual rendered camera transforms and PresentMon display times shows
repeated animation time 0 on adjacent displayed frames in new jobs. Their precise
clock samples are unavailable, and their active curve is still the initial
OutSine (`hermiteRatio == -1`), before any cubic retarget. For example, cold job 5
commits visualY 5.025078 twice at animationMs 0, then advances to 12.543416 at 6 ms.
The displayed repeat spans 6.2467 ms. Coverage is ready in the recorded frames.

Thus this is a startup path that the different traversal/job timing exposes,
not evidence that the preserving cubic itself freezes. Its exact Qt driver/job
registration cause remains unproven. Do not hide it by reporting only p95 or
equating zero missed refreshes with smooth movement.

A new optional maximum-step-error gate supplements p95 and presentation coverage.
At a 2 px regression ceiling, the preceding production capture passes and this
prototype fails. The ceiling is a regression guard, not a definition of the
full 160 Hz objective. Full startup/retarget coverage still needs work.

## State and next action

The normal worktree app remains the verified identical-retarget build:
host SHA256 `c27b27dad94a944219a9716131e71ff184b7773ba63a6700dd804512226fa81b`.
Its responsive extracted host was rechecked. The diagnostic process was closed
by its exact worktree-specific launcher path. No production source, Qt dependency
or portable payload was changed by this experiment; no commit or push was made.

Verified compressed evidence is indexed by
`../experiments/velocity-retarget-prototype-2026-10-20/manifest.json`.
Next trace why a newly committed job can reach successive displayed frames
without an animation tick. Fix/retest that path before promoting interpolation,
then address the remaining incompatible-slope/reversal cases and broader
workload coverage. The original constant 160 Hz without spikes goal remains active.
