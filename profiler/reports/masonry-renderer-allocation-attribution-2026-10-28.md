# Repeated logical uniform-buffer growth recreates unchanged physical capacity

A bounded attribution capture identifies **395 uniform-buffer recreations with
unchanged native D3D capacity** across the recorded process. Of these, 284 occur
in the cold scrolling block and 34 in the next block. This is a concrete target
for a capacity-retention experiment. No production optimization is implemented
in this step; constant 160 Hz remains unachieved.

## Instrumentation and checks

The diagnostic uses the current production Qt Quick batch-renderer source,
including existing buffer-selection and upload-policy fixes. Strict source
replacements reverse exactly to that source. It records actual geometry and
uniform creation brackets, previous logical capacity, requested size, batch,
renderer and material identity. A local private-header copy records new batch
creation; reused/recycled batches are aggregate counters. There is no ABI or
allocation-policy change. The original RHI owner/resource/producer recorder
remains enabled; no video conversion experiment is included.

The first attempt is rejected: recording every batch reuse/recycle generated
368,232 events against 32,768 slots. Its source, receipt and rejection are
retained. It cannot support complete attribution or presentation conclusions.
The revised recorder removes individual reuse/recycle timestamps, keeps their
bounded aggregate counters, and retains individual actual allocations/new
batches. The valid capture contains 2,608 events within the same capacity.

The final instrumented renderer passes 44 native checks with no failures/skips
at expected actual DPR 1.75, using the requested D3D11 backend. Rendering tests
include buffer selection, order/opacity behavior, geometry retention, scene
origins/unit transforms and rendered pixels. The host passes four import and
compiled-host checks and the system-DLL audit. These gates check diagnostic
compatibility, not complete portable release rollout. No UI layout changed.

## Valid live workload

`real-scroll-cold-renderer-allocations-r2-20261028-wheel-masonry`, Qt PID 91876,
render thread 108664. Actual Downloads, 190 entries, Masonry density 152,
160 Hz, DPR 1.75 and an initially empty separate application cache. Two
ten-second rapid-wheel blocks pass the unchanged active-window, input-count,
record-capacity and target-PID presentation gates. OS disk cache and Go metadata
are not cleared. No builds, tests, offline analysis or UI Automation observer
run during gestures. The normal portable worktree app stays alive.

| Block | Recorded renderer buffer/texture creations | Matched to creation brackets | Unmatched |
| --- | ---: | ---: | ---: |
| Cold | 1,518 | 1,418 | 100 |
| Next | 180 | 142 | 38 |

Matching requires the same thread and resource-creation timestamp inside the
recorded creation bracket. No nearest-time matching is used. Multiple native
resources can belong to one QRhi geometry-buffer creation. Unmatched resources
remain unattributed; counts do not include every driver-internal allocation,
shader, sampler or pipeline.

Cold creation groups include:

- Text-mask geometry rebuilds: 289, including 98 logical size increases.
- Smooth-color geometry rebuilds: 251, including 115 size increases.
- Smooth-color unmerged uniform buffers: 204; 18 new and 186 logical growths,
  all requesting 256 bytes.
- Opaque-texture geometry rebuilds: 154 at unchanged logical size.
- Text-mask merged uniform buffers: 107; 31 new and 76 logical growths,
  all requesting 160 bytes.

An unchanged-size geometry rebuild is not automatically waste: static-to-dynamic
promotion also requires recreation. This probe does not yet separate every
promotion or establish that such geometry creations can be removed safely.

## The avoidable-capacity pattern

The analysis follows uniform-buffer identity across its recorded creations.
It requires positive previous logical size, agreement with the preceding
record's requested size, and equal native `D3D11_BUFFER_DESC::ByteWidth` in
the corresponding exact creation brackets. It finds 395 repeated equal-capacity
allocations; the 77 outside the two scrolling blocks are not called cold-scroll
work.

Example on renderer `2ecb5c20`, batch `2bce6de0`, QRhi buffer `759fe7b0`:

1. Opaque-texture material initially requests 64 logical bytes; D3D allocates
   256 bytes, native resource `75ca4d20`.
2. Vertex-color material reuses the buffer and requests 68 logical bytes.
   The renderer calls `setSize` and `create`, yielding another 256-byte native
   resource `75ca4fe0`.

The source explains the discrepancy: the batch renderer compares exact logical
sizes, while `QD3D11Buffer::create` rounds uniform-buffer allocation to 256 bytes.
Its CPU shadow allocation uses the logical size. Therefore skipping native
recreation inside RHI without also handling CPU shadow capacity would be unsafe.
The next candidate should make the batch renderer reserve the existing D3D
allocation capacity from the outset, keeping actual shader binding/update ranges
unchanged. It needs a failing native regression before a fix, an allocation
count check, rendered verification and live on/off measurements. This is resource
capacity policy, not per-frame pixel-position rounding. Native GPU capacity
need not increase for requests already rounded by D3D; CPU shadow padding and
larger uniform groups must still be measured and bounded.

## What this does not establish

The longest cold API calls are Maps of 13.1877/13.1436 ms and an upload of
8.5733 ms. They do not occur inside the recorded creation brackets. Individual
recorded creation scopes are generally much shorter. Removing duplicate
allocations may reduce contention, but this capture does not prove that it will
remove those waits. There is no scheduler trace in this capture, so blocked
time, GPU execution and lock ownership cannot be inferred from API wall time.

Presentation still misses 14 refresh opportunities in the cold block and three
in the next; maximum display gaps are 24.4893 and 12.5181 ms. Coverage holds are
zero, but individual motion errors remain above the acceptance gate. Observer
overhead has not been isolated, so these are diagnostic results rather than
an uninstrumented performance baseline or an improvement claim.

Sources, receipts, native results, complete/rejected capture packets and
analysis are frozen in the matching experiment directory. Large binaries have
external hash references. Only diagnostic process trees were stopped. The
normal launcher is unchanged. No production rollout, commit or push.

AI assistance has been used to create this output.
