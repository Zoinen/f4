# Masonry: combined-build CPU and presentation-wait attribution

## Outcome

Constant 160 Hz without spikes remains **unachieved**. This turn records one
new, intrusive scheduler/CPU trace of the combined private-heap and
thumbnail-decoder-device-reuse build. No production implementation or portable
payload is changed, and no commit/push is made.

The trace separates two remaining costs:

1. **Preparation CPU work.** Across 3,198 measured preparations, 3,201.6306 ms
   of 3,206.0741 ms elapses while the render thread is scheduled. The principal
   sampled self hotspot is `QSGNodeUpdater::isNodeBlocked`, called from
   `QSGRenderer::preprocess`. The custom vector-gate constructor, node traversal
   and material/buffer preparation are further hotspots.
2. **A submission wait.** One render frame lasts 15.3367 ms, including
   12.5945 ms blocked after rendering. The render thread resumes in the
   NVIDIA/D3D11/DXGI `Present` path after a critical-section wait. Its NVIDIA
   worker is itself blocked in graphics allocation; that worker is woken by
   another application thread during graphics-context creation for
   `QVideoFrame::toImage()` conversion.

The conversion devices were identified in earlier work. This trace adds an
actual scheduling/wake-up chain in the **combined build**; it is not a claim
that conversion-device existence is a new discovery or that a lane alone is
now proven beneficial.

## Capture and provenance

Case: `real-scroll-combined-av-cpu-20261029-wheel-masonry`.
Qt PID 44836, render thread 55152, GUI thread 34780.
Actual Downloads, all 191 entries, Masonry density 152, 3840×2160 / 160 Hz /
actual DPR 1.75, two ten-second rapid-wheel blocks, initially empty isolated
gallery cache `cold-gallery-cache-20261027-zzi`. Normal cold-scrollbar setup is
retained. OS file caches are not flushed. This is not a verified-warm capture.

Host: `artifacts/f4-qt-host-av-thumbnail-device-reuse-20261029.exe`, the exact
already-tested host used by the preceding combined comparison. Both diagnostic
features are enabled. No new native executable is built for this trace.

The host reports one thumbnail decoder-device creation, 31 reuse hits, no
device or reference failures, 642,340 private allocations and 642,336 frees.
Four live allocations totaling 184 bytes belong to the intentionally retained
device context. As already verified by the combined decode regression, clearing
that context after its clients finish releases the tracked allocations.
This byte count does not include native GPU/driver resources.

The original 100-entry asynchronous buffer pool belongs to the main render
thread only; it records 1,361 creates. Offscreen video conversion did not
accidentally acquire another such prewarm pool. Other existing renderer,
resource-bound, content, display, input/focus and error guards pass.

The owned NT Kernel Logger records `PROC_THREAD+LOADER+CSWITCH+DISPATCHER+PROFILE`
with `Profile+CSwitch+ReadyThread` stacks. It is stopped only after verifying its
trace path still belongs to this capture. No tests, builds, UI Automation
observation or offline analysis run during the gestures. The existing sampling
interval is not changed; sample counts below are not converted into CPU ms.

- Trace size: 720,109,568 bytes; gzip archive: 134,546,383 bytes.
- SHA256: `28f42eb9cd58b88e63b4d70d1d52a7317c688a94db617ed89caa5edd3a61bdd0`.
- 8,486,264 ETW events, 2,616,589 context switches, 1,294,714 ready events.
- Zero lost events/buffers, schema errors or ProcessTrace errors.
- Scheduler frequency: 10,000,000 ticks/second.
- Scheduler reader reports one published-schema fallback. Raw ReadyThread
  header source PID/TID values are invalid sentinels; reconstructed CPU context
  is independently checked against the StackWalk payload PID/TID at both
  wake-up hops discussed below.
- Main/GUI stack export: 142,033 stacks and 20,097 CPU samples.
- CPU prefix fallback is independently verified against **467 xperf-decoded
  samples**, matching thread, IP and timestamp with no mismatches.
- All 3,107 CPU samples that fall within measured render preparation join
  corresponding stack fragments containing their sampled instruction pointer.

The host map was produced by its original build. Its executable/map hashes
match the captured build receipt; preferred image base and PE timestamp agree.
`.text` and `.pdata` hashes are recorded. Application symbols use runtime
function/unwind bounds and preserve ICF ambiguity instead of inventing a
specific aliased callback. Windows symbols require matching image timestamps
and sizes. NVIDIA private frames remain offsets.

Live presentation/motion results from this intrusive run:

| Block | Preparation max | Display max | Display intervals >9.375 ms | Max motion error, physical px |
| --- | ---: | ---: | ---: | ---: |
| 0 | 5.4428 ms | 12.6459 ms | 4 | 116.013 |
| 1 | 4.6090 ms | 6.5504 ms | 0 | 5.906 |

Both blocks have zero coverage holds. Both fail the original maximum motion
error requirement. The 13.7892 ms preparation tail in the preceding clean
comparison is **not reproduced** by this trace; absence in this one run does
not establish that it is fixed. Intrusive tracing changes the workload and is
not a clean before/after performance comparison.

## Preparation scheduling and CPU samples

Preparation totals:

| State | Time |
| --- | ---: |
| Scheduled on CPU | 3,201.6306 ms |
| Runnable, awaiting CPU | 3.4186 ms |
| Blocked | 1.0249 ms |
| Unknown | 0 |
| Total wall | 3,206.0741 ms |

Scheduled time is approximately 99.86% of preparation wall time. This accounting
includes interrupts/DPC execution on that CPU; their durations are not
independently captured. It is not a claim of pure user-instruction service.
The sampled application stacks separately establish substantial renderer work.

The longest preparation is 5.4428 ms: 5.4380 ms scheduled, 0.0048 ms runnable,
zero blocked. Its larger measured phases are before-body 1.3063 ms, geometry
uploads 1.2582 ms, render lists 1.2065 ms and alpha materials 0.9341 ms.
Another 5.3767 ms preparation and its 2.9088 ms recording phase contribute to
a fully scheduled 10.0343 ms render span. Thus preparation is not the entire
per-frame CPU budget.

Measured scheduled time by selected preparation scope:

| Scope | Scheduled time |
| --- | ---: |
| Before renderer body, including preprocessing/update | 909.52 ms |
| Alpha-material preparation | 887.38 ms |
| Geometry uploads | 556.22 ms |
| Render lists | 292.64 ms |
| Vector-gate setup | 263.08 ms |
| Batching | 112.97 ms |

Selected **self sample counts**, out of 3,107 preparation CPU samples:

| Function | Samples |
| --- | ---: |
| `QSGNodeUpdater::isNodeBlocked` | 367 |
| `F4VectorGateFrame` constructor | 183 |
| Batch renderer `Updater::visitNode` | 167 |
| `Renderer::updateMaterialDynamicData` | 123 |
| `QRhiBufferData::assign` | 121 |
| `Renderer::prepareRenderMergedBatch` | 72 |
| `Updater::visitTransformNode` | 70 |

All 367 self samples in `isNodeBlocked` have `QSGRenderer::preprocess` as their
next caller. They are not sampled from node-addition or render-list checks.
The source shows a loop over the copied preprocessing set and an ancestor walk
for each eligible node; its deletion-during-callback guard must be retained.
The counts are samples, not invocation counts or exclusive milliseconds.

All 183 self samples in the vector-gate constructor come from
`Renderer::prepareRenderPass`. Its source scans batches and walks ancestors
to locate owning vector gates; incomplete/pending sources can cause a further
membership pass. The current logic protects residency and delayed batches.
It cannot be removed merely because a source has a cached CPU image.

The prior bounded ancestor-hash-cache experiment regressed preparation and
remains rejected. This attribution supports targeting preprocessing eligibility
and repeated gate work at their callers, rather than reapplying the same cache.
Any candidate must preserve callback deletion/reparenting, dynamic subtree
blocking, opacity, root-exclusion semantics, source generations, authored
animation and batch-residency checks.

## Submission wait and transitive wake-up chain

One frame starts rendering at 1017146300263500 ns. Its preparation takes
1.7759 ms and recording 0.6458 ms, neither blocked. Submission then takes
12.9137 ms, including a 12.5945 ms wait. The full render span is 15.3367 ms.
That span is not itself a measured display interval; queued presentation can
have a different displayed interval.

1. **Render thread 55152.** It waits from QPC 10171463029289 until ReadyThread
   at 10171463155234. Its resumption stack includes
   `NtWaitForAlertByThreadId → RtlpWaitOnCriticalSection → RtlEnterCriticalSection`,
   NVIDIA user-mode frames, `NDXGI::CDevice::PresentImpl`, DXGI flip-presentation
   code and `QRhiD3D11::endFrame`.
2. **NVIDIA worker 39824.** The render-thread ReadyThread CPU context and exact
   StackWalk payload both identify this thread in PID 44836. Its wake stack
   includes NVIDIA frames and `RtlLeaveCriticalSection → RtlpWakeByAddress →
   NtAlertThreadByThreadIdEx`. It is not executing a Qt/FFmpeg application
   thread entry in that stack. Across the render wait, this worker is scheduled
   for 0.1210 ms, runnable for 0.0084 ms and blocked for **12.4651 ms**. There are
   no CPU samples for it within that interval; this is not evidence of twelve
   milliseconds of shader compilation on that worker.
3. **Graphics allocation.** The worker's own resumption stack shows
   `DXGPUSHLOCK::AcquireExclusive → DXGDEVICE::CreateDriverAllocations →
   DxgkCreateAllocation`, below NVIDIA/D3D11 allocation callbacks.
4. **Conversion thread 98984.** At QPC 10171463153940 this application thread
   readies the NVIDIA worker. CPU context again agrees with the exact stack
   payload PID/TID. The ReadyThread kernel stack includes a push-lock release
   in `ADAPTER_RENDER::DdiCreateContext → DXGCONTEXT::Initialize →
   DXGDEVICE::CreateContext → DxgkCreateContextVirtual`.
5. **Nearby user context.** At +0.1596 ms, and in subsequent nearby switch-in
   user fragments, thread 98984 is in:

   ```
   QRhiD3D11::create
     QRhi::create
       ThreadLocalRhiHolder::ensureRhi
         qImageFromVideoFrame
           QVideoFrame::toImage
             VideoFrameGeometry::displayImage
               VideoThumbnailRunner::run
   ```

This connects the submission wait to concurrent graphics-context creation in
the existing video conversion path. The decoder-device cache does not replace
Qt's separate thread-local conversion QRhi. Kernel/critical-section object
addresses are not captured, so the data does not prove one exact lock object
or that thread 98984 owned it throughout the whole 12.4651 ms interval.
Template/ICF callback labels are not evidence that an overlay-query API was
actually called.

NVIDIA private function names are unresolved. Its published
[driver-symbol-server description](https://developer.nvidia.com/blog/nvidia-driver-symbol-server-now-available/)
states that it distributes binaries rather than PDBs. Offsets are retained;
no private driver function names are guessed. No global driver setting or
device-threading flag is changed. GPU execution time is not measured by this
trace, and these waits do not by themselves prove GPU saturation or VRAM pressure.

## Follow-up direction

Review conversion **resource lifetime/reuse** on the earlier pixel-tested
dedicated lane, including device creation before interactive rendering and
bounded compatible pipeline/buffer/readback reuse. Preserve the GPU reference's
color, alpha, crop, rotation and mirroring behavior, ownership and cancellation.
The prior lane-only comparison did not establish a scrolling benefit; it is
not repeated or promoted here. The private CPU converter also remains rejected
as a universal replacement because it failed tested color/alpha equivalence.

For preparation, investigate incremental preprocessing eligibility and gate
membership at the confirmed callers. Implement only after a failing regression
demonstrates the behavior being changed, then verify scene/pixel fidelity and
actual performance. A new per-frame table of rounding/hash work is not justified
by this result.

Cold loading, verified-warm rapid wheel and quarter-height middle scrolling
still need all original zero-miss, motion-error and coverage gates. No success
criterion is narrowed to this trace or to a passing median/p99.

## Evidence

Sources, analyses, raw-prefix references, decoded scheduling accounting and
symbol proofs are under `artifacts/combined-av-cpu-20261029/`. Capture packets
are under `artifacts/modes-profile/`. ETL, its verified archive and selected
stack/scheduler exports are retained in
`C:/Users/xs/AppData/Local/Temp/f4-combined-av-cpu-20261029/`.

Frozen evidence:
`profiler/experiments/masonry-combined-cpu-and-present-waits-2026-10-29/`.
The normal worktree application is preserved. Only the exact diagnostic
launcher process tree is terminated after analysis.

AI assistance has been used to create this output.
