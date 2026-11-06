# Masonry: live uniform-rotation comparison and remaining CPU work

## Status

The constant-160-Hz objective remains incomplete. No renderer experiment was
promoted, no embedded payload was replaced, and no commit or push was made.
This report supersedes the display-blocked status in the preceding uniform
backing-rotation report, without changing its correctness results.

Windows Display Settings initially showed 175% while a newly started Qt probe
reported DPR 2.25. Reapplying the setting through Windows (200%, then 175%)
restored actual Qt DPR 1.75 at 3840x2160 and 160 Hz. Merely selecting the already
displayed 175% entry did not do so. The reason for the inconsistent state is
not established. The final independent probe still reported 160 Hz / 1.75.
No Qt scale override was used for the performance captures.

## Matched workload and admission gates

Real Downloads folder, Masonry density 152, 191 entries, two ten-second wheel
blocks with 10-ms nominal input interval and direction reversal. Logical window
2194x1186; panel 1097.142857x1035.428571 at DPR 1.75. Each run started with its
own empty gallery cache. Both arms enable the worker-prepared geometry/constant
buffer pool and skip the D3D11 native-handle capability query. Only the uniform
backing-rotation flag differs. CPU profiling was absent in the four A/B runs.
No UI Automation snapshots or analysis ran during their input windows.

Case prefix: `real-scroll-cold-uniform-backing-rotation-session-`; suffix:
`-20261028-wheel-masonry`.

| Arm | Input events | Capture gate | Cold / warm display misses | Cold / warm maximum display interval |
| --- | ---: | --- | ---: | ---: |
| r3-on | 1490 | rejected: fewer than 1500 inputs | 129 / 40 | 38.8762 / 14.0998 ms |
| r3-off | 1503 | passed | 58 / 54 | 20.4706 / 25.0037 ms |
| r4-off | 1422 | rejected: fewer than 1500 inputs | 29 / 41 | 36.6251 / 18.8877 ms |
| r4-on | 1588 | passed | 24 / 41 | 18.7507 / 19.0488 ms |

Every run retained focus for every recorded input, reported DPR 1.75 / 160 Hz,
and had no display-change event inside a measurement window. Rejected runs
remain rejected; their timing is diagnostic evidence, not an accepted cadence
comparison. A display miss is an interval above 9.375 ms. Every block also fails
the maximum two-physical-pixel motion-error gate. No coverage hold was recorded.

Rotation has no established, repeatable cadence benefit. The accepted r4-on
warm block still has 41 display misses. Its largest recorded render-thread RHI
API call is only 0.3701 ms; frame phases include a 7.145-ms preparation interval
and a 16.040-ms whole frame. Slow-call samples have a reporting threshold, so
these API maxima do not prove that all driver overhead is negligible.

r3-on performed 3301 rotations, released all 3301 retired buffers, and peaked at
176896 retired bytes; r4-on performed 2322, released all, and peaked at 122112
bytes. Fresh-buffer misses remain substantial (880 and 816 rotation attempts).
The bounded fresh-pool policy was not widened to conceal these misses.

## Scheduler attribution

`real-scroll-cold-uniform-backing-rotation-scheduler-20261028-wheel-masonry`
retains a lossless scheduler trace: zero lost events, lost buffers, schema
errors, or ProcessTrace errors. Its 1290 inputs fail the capture admission gate;
this instrumented run therefore does not establish cadence improvement.
Individual measured spans can still be joined exactly to scheduler intervals.

The worst warm preparation span is 12.5830 ms:

- scheduled on render thread: 12.4719 ms;
- runnable but not scheduled: 0.0578 ms;
- blocked: 0.0533 ms.

The largest complete warm render span is 17.6735 ms, with 17.5161 ms scheduled.
This particular spike is primarily execution on the render thread, rather than
waiting for GPU completion or CPU scheduling. Scheduled time is not a pure
instruction-retirement measurement and does not identify a function by itself.
Other spans still contain waits: a warm pacing/sync interval is 9.0170 ms,
including 7.1256 ms blocked. There is more than one source of frame delay.

## Independently checked CPU samples

`real-scroll-uniform-rotation-cpu-20261028-wheel-masonry` passes the capture
gate: 1543 inputs, zero inactive inputs, actual DPR 1.75 / 160 Hz. Its kernel
trace loses no events or buffers. A separately linked symbol map matches the
profiled host's `.text` bytes and runtime-function table byte for byte.

The offline stack reader initially rejected 11 valid stacks exceeding its
192-frame limit. A new 256-frame regression failed with exit 4 before the fix.
The isolated reader now accepts up to 256 frames and rejects 257, malformed
sizes and unsupported versions; its self-test passes. Re-decoding yields 23038
stacks, 21374 profiles, 752 module records, and zero schema errors. The sample
prefix fallback is independently checked against xperf: all 120 reference
samples match thread, instruction address and timestamp, with zero errors.

Sampled self counts in the combined preparation phases include:

| Symbol | Samples |
| --- | ---: |
| `QSGNodeUpdater::isNodeBlocked` | 431 |
| `F4VectorGateFrame` constructor | 232 |
| `QSGBatchRenderer::Updater::visitNode` | 152 |
| `Renderer::updateMaterialDynamicData` | 148 |
| `QRhiBufferData::assign` | 106 |

Recording self samples include `StageUniformBufferBatches` copy construction
(362), `QRhiD3D11::setShaderResources` (139), and `ResourceBatches` copy
construction (134). Counts are samples, not exact elapsed durations. Rotation
changes native generations and can increase resource-binding rebuild work;
without the matching control CPU trace these counts do not prove a regression
caused by rotation. Identical-code-folding aliases are retained in the output.

Preparation maxima in this separate CPU capture are 7.7813, 7.4011 and 6.3036 ms;
they are not the same frames as the scheduler-only 12.583-ms span. Source
inspection confirms that `F4VectorGateFrame` scans upload batches and walks
ancestors to identify vector owners on each relevant frame. A cache would need
correct invalidation for batch/node replacement, ancestry changes, gate
generation, deferred vector publication and destruction. Removing readiness
checks or bypassing upload admission would change fidelity and is not a fix.

## Next work

Obtain the matching rotation-disabled CPU profile, distinguish existing scene
traversal from rotation-induced binding copies, then prototype one measured
cost reduction with explicit invalidation/lifetime regressions. Preserve vector
readiness, drawing order, cold loading, physical-pixel fidelity and the complete
cold/warm wheel plus middle-scroll acceptance workload. Repeat live cadence
and motion verification; a reduction in one sampled function is insufficient.

The exact diagnostic launcher tree was stopped after collection. The preserved
normal launcher PID 43936 and embedded Qt host PID 119460 remain alive. Sources,
receipts and derived results are snapshotted separately; raw traces and binary
outputs remain external references with hashes.
