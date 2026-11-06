# Worker-prepared geometry removes recorded large creates, but cadence remains mixed

A bounded D3D11 worker prototype supplies fresh geometry buffers before demand.
Both enabled runs adopt every eligible request with zero misses or creation
errors, and record no main-context large-buffer allocation wrapper lasting
50 microseconds or more. One control has an 11.5811 ms large-buffer creation.
However, display misses and loading progress are mixed, so the candidate is
not promoted. Constant 160 Hz remains incomplete.

## Ownership and policy

Microsoft distinguishes thread-safe ID3D11Device resource creation from the
single-threaded device-context/DXGI operations. The prototype only calls
CreateBuffer on its worker; Map, Unmap, copy, draw, queries and Present stay
on their original threads. Eligibility additionally requires the driver's
DriverConcurrentCreates support and rejects SINGLETHREADED devices.
[Microsoft threading documentation](https://learn.microsoft.com/en-us/windows/win32/direct3d11/overviews-direct3d-11-render-multi-thread-intro).

The opt-in worker starts after an eligible non-video QRhi device is created.
It prepares two fresh dynamic buffers for each power-of-two capacity from
64 KiB to 4 MiB, separately for exclusive vertex and index bindings. Video
conversion devices are excluded by their existing producer tag. All 28 reserve
slots together hold at most 33,292,288 bytes (31.75 MiB), below the 32 MiB
per-device budget. There are at most 32 recorded devices; this is a per-device
bound, not a universal 32 MiB process bound. The live workload has one eligible
device. Adopted active allocations are outside the reserve budget.

The render thread takes a matching ready buffer under a short mutex; native
creation runs outside that mutex. A request can adopt only a capacity at least
its requested size and strictly below twice that size. Unsupported descriptors,
sizes, threads or unavailable slots retain the original synchronous allocation.
The QRhi logical/shadow size, upload length and draw lengths remain unchanged;
native allocation descriptors report the adopted physical capacity. Buffers
are fresh, never returned from GPU use to this reserve. Existing GPU-completion
and DISCARD/NO_OVERWRITE rules are unchanged.

Consumption wakes the worker to refill the slot. Device shutdown sets stop,
joins the worker, releases remaining reserve objects and its extra device
reference. There is no render-thread wait for a ready slot. Device destruction
does wait for a currently executing worker creation to finish; this is a
shutdown cost, not evidence of a nonblocking destruction path. Device creation
can still contend with rendering in a driver; legal threading does not promise
performance isolation.

## Validation

Four pool checks pass, including setup/cleanup: actual D3D11 vertex/index data
copy/readback for five sizes each, capacity and descriptor validation, budget,
unsupported-size/constant-buffer/wrong-thread rejection, and worker/resource
shutdown. QTRY acquisition short-circuits once the pointer is acquired, avoiding
duplicate acquisition on QtTest's final assertion evaluation. The fixture was
rebuilt before testing and capture; both source and binary hashes are retained.

The full native scene suite passes 47 checks at actual DPR 1.75; the rendered
shader suite passes 27 checks, with actual guarded uniform reuse and zero failed
maps. Embedded import isolation passes four checks and the Windows dependency
audit passes. No UI layout was changed.

## Live captures

Real Downloads, 190 entries, Masonry density 152, 160 Hz / DPR 1.75. Enabled,
control, repeated control, repeated enabled use the same executable with fresh
application caches bi/bj/bk/bl and two 10-second rapid-wheel blocks each. Timing
queries are disabled. Earlier experimental atlas/DXBC, staging, geometry and
uniform policies are identical in all arms; this is not a comparison with the
unchanged normal application. Helpers buffer output until collection ends.
No tests, offline analysis or UI Automation run during gestures.

| Arm / Qt PID | Reserve hits / misses | Worker creates / total worker wall time | Cold / next missed refreshes | Cold / next max display interval |
| --- | ---: | ---: | ---: | ---: |
| Enabled / 57692 | 35 / 0 | 63 / 9.2881 ms | 7 / 4 | 25.0095 / 12.5174 ms |
| Control / 125076 | Disabled | Disabled | 11 / 0 | 18.8508 / 7.2276 ms |
| Control repeat / 70216 | Disabled | Disabled | 3 / 1 | 12.5888 / 12.5088 ms |
| Enabled repeat / 47008 | 33 / 0 | 61 / 7.9474 ms | 5 / 1 | 18.7771 / 12.5114 ms |

Misses are displayed intervals above 9.375 ms within gesture windows. Worker
counts/hits cover the full recorded process, including initial prefill. Both
enabled reserves reach 31.75 MiB with zero creation errors. Eligible hits
transfer 18.25 / 17.375 MiB cumulatively; these are not simultaneously live-byte
measurements. The recorded wrapper named CreateBuffer now includes adoption:
its resource record does not itself prove a new native creation. Worker creates
have a separate API/producer label. Large wrapper times below the 50 us recording
threshold are absent, not zero-duration calls.

| Arm | Cold / next CPU render p99 | Cold / next CPU render max | Cold max >=64 KiB geometry allocation wrapper |
| --- | ---: | ---: | ---: |
| Enabled | 5.5935 / 5.9334 ms | 15.9676 / 10.9135 ms | No recorded >=50 us call |
| Control | 6.8385 / 3.7205 ms | 15.6954 / 6.3237 ms | 11.5811 ms |
| Control repeat | 5.3613 / 4.0503 ms | 13.6289 / 7.5269 ms | 0.2496 ms |
| Enabled repeat | 5.8783 / 3.6116 ms | 11.7868 / 5.5074 ms | No recorded >=50 us call |

End stored-cache counts are 189/206/221/198; they are snapshots, not unique-file
throughput. End private memory is 1215.54/1140.70/1116.23/1043.16 MiB; process
peak pagefile is 1887.46/2137.93/2150.69/1752.56 MiB. These variable whole-process
samples do not isolate the reserve or prove memory/loading neutrality. Coverage
holds are zero, but all four runs fail the complete no-miss/motion requirements.

## Remaining spikes and next action

The first enabled cold block still has a 6.5197 ms vertex-buffer creation for
50,952 bytes, below the 64 KiB reserve cutoff. Other tails include Present at
13.4252 ms, uniform DISCARD at 3.1376 ms in its next block, and geometry DISCARD
at 3.5502 ms in the repeated enabled cold block. Moving some creates to a worker
does not establish that these calls are independent of driver allocation work.

The measured 50,952-byte stall motivates testing a lower reserve cutoff using
the same bounded reserve, rather than widening unrelated capacity heuristics.
Unfinished-marker uniform/geometry DISCARD and present scheduling remain
separate unresolved tails. No further optimization is proven by this report.

Only exact diagnostic launcher trees were stopped. The normal worktree app
remains PID 43936, embedded host PID 119460; no payload replacement, commit or
push occurred. The matching snapshot retains build receipts, sources, tests,
capture packets, PresentMon target rows, memory, motion analysis and external
binary/object hashes.
