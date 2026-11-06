# Native continuous middle-button scrolling — 2026-10-20

## Implemented behavior

Masonry middle-button auto-scroll now uses the same native camera owner as wheel
and keyboard motion. GalleryScrollAnimation exposes continuous velocity, stop,
and running state. One render job integrates velocity and updates its current
bounded preparation target from GUI synchronization. Velocity changes reuse the
job; stop commits the latest camera before persistence. Geometry offsets and
extent limits retain the existing camera contract.

AutoScrollController calculates the existing speed curve in logical pixels per
second when pointer motion changes. The dead zone stops native motion. Other
modes and software rendering retain FrameAnimation with algebraically identical
speed calculation. The controller passes its existing animation owner through
GalleryViewport/GalleryPointerLayer. Wheel input ends an armed middle gesture
before starting wheel animation. Cancellation defers pointer reconciliation via
Qt.callLater. No new visual wrapper, module, setting, Go field or dependency.

## Reproduction and regressions

Baseline: Downloads186, Masonry152,160Hz,DPR1.75,296.5logical pointer distance,
exactly one quarter of the measured1186logical window height. Two directions
at1500ms each in both cold/warm stages, avoiding endpoint dwell in input windows.
Expected speed1627.433412logical pixels/second. Cold first window has three
missed displayed refreshes, maximum12.5043ms; remaining windows have none.

The continuous API regression initially fails because no driver exists. The new
driver then advances rendered QSG camera transforms during a120ms GUI pause,
while GUI contentY stays unchanged. Stop commits25–65logical pixels of progress
at300pixels/second, remains stable, reverses direction in one job, respects the
bottom limit and rejects Grid eligibility. The wired gesture regression initially
fails because middle input still drives GUI ticks. After wiring it verifies
native ownership, neutral idle, movement during GUI pause, stop, and wheel exit.

The stale-viewport test initially assumes stop equals the last GUI snapshot and
fails intermittently on D3D11. The test now snapshots immediately after end,
requires forward progress, preserves its no-reversal assertion, then verifies
that exact committed position stays stable and is persisted. This tests the
native handoff instead of requiring stale GUI bookkeeping.

## Clock refinement and physical motion evidence

The first native prototype removes displayed misses in the short capture, but
uses integer-millisecond Qt animation time for velocity integration. At160Hz its
committed-camera step error against elapsed time has median about1.1physical
pixels and p95 up to2.86pixels.

Continuous motion now integrates high-resolution steady-clock time at render
commit. Geometry offset is mapped into the job's current coordinates before
clamping. Finite eased animations retain their existing clock behavior. This
adds one clock query for continuous commit, not fitting or rounding per leaf.

The high-resolution capture has zero displayed intervals above9.375ms in all
four input windows (six total seconds), maxima6.3308,6.2740,6.3330,6.3104ms.
There are no recorded coverage stalls or missing presentation brackets.
Actual start events confirm continuous=true with velocities+/-1627.433412.
Camera-step error uses the rendered matrix, not only the requested value:
median0.036–0.040physical pixels,p95 0.20–0.23pixels. A diagnostic p95<=0.5pixel
gate fails on the quantized prototype and passes on the high-resolution capture.

These are short observations, not statistically isolated speedup or universal
160Hz proof. Committed-camera fidelity is distinct from displayed-motion fidelity.
After joining actual PresentMon display times, displayed step-error p95 remains
about0.97–1.61pixels. Maxima about10.4–12.3pixels occur29.5–35.8ms after start
in three windows; the other window's maximum3.07pixels is around1015ms.
Initial presentation phase/queue behavior remains work despite zero refresh
misses. Preserve these outliers; do not substitute commit timing for display.

## Verification

- Wired gesture suite:30passed per D3D11/OpenGL,0failed/skipped.
- Final high-resolution suite:27passed/backend,0failed/skipped. Includes actual
  rendered GUI-pause motion, both grouped/ungrouped middle cases in all modes,
  stale restoration, thumb mappings, metadata rewrap and shrinking extents.
- Current SVG/viewer integration:12passed/backend,0failed/skipped, all panel
  modes, gradients/transparency, viewer transitions, cache reuse and progress.
- Final software fallback:13passed,0failed/skipped. Images saved separately.
- Resting Text/image leaves retain175% scene origins and unit transforms;
  current Masonry capture inspected including small captions.
- Portable import isolation, system-DLL audit, extraction tests and packaging
  passed. Parent/submodule diff checks pass with existing CRLF warnings.

Input windows run without builds/tests/analysis/UIA/scheduler tracing. Same bounded
material/phase probes as previous captures; general frame recorder is disabled.
Capture identities preserve actual-link/source hashes and capacity checks.
A disk-space build failure was resolved by losslessly compressing two old raw
scheduler CSVs and verifying decompressed SHA256 before removing originals.
The normal app remained alive. Quantized source was recovered by reversing only
the clock change and requiring exact equality with its captured SHA256; an
initial comparison differed by CRLF normalization and was corrected in bytes.

## Deployment and remaining objective

Worktree launcher directly runs --gui=qt with no override. Responsive extracted
host matches current built host SHA256:
8d5721f233363d8744c597b46b5942abdbe67af2482526df89fc06202c7e5fc7.
Launcher SHA256:
cb23ed36199b6099cd2052acc97e92eda84a05312a8efc6b14c5bf6e17b7b26c.
Only exact diagnostic/worktree launcher trees were stopped; canonical binary
untouched. No commit/push. Evidence archived with round-trip SHA256 checks.

Constant160Hz without spikes remains incomplete. Next work must investigate
presentation-phase motion sampling, extend middle/wheel stress duration with
bounded reversals, and verify continuous-mode window suppression/restore and
source-replacement lifecycle. Existing finite-motion lifecycle tests alone do
not prove the new continuous gesture's complete lifecycle.

AI assistance has been used to create this output.
