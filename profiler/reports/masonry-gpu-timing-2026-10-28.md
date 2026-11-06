# GPU timing separates target wait from drawing; constant 160 Hz remains incomplete

Native D3D11 timestamp queries show that most of the measured GPU elapsed frame
interval precedes selection/clearing of the window render target. In the warm
wheel block, the interval after clear is 0.080 ms median, 0.101 ms p99 and
0.120 ms maximum. This does not support treating PresentMon's approximately
6 ms GPU-active metric as six milliseconds of thumbnail drawing. It does not
identify the owner of a wait or prove a driver defect.

## Method and precision

The diagnostic host builds on the unpromoted per-buffer reuse candidate,
including its precompiled direct-atlas shader and earlier experimental backend
policies. These results are not measurements of the unchanged normal build.
Both arms use the same executable and policies; only timestamp observation is
disabled in the control.

Four GPU timestamp markers surround onscreen command execution: before target
commands, after first matching target selection (and optional native bind),
after its clear, and after command execution before Present. Each of eight
bounded slots has a disjoint query and four timestamps. Retirement uses
GetData(DONOTFLUSH), with no live flush or wait. Offscreen video contexts have
no onscreen samples. Query scopes include queue idle/preemption and CPU command
emission gaps; they are elapsed intervals, not isolated shader utilization.
Target state caching can elide a native bind. Pre-clear time cannot be assigned
exclusively to swapchain synchronization; preceding resource work also exists.

The first captures serialized uint64 GPU counters as JSON doubles. Their
approximately 1.79e18 values lose low bits. Those packets are retained as
approximate evidence, not exact-counter measurements. The separate exact host
serializes counters as decimal strings. A regression fails against the old
format and passes against the new format, checking every counter round-trip.
Startup frames outside gesture windows are excluded from frame matching.

## Exact-counter live comparison

Real Downloads, Masonry, 160 Hz / DPR 1.75, fresh application caches bd/be,
two 10-second rapid-wheel blocks per process. Collector output is buffered
until child exit, and no offline analysis or tests run during gestures.

| Metric | Timing cold | Control cold | Timing warm | Control warm |
| --- | ---: | ---: | ---: | ---: |
| Display intervals >9.375 ms | 3 | 4 | 0 | 0 |
| Maximum display interval, ms | 12.495 | 18.788 | 6.843 | 8.045 |
| CPU render p99, ms | 6.017 | 5.459 | 3.796 | 3.337 |
| CPU render maximum, ms | 14.943 | 11.540 | 5.803 | 7.164 |

Timing-on GPU samples cover all 1596 cold and 1601 warm complete CPU frames.
All end markers map into submission. There are 3244 completed lifetime samples,
zero skipped slots, disjoint/invalid rows, read/create errors or nested scopes.
Total polling time is 8.761 ms over 3245 frames, maximum single poll 0.019 ms.
The CPU differences above remain visible; a single pair cannot establish
negligible observer overhead or a stable performance improvement.

| GPU elapsed interval, ms | Cold median | Cold p99 | Cold max | Warm median | Warm p99 | Warm max |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Start to end | 4.493 | 5.673 | 12.953 | 4.748 | 5.308 | 5.810 |
| Start to target selection | 4.436 | 5.605 | 12.719 | 4.660 | 5.263 | 5.708 |
| Target selection to clear | 0.005 | 0.017 | 0.385 | 0.016 | 0.017 | 0.017 |
| Clear to end | 0.040 | 0.566 | 2.878 | 0.080 | 0.101 | 0.120 |

The longest cold GPU interval spends 12.719 of 12.953 ms before target
selection; CPU command emission is 0.458 ms and matched Present takes
11.840 ms. Cold CPU preparation separately reaches 13.968 ms. Neither cold
spikes nor the existing motion gates are solved by observing these intervals.
Warm zero-miss blocks do not establish sustained constant 160 Hz.

## Validation and rollout

- Old serialization regression: 4 pass / 1 expected failure.
- Exact timing/safety/lifetime tests: 5 pass, no skips.
- Rendered atlas shader tests at actual DPR 1.75: 27 pass; actual guarded
  reuse observed, zero failed reuse maps.
- Isolated embedded QML import tests: 4 pass.
- Windows static dependency audit: pass.

No UI layout or production rendering policy changed. The normal worktree app
remains PID 43936 with embedded host PID 119460. Only the exact diagnostic
launcher process tree was stopped after capture. No portable payload was
rebuilt or promoted, and no commits or pushes were made.

Next: attribute the pre-target wait against swapchain/present scheduling,
while separately reducing cold preparation/allocation tails. Retain the
strict display, motion, loading and fidelity gates when testing a change.
Evidence and exact executable/object hashes are in the matching experiment
snapshot; raw diagnostic binaries remain under artifacts.
