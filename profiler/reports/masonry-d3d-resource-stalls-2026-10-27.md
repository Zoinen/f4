# Masonry cold-scroll D3D11 resource-call attribution

The cold-cache pauses have been located inside specific D3D11 resource calls.
Scheduling evidence shows that their long wall times are predominantly blocked
time, rather than CPU execution or time waiting to be scheduled. This is an
attribution result, not an optimization or proof of the underlying wait owner.
The constant-160-Hz objective remains incomplete.

## Baseline and diagnostic validity

The cached Qt 6.11.1 RHI source was recompiled using the SDK's existing compiler
options and PCH. All 412 executable `.text*` sections (180,832 bytes) match the
original Qt6Gui archive member byte for byte. This does not claim whole-object
identity or normalized relocation-target equivalence. The SDK archive was not
modified.

A diagnostic-only override wraps 102 existing COM call sites without changing
arguments, return types or call count. Reversing the wrappers reconstructs the
unmodified source exactly. A fixed 65,536-record buffer retains calls lasting
at least 50 microseconds; it performs no per-call allocation or JSON export.
Timestamps and atomic recording add overhead, whose complete enabled/disabled
frame-time impact has not yet been quantified. Therefore these captures locate
blocked calls; they are not a production performance comparison.

Validation before live capture:

- Recorder behavior: five cases passed; disabled-recorder invocation: three
  passed, including test setup/cleanup in these counts.
- Current native D3D11 rendering suite at DPR 1.75: 44 passed, zero failures,
  zero skips. An earlier invocation lacking the native-renderer environment
  skipped cases and is not counted as verification.
- Four import-isolation/compiled-host cases and system-DLL audit passed.

Only the RHI object and diagnostic controller differ from the production
baseline. Earlier experimental renderer objects are excluded from the link.

## Captures

Both captures use the actual Downloads folder, Masonry, 160 Hz, DPR 1.75,
default media behavior, and a new empty application cache root frozen before
model initialization. The OS page cache and Go metadata cache are not cleared.
Each contains two 10-second rapid wheel blocks. No tests, builds, UI Automation
observer, or offline analysis run during the gestures.

| Capture | Block | Gaps over 9.375 ms | p99 display interval | Maximum interval | Coverage holds |
| --- | ---: | ---: | ---: | ---: | ---: |
| Resource calls, Qt PID 112532 | 0 | 11 | 6.6204 ms | 24.9836 ms | 0 |
| Resource calls, Qt PID 112532 | 1 | 2 | 6.3793 ms | 12.5180 ms | 0 |
| Resource calls + scheduler, Qt PID 105248 | 0 | 7 | 6.8322 ms | 18.8438 ms | 0 |
| Resource calls + scheduler, Qt PID 105248 | 1 | 0 | 6.3224 ms | 7.5986 ms | 0 |

The first capture recorded 2,390 slow calls out of 8,595,027 invocations;
the scheduler capture recorded 2,360 out of 8,794,031. Neither overflowed.
The cache-activity observer's maximum callback was 50.4 and 35.8 microseconds,
respectively; these numbers do not measure the RHI wrapper's aggregate overhead.
In the scheduler capture, stores increased from 3 to 167 in block 0 with up to
57 requests pending; block 1 increased from 168 to 183 with up to 16 pending.
Thus the first block demonstrably overlaps preparation.

## Same-capture scheduler attribution

The kernel trace lost zero events/buffers, reported zero schema errors and
completed ProcessTrace successfully. Every measured API interval below has
zero unknown scheduler time. These are separate calls, not additive components
of one frame.

| API and Qt call site | Frame phase | Wall | Scheduled CPU | Blocked | Runnable |
| --- | --- | ---: | ---: | ---: | ---: |
| UpdateSubresource, line 3081 | submission | 12.2417 ms | 0.2104 ms | 12.0205 ms | 0.0108 ms |
| UpdateSubresource, line 3081 | submission | 10.3437 ms | 0.3543 ms | 9.9792 ms | 0.0102 ms |
| Map, line 2678 | recording | 6.2070 ms | 0.3266 ms | 5.8716 ms | 0.0088 ms |
| Present, line 1541 | submission | 6.1471 ms | 0.0819 ms | 6.0593 ms | 0.0059 ms |
| CreateTexture2D, line 3686 | preparation | 5.7060 ms | 0.1867 ms | 5.4942 ms | 0.0251 ms |
| Map, line 3230 | preparation | 3.0717 ms | 0.1265 ms | 2.9374 ms | 0.0078 ms |

Line 2678 maps a pending dynamic buffer with WRITE_DISCARD before copying its
contents. Line 3230 maps a full dynamic-buffer update with WRITE_DISCARD.
Line 3686 allocates a D3D11_USAGE_DEFAULT texture. Line 3081 executes a queued
UpdateSubresource command; this probe does not yet record its resource type,
size or identity, so it must not be attributed exclusively to image textures.

The longest recorded render-thread call in block 1 is a 0.2761-ms
UpdateSubresource with all time scheduled. That narrows the problem to activity
associated with preparing cold resources, but does not establish which resource
or concurrent producer causes contention. The earlier software-video control
also failed to eliminate cold stalls and does not justify disabling hardware
video decoding.

## Next experiment

Record bounded resource metadata for the slow calls: texture/buffer identity,
description and upload size, plus correlation to thumbnail/atlas preparation.
Do not query COM descriptions repeatedly in the hot path; collect metadata
when the resource is created. Establish recorder overhead with a disabled
control. Use those results to choose between reducing upload bursts, reusing
resources or changing their update strategy. A scheduler wait alone cannot
identify driver locks, GPU execution, resource hazards or a lock owner.

Do not change layout rounding, safety clamping, animation clock, cache bounds
or hardware decoding based on this evidence. No production source or embedded
payload changed in this experiment. Only the exact diagnostic launcher tree
was closed afterward; the normal portable worktree instance remains running.
No commit or push was made.

Evidence: `profiler/experiments/masonry-d3d-resource-stalls-2026-10-27/`.
Receipts pin the baseline, probe sources, tests, capture packets, presentation
CSVs and scheduler attribution. Large binaries and the compressed 171,966,464-
byte raw scheduler trace remain in artifacts, referenced by hashes.

> AI assistance has been used to create this output.
