# Render-thread wake-up attribution and lazy geometry shadows

2026-10-29, `zoin_branch/cached-pixel-grid`. The previous goal turn made
progress by testing and rejecting the ancestor-cache implementation. This
continuation identifies code associated with three reproduced waits and tests
a targeted allocation change. Constant 160 Hz without spikes remains open.
No performance candidate is promoted, no portable embedded payload is replaced,
and no commit or push is performed.

## Trace and verification

The owned kernel session enables PROC_THREAD, LOADER, CSWITCH and DISPATCHER,
with CSwitch/ReadyThread stack walking. It does not enable sampled CPU profiling.
The captured workload is the real 191-entry Downloads folder, Masonry density
152, actual 3840×2160 / 160 Hz / DPR 1.75, cold loading and two ten-second rapid
wheel blocks. Normal content, source priorities, hardware decoding and rendering
remain enabled. No compiler, tests, offline analysis or UI Automation observer
runs during input. The detailed per-RHI-call recorder and ancestor-step counter
writes are disabled; bounded frame/upload probes remain.

`real-scroll-unmap-wait-stacks-20261029-wheel-masonry` passes capture guards.
The 630,194,176-byte ETL reports zero lost events/buffers, schema errors or
ProcessTrace errors and covers both windows. Stack walking is intrusive, so
these timings establish attribution, not clean performance or prevalence.
The raw ETL and verified compressed archive are retained on C:.

Microsoft documents the [ReadyThread target and flags](https://learn.microsoft.com/en-us/windows/win32/etw/readythread),
[stack-walk options](https://learn.microsoft.com/en-us/windows-hardware/test/wpt/stackwalk)
and [ThreadTime wake-up analysis](https://devblogs.microsoft.com/dotnet/work-flow-of-diagnosing-memory-performance-issues-part-2/).
The observed ReadyThread event headers contain invalid source PID/TID values
(`0xffffffff`). The extended reader therefore tracks all CPU switch states,
checks per-CPU event ordering, and records the running context separately from
the raw header. For all five selected ReadyThread events, this reconstructed
context independently matches the PID/TID inside the corresponding StackWalk
payload. It is not an inference from a thread name.

The reader retains all thread-lifetime events and module ownership by PID.
Thread names are read only after capture; an already-exited thread remains
identified by its trace records. Native decoder self-tests pass. Both final
reader sources regenerate byte-for-byte. No sampled-profile prefix fallback
is used as CPU evidence in this trace, because CPU profiling is disabled.

The application symbol map is verified against **42,085,978 byte-identical
executable-code bytes** and the same runtime function table as the captured
host. Resolution retains runtime/unwind bounds and identical-code-folding
aliases. Windows symbols are obtained through Microsoft's symbol service;
local image size and PE timestamp match the recorded module signatures for
ntoskrnl, dxgkrnl, d3d11, kernel32 and ntdll. Image-load records with real
timestamps take precedence over later rundown records with timestamp zero.
Unresolved driver offsets are not assigned invented names, and an ICF alias
in a generic D3D wrapper is not interpreted as proof of that alias's API call.

## Three reproduced waits and their wake-up paths

### Small text vertex update: Direct3D allocation path

An existing 1,520-byte text-mask vertex-buffer unmap takes 3.1446 ms, including
3.1443 ms in the data-update branch. No buffer creation runs. It contains
2.8979 ms blocked, .0217 ms runnable and .2250 ms scheduled.

The first blocked interval is 2.5535 ms. The render-thread resumption stack
contains `QD3D11Buffer::beginFullDynamicBufferUpdateForCurrentFrame`,
`F4GeometryReuse::map`, Direct3D `NDXGI::CDevice::AllocateCB`,
`DxgkCreateAllocation`, `DXGDEVICE::CreateDriverAllocations` and
`DXGPUSHLOCK::AcquireExclusive`.

The matching ReadyThread event is emitted in another thread of this Qt host
(TID 60796). Its kernel stack exits `DXGDEVICE::CreateDriverAllocations` through
`ExReleasePushLockExclusiveEx` and wakes the render thread. Only a kernel stack
is available for this readier event; its precise application caller remains
unidentified. A generic live `QThread` name is not enough to call it a decoder
or buffer-prewarm worker.

Two later blocked intervals (.1117 and .2327 ms) resume in
`ExAcquireResourceSharedLite` under the same allocation path. Their readier
stacks are System-process worker threads releasing a resource. The exact lock
addresses and driver allocation objects are not captured. The stacks establish
the allocation/wake-up path, not a GPU execution-duration measurement. Context
switch stacks occur at resumption here; they are not mislabeled as switch-out
stacks. The exact D3D map type is not inferred from a template symbol name.

### Recreating a small smooth-color buffer: heap free versus codec teardown

A 3,840-byte vertex-buffer unmap takes 1.9155 ms. Creation/recreation accounts
for 1.9128 ms, with 1.8529 ms blocked. Its render-thread resumption stack is
`QD3D11Buffer::create` → `destroy` → `_free_base` → `RtlFreeHeap` →
`RtlpFreeHeap` → contended critical-section entry.

The ReadyThread stack in TID 76432 leaves a heap critical section while freeing
FFmpeg allocations: `VideoThumbnailRunner::run`, QMediaPlayer / FFmpeg playback
engine teardown, `avcodec_free_context`, `ff_frame_thread_free`,
`hevc_decode_free`, `pic_arrays_free`, `_aligned_free`, `_free_base`, and
`RtlFreeHeap`. This is concrete evidence linking the wait to concurrent codec
teardown's heap operation, rather than merely concurrent media activity.

### Another recreation: heap free versus demuxer allocation

An 8,640-byte vertex-buffer unmap takes 1.9106 ms. Creation/recreation is
1.9057 ms, including 1.8475 ms blocked in the same heap critical-section path.
Its readier, TID 38124 in the same host, leaves heap allocation code under
`QFFmpeg::Demuxer::doNextStep`, `av_read_frame`, MOV packet acquisition,
`av_grow_packet`, `av_buffer_alloc`, `av_malloc` and `_aligned_malloc`.

The two heap cases strongly support shared allocator contention. Heap handles
and critical-section addresses are not present, so exact lock-object identity
is not asserted. The Direct3D allocation wait is a separate observed path;
it must not be relabeled as the CRT heap wait.

## Targeted experiment: allocate CPU shadows only when needed

The current D3D11 backend allocates `dynBuf` for every Dynamic buffer and frees
it during destroy/recreate. The direct full-geometry update bypasses that CPU
shadow. The diagnostic candidate therefore leaves the shadow null for pure
Dynamic VertexBuffer or IndexBuffer objects, allocating it on the first batched
update or CPU read that requires it. Uniform buffers and combined usage flags
retain eager allocation. All three backend accesses to the CPU shadow are
covered. Native mapping, first-use/reuse proofs, resource ownership, requested
bytes and destruction remain otherwise unchanged. There is no new cache,
private allocator, global malloc replacement, setting or wire-protocol change.

The source transformation is reversible, and a same-executable flag selects
candidate/control. No production dependency recipe is changed.

Regression results:

- Direct geometry initially expects no shadow: both vertex/index rows fail in
  the control, as intended; remaining control behavior passes (7 pass / 2 fail).
- Candidate: **9 passes, zero failures/skips**. Repeated direct writes and
  recreate sizes 16, 64, 128 and 65,536 preserve GPU staging-read contents while
  keeping the shadow null. Batched full/partial writes preserve CPU and GPU
  contents. An initial CPU read materializes storage without asserting values
  for previously uninitialized data. Uniform and combined usage stay eager.
- Broader native rendering at actual DPR 1.75: **48 passes, zero failures/skips**.
- Shader/pixel checks: **27 passes, zero failures/skips** after setting the
  existing uniform-reuse feature flag required by that fixture's cleanup
  assertion. The first invocation records 26 passes / 1 cleanup failure and
  is retained; it is not reported as a passing feature-use check.
- Four embedded-import checks and the Windows system-DLL-only audit pass.

The existing direct-versus-batched API limitation is not broadened into a new
promise: a direct GPU write does not synchronize the old CPU shadow in the
original backend either. No new mixed-update semantics are claimed.

## Four clean comparisons: allocation savings do not prove smoother output

Fresh processes/caches, control/candidate/candidate/control. All preserve the
full folder and actual 160 Hz / DPR 1.75 setup. Each has two ten-second rapid
wheel blocks. No scheduler/stack trace, compiler, tests, analysis or UIA runs
during these comparisons. All input, source, activity, target PID, feature-use,
error and recorder-capacity guards pass. OS caches are not flushed.

| Arm | Preparation median, blocks 0 / 1 ms | Preparation p99, blocks 0 / 1 ms | Preparation maximum, blocks 0 / 1 ms | Misses, blocks 0 / 1 |
| --- | --- | --- | --- | --- |
| Control A1 | .7400 / .6783 | 3.0764 / 4.3444 | 4.9356 / 6.7364 | 2 / 5 |
| Candidate B1 | .7006 / .6674 | 3.1384 / 4.4775 | 5.9640 / 9.1527 | 1 / 7 |
| Candidate B2 | .7524 / .7118 | 3.1936 / 4.2391 | 6.2636 / 14.9990 | 7 / 12 |
| Control A2 | .7689 / .6865 | 3.1003 / 4.2373 | 6.7680 / 6.5230 | 3 / 5 |

The candidates skip 1,046 and 1,031 shadow allocations cumulatively across all
contexts/startup/input. Neither run materializes a shadow through batched writes
or reads. These are operation counts, not saved live bytes or main-thread-only
counts. Controls record no skipped allocations. The optimization is exercised.

Nevertheless, candidate totals are 8 and 19 missed refreshes versus 7 and 8
for controls. This does not establish an improvement. Candidate B2 reaches
14.9990 ms preparation, distributed over uploads (4.5205), alpha preparation
(2.5430), pre-body work (2.3169) and other phases. Those comparison frames do
not have scheduler stacks, so their CPU/wait split remains unknown.

Every block has zero coverage holds but fails maximum 2-physical-pixel motion
error; maxima range from 20.94 to 160.02 px. A missed refresh remains an interval
over 9.375 ms. The strict limits are unchanged. The separate intrusive trace
also fails (2/3 misses); it is not used as a clean comparator.

## Outcome and next evidence

The lazy-shadow candidate remains unpromoted. Eliminating these unused arrays
does not establish smooth 160 Hz, and a counter-only win is insufficient.
The stronger progress is identifying FFmpeg heap operations in actual wake-up
stacks and separately locating the Direct3D allocation lock path.

A future allocator/context-lifetime change should target the measured bulk
codec allocation/teardown boundary, retain cancellation and image priorities,
and preserve alignment, realloc, ownership, failure and cross-thread lifetime
contracts. Merely moving decoder work to another thread is insufficient: it
already runs outside the render thread. Likewise, the application caller of
TID 60796 must be established before changing GPU worker scheduling. Neither
an allocator-isolation implementation nor a codec reuse cache is implemented
or claimed here.

## Reproduction and preservation

Wait capture: cache zr, stem
`real-scroll-unmap-wait-stacks-20261029-wheel-masonry`. Comparison caches zs,
zt, zu, zv correspond to `real-scroll-lazy-shadow-{a1,b1,b2,a2}-20261029-wheel-masonry`.

Main scripts are `prepare-unmap-wait-readers-20261029.py`,
`prepare-unmap-wait-source-context-20261029.py`,
`capture-unmap-wait-stacks-20261029.py`,
`analyze-unmap-wait-stacks-20261029.py`,
`resolve-unmap-wait-windows-symbols-20261029.py`,
`build-lazy-geometry-shadow-20261029.py` and
`analyze-lazy-geometry-shadow-20261029.py` under `artifacts`.

Exact selected stacks and module proofs are in
`unmap-wait-stacks-20261029/{selected-stack-attribution,windows-symbol-attribution,
readier-identity-verification}.json`. Source-context CSV, reader metadata,
regeneration proof, application map verification and all test logs are retained.
Compressed ETL, extracted ETL and selected-stack JSONL remain on C: with paths
and hashes in metadata. The heap-readier thread that exited before the live
name query is not dropped; its trace identities and stacks remain available.

Lossless NTFS compression of inactive functional-test binaries and eight older
large CSV exports recovers space without deleting data. Before/after SHA256
checks and manifests are retained. None of the hosts used in these timed
comparisons is compressed as part of that recovery. The normal worktree app
remains alive; only the exact diagnostic launcher tree is stopped afterward.

AI assistance has been used to create this output.
