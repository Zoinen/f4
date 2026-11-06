# Graphics-wait stacks locate allocation and driver-lock contention

A lossless cold-scroll stack trace locates the previously measured D3D11
blocked intervals in Windows graphics-memory allocation paths and NVIDIA
user-mode critical-section waits. This changes the next investigation from
generic resource timings to specific allocation/lock paths. It does not identify
the lock holder, establish a driver defect, or complete constant 160 Hz.

## Capture validity

The existing resource-identity diagnostic host runs with default scheduling,
four configured decode workers, original atlas uploads, default media behavior,
a fresh empty application cache, and the actual Downloads folder (190 entries).
Both ten-second rapid-wheel blocks run at 160 Hz and actual DPR 1.75. No builds,
tests, UI Automation observation or offline analysis run during input.

An owned kernel session records process/thread, loader, context-switch and
dispatcher events, with CSwitch/ReadyThread stacks. It uses a 2-GiB sequential
file cap, bounded buffers and an ownership guard that refuses an existing kernel
session and stops only its own. It reports zero lost events/buffers, zero schema
errors and successful ProcessTrace. Raw trace size is 670,040,064 bytes;
verified gzip size is 120,854,096 bytes. The raw ETL was removed only after
decompression SHA256 verification. The compressed trace remains in artifacts.

Qt PID is 69840; render thread 97616; GUI thread 79348. The decoder exports
656,719 stack records for the target process and 754 target/kernel module rows.
The cache-activity observer's longest callback is 27.8 microseconds. System-wide
stack walking itself adds overhead that is not quantified by this callback.
Use the capture for attribution, not a performance comparison with lighter
captures. Similar API stalls were already reproduced without stack walking.

| Block | Display gaps >9.375 ms | p99 display interval | Maximum interval | Coverage holds |
| --- | ---: | ---: | ---: | ---: |
| 0 | 13 | 6.7124 ms | 18.7755 ms | 0 |
| 1 | 0 | 6.3565 ms | 6.4439 ms | 0 |

These are diagnostic results, not a strict motion pass.

## Stack/event association and symbol validation

The decoder uses each StackWalk payload's original-event timestamp, as specified
by [Microsoft's StackWalk event schema](https://learn.microsoft.com/en-us/windows/win32/etw/stackwalk-event).
It checks 64-bit shape and filters by process. A synthetic fixture verifies
timestamp/thread/address decoding, malformed-length rejection and process
filtering. It is not a comprehensive ETW schema test suite.

The initial analysis searched for render-thread switch-out stacks and found
none. Actual event inspection shows exact timestamp matches with render-thread
switch-in events: 6,834 new-thread matches and zero old-thread matches. The
corrected analysis associates stacks by exact original-event QPC and thread ID
at resumptions inside each measured API interval. It does not use nearest-time
matching. Kernel and user stack records can share the same timestamp; both are
retained. The API timing bracket and scheduler spans independently establish
the enclosing operation and blocked duration.

Module identity comes from the same trace's image base/size/path and timestamp.
All 64 selected addresses from seven Windows modules resolve with exact public
symbols, fetched from Microsoft's symbol server into an absolute local cache.
The initial relative-cache-path symbol attempt failed and is not used. The
final resolver rejects unresolved addresses; its successful run and module-file
hashes are retained. NVIDIA frames remain module+offset because no private
driver symbols are available. Function names are not invented for those frames.

## Measured waits and resolved paths

The following spans belong to the same trace; they are separate calls and must
not be summed into a single frame.

| Operation / phase | Wall | Scheduled | Blocked | Runnable |
| --- | ---: | ---: | ---: | ---: |
| Map pending dynamic buffer / recording | 10.7932 ms | 0.3082 ms | 10.4683 ms | 0.0167 ms |
| UpdateSubresource / submission | 10.1701 ms | 0.2389 ms | 9.9060 ms | 0.0252 ms |
| UpdateSubresource / submission | 8.7336 ms | 0.1430 ms | 8.5831 ms | 0.0075 ms |
| Map full dynamic-buffer update / preparation | 3.9667 ms | 0.0410 ms | 3.9182 ms | 0.0075 ms |

For the 10.7932-ms Map, the user stack contains the NVIDIA driver, D3D11's Map,
`NDXGI::CDevice::AllocateCB`, `D3DKMTCreateAllocation` and
`NtGdiDdDDICreateAllocation`. The corresponding kernel stack contains:

```
DXGDEVICE::CreateAllocation
DXGDEVICE::CreateDriverAllocations
DXGPUSHLOCK::AcquireExclusive
ExAcquirePushLockExclusiveEx
ExfAcquirePushLockExclusiveEx
KeWaitForSingleObject
```

The 10.1701-ms upload has two resumptions. One shares that allocation-lock path;
the other contains `NtGdiDdDDILock2`, `VidMmLock`, `VIDMM_GLOBAL::LockCommon`,
`VIDMM_RECYCLE_HEAP_MGR::ZeroIfNeeded` and `DXGAUTOMUTEX::Acquire`.
Several other 3.4-8.7-ms uploads/allocations share the CreateDriverAllocations
path. This identifies allocation-related blocking inside calls that appear
to be only a map or small upload at the Qt API boundary.

The 3.9667-ms full-update Map and another 3.8051-ms Map instead have user stacks
through NVIDIA frames into `RtlEnterCriticalSection`,
`RtlpEnterCriticalSectionContended`, `RtlpWaitOnCriticalSection` and
`NtWaitForAlertByThreadId`. These are driver critical-section waits; the trace
does not expose their lock object or owner. They must not be relabeled as GPU
execution, QML mutex waits or decoded CPU work.

The first 13 selected slow API intervals have exact matching resumption stacks.
The full symbolized subset, API timings, scheduler spans and original raw-address
records remain available for scrutiny. Symbol displacements are retained; public
symbols do not provide private implementation details or exact source lines.

## Consequences and next control

The evidence supports avoiding or reducing graphics-allocation contention,
rather than adding layout rounding or merely changing atlas dimensions.
The preceding smaller-atlas, combined-upload and serialized-decode controls did
not deliver constant 160 Hz with acceptable thumbnail preparation. Keep all
three out of production.

Next validate an alternate Qt graphics backend's native rendering and then
compare the same cold workload with default decode concurrency and full media
support. That is a diagnostic control for these D3D11 paths, not authorization
to drop platform compatibility or switch the production backend without evidence.
Device/context identity and allocation lifetime/reuse remain useful follow-ups
if the stalls persist. No lock owner or responsible concurrent producer has
been proven yet.

No production source, dependency archive or embedded payload changed in this
step. Only the exact resource-diagnostic launcher tree was closed. The normal
portable worktree launcher 43936 and Qt 119460 remain running. No commit/push.

Evidence: `profiler/experiments/masonry-graphics-wait-stacks-2026-10-27/` contains
the reader/resolver sources and build scripts, fixture result, selected stacks,
symbol resolution and module hashes, capture packet and presentation/scheduler
data. Large full-stack CSV, compressed ETL, executable tools and symbol-cache
files remain in artifacts with external hash references.

> AI assistance has been used to create this output.
