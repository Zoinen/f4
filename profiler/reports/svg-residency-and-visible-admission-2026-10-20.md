# SVG residency and visible upload admission, 2026-10-20

The 160 Hz goal remains incomplete. This investigation has a passing candidate,
not a deployed production change. The running worktree application still uses
the previously verified small-upload buffer selection build.

## Evidence and rejected work

A bounded attribution probe compared complete staged vertex/index byte ranges
when the same source geometry reentered the renderer. Eleven staged comparisons
covering 9,436,464 bytes were identical, including a 2,241,216-byte batch. All
344 recorded removals were under blocked ancestors. Geometry data reports
AlwaysUploadPattern; simply declaring it static would violate its contract.
The copy/compare probe adds overhead and is not an end-to-end performance baseline.
The staging source and header were reconstructed byte-exact against their captured
build identities after generated sources had been overwritten.

A D3D11-only content-aware buffer reuse prototype used a bounded 16 MiB CPU shadow,
without retaining extra GPU buffers. Exact byte matches avoided 215,264,952 upload
bytes across 504 warm hits. Removing duplicate comparisons reduced comparison time
from 26.508 to 15.3545 ms. Warm pending-upload time fell from 53.0132 ms in a
same-binary disabled control to 40.7148 ms with the revised prototype, but complete
preparation p99 did not improve convincingly. This prototype is not promoted.
Its disabled control had a 12.4037 ms display interval and failed the motion gate;
those failing results remain evidence rather than being discarded.

A cold preparation spike also exposed a separate admission bypass: roots already
marked ready and visible bypassed the admission queue when additional geometry
appeared. A deterministic test appends 24 large batches after initial readiness.
Production uploads all 24 in one frame on both D3D11 and OpenGL, exceeding the
existing byte budget (at most five in this fixture).

The first admission candidate invalidated readiness whenever batch pointers changed.
Its focused renderer tests passed, but the complete OpenGL camera suite failed.
Tracing established that the same renderer repeatedly rebatches the same geometry
nodes. Revoking readiness for those changes causes unnecessary transitions and
partial admission. Refreshing the host observer after rendering did not solve this;
that edit was restored byte-exact (SHA256
21fd034964d005168d32941d2e8dbf6db9f98c149fa8975bac7c13a280736529).
One early trace pathname was reused: the archive preserves the final file at that
path and the separate identity trace, not both overwritten versions.

## Revised candidate

Track prepared geometry-node membership per source gate rather than batch-pointer
membership. New members revoke readiness and enter existing priority/byte/time
admission; rebatching existing members preserves readiness. A reverse index removes
membership when renderer nodes leave, guarding against stale pointer reuse. Gate
replacement and renderer destruction clear the associated entries. Animation and
existing byte/batch/time budgets are unchanged; no rasterization or new QML work.

The candidate lives in artifacts/prepare-visible-gate-admission.py and its generated
renderer. It has not been added to the portable Qt patch recipe or normal host.
The initial generator build failed due to escaped newlines; the corrected build
completed before all results below were collected.

At DPR 1.75 on each of D3D11 and OpenGL:

- Renderer suite including late visible geometry: 44 passed, zero failed/skipped.
- Native camera suite: 30 passed, zero failed/skipped.
- SVG presentation/animation/transition subset: 14 passed, zero failed/skipped.

Native tests include scene-coordinate/transform and rendered-image checks inherited
from the suites; this does not substitute for production packaging and import gates.

## Scrolling capture

real-scroll-visible-known-nodes: Downloads, 186 entries, Masonry density 152,
DPR 1.75, 160 Hz screen, 30 ms bouncing wheel. Four 10-second input windows;
no builds, tests, offline analysis or UIA observation during input. Native scrolling
uses the production default and finite precise animation clock. Trace logging and
content-aware cache are disabled. Instrumentation is bounded; successful-link receipts
pin the experimental renderer and actual linked library.

All four blocks had zero missed display intervals (>9.375 ms), no out-of-presentation
camera commits, and passed p95 <=0.5 px / maximum <=2 px motion error limits.
Display maxima by block: 6.3613, 6.4003, 6.3623, 6.3403 ms.
Motion error maxima: 0.7369, 0.8232, 0.7920, 0.6405 physical pixels.
No thresholded slow upload (>=100 us) from a ready+visible gated SVG bypassed
admission in the input windows. This is thresholded evidence, not proof of zero
unrecorded sub-100-us operations.

Whole preparation p99: cold 2.2893/2.0378 ms; warm 2.1111/2.0941 ms.
Public preparation maximum remains 6.9057 ms (render-to-frame-end 8.5092 ms).
The worst internal renderer preparation during input is 6.4487 ms; pending upload
is 3.9356 ms, alpha material work 1.3618 ms. These inclusive scopes must not be added
blindly. The existing 2 ms admission limit is soft: one costly batch can exceed it.
The next optimization must address that cost, not infer success from display cadence.

## Remaining work

Promote the known-node candidate only after reviewing lifecycle/overhead and adding
the regression and strict source replacements to the portable Qt patch recipe.
Rebuild the production library/host, run import-isolation, dependency, embedding and
extraction gates, then restart only the worktree binary and measure that exact build.
No experimental cache or failed observer edit should enter that build.
Further reduce large single-batch upload and material costs, then repeat cold/warm
input and additional scrolling paths. One 40-second zero-miss run does not establish
constant 160 Hz without spikes across the full application.
