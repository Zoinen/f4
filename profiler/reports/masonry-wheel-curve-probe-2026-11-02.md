# Wheel-curve state and camera measurement

## Result and scope

The wheel-motion measurement gap now has compiled telemetry and a tested model.
The new probe joins curve state to the existing native camera samples by exact
sample index and commit identity. Native unit checks, 27 Qt easing-oracle points,
six Python tests, and a real Qt animator integration test pass.

This report describes probe validation. The first functional checks used the
Microsoft Remote Display Adapter at 1512x949 and 32 Hz. After transferring the
existing logged-in session to the unused console, two fixed-input captures ran
at physical 160 Hz. See the [fixed-input findings](masonry-fixed-input-catalog-resets-2026-11-03.md)
for those results and the incomplete comparison. The full goal remains active
and unachieved.

## Why the earlier wheel samples were excluded

The original motion analyzer knows only continuous velocity. It cannot model a
wheel easing segment, whose start/from state can also reset when coverage
constrains the camera. In `cold-m142`, its exclusions are 3,128 wheel-curve pairs,
1,662 input-revision pairs and 4 job changes. Those old traces do not contain the
missing curve parameters; they must not be retrospectively filled with invented
states or treated as zero-error samples.

## Instrumentation

`profiler/WheelCurveProbe.h` adds separate bounded POD storage. The original
`CurrentMotionProbe` header and sample layout are unchanged. The copied native
scroll animator records its curve start/from/destination, duration, easing type,
animation time, job and revision before coverage constraint, and continuation
start/from after any coverage reset.

Each curve record names the existing native-motion index and reuses that
sample's timestamp. Mismatched jobs, revisions or animation times, duplicate
joins and trace overflow are rejected or flagged. Easing-name reflection and
JSON serialization run during the later snapshot, not on every render tick.
Storage is prefaulted before input and remains bounded by the existing native
sample capacity. Live instrumentation overhead is still unmeasured at 160 Hz;
do not mix old and new instrumentation for a causal timing claim without an
overhead comparison.

The fixed-input recorder includes the new snapshot. Both control and candidate
hosts use identical new main and animator objects. Their only behavioral
comparison remains the prior frame-boundary handoff prototype. No production
renderer, layout, input policy or embedded payload is changed in this turn.

Control SHA-256:
`42aefe69f8eb8c3be95e6fe5d0203950c5e0d3e49c1d530d54ab6273a19422e1`.
Candidate SHA-256:
`129f3de9484926323454ef6ec637d7d714030f30150ee0333c88d7bf6445112d`.

## Model and tests

`profiler/wheel_curve_analysis.py` supports Linear, OutSine (the panel's current
wheel easing), and OutCubic. It evaluates native requested/camera positions from
the previous commit's continuation state and separately compares camera steps
against recorded display intervals. A coverage hold is an error sample, not an
exclusion. Retargets, job changes, geometry changes, unsupported curves and
missing curve records remain explicitly unverified. They do not become passing
samples. Metadata from an older exclusion cannot override the new result.

Six Python tests cover regular curves, clock mismatch, coverage holds, reset
continuation, retarget/unsupported/geometry exclusions, invalid parameters and
indexed joins. The native test verifies a successful indexed join and expected
duplicate, mismatch and overflow rejection. Its 27 progress points across the
three supported curves match Qt's actual `QEasingCurve` output exactly in the
Python comparison, including clamped progress before/after the segment.

## Real-animator integration at the available refresh rate

A diagnostic copy of `nativeScrollRetargetKeepsOneJob` enables both probes during
its actual native retarget animation. It passes **3 tests including fixture
setup/cleanup, 0 failures, 0 skips**. The trace contains 16 native samples and
16 curve samples, with zero join errors or overflow. Its actual window DPR is
1.75 using explicit Qt scale factor, refresh rate is **32 Hz**, and graphics API
value is 4 (the native backend used by this test).

All 16 same-commit requested positions match the recorded OutSine trajectory
with zero measured model error. Camera-versus-requested error reaches
**37.039 physical-coordinate pixels** when coverage constrains the camera. This
is useful positive evidence that the model preserves a hold rather than
misreporting zero camera error. It is not an optical measurement, a Downloads
benchmark, a display-clock result, or an estimate of 160-Hz behavior.

The test also retains its existing caption scene-coordinate/unit-transform
checks and byte-equal native-rest/GUI-reference capture comparison. The capture
was inspected; the existing MS Sans Serif/DirectWrite warning is retained in
the logs. No new sharpness claim or static layout change is made. The initial
fixture preparation used the process's default text encoding and failed to read
the UTF-8 source; explicit UTF-8 was used before the actual build/test.

## Build checks and rollout state

Both hosts pass embedded-only QML import isolation (3 passes each), the Windows
GUI subsystem test (3 passes each), and the system-DLL audit. Every native link
uses the response-file normalizer. No packaged extraction/default-cache launch
or production rollout is claimed. Closed binaries are hash-preserving archive
symlinks; the normal application is preserved.

Fresh curve-enabled capture/analysis recipes were prepared for cases
`cold-o161` through `cold-o164`; the follow-up report records their outcomes. The older
`cold-n151` remains a prepared, unlaunched control without this additional probe;
its frozen identity is not changed. New curve analysis requires actual curve
telemetry and must not be run as though an older capture supplied it.

The display preflight remains mandatory. Once physical 160 Hz is available,
measure instrumentation overhead and run the fixed-input comparison, retaining
all missing/late slots and exclusions. Resolve retargeted-pair and optical
verification gaps, remove remaining coverage holds/spikes, verify the full
cold/warm/middle-button workload and complete the portable production rollout
before claiming the goal achieved. No commit or push was made.
