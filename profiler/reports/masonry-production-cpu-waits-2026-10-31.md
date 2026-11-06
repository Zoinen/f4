# Masonry production graph: CPU and wait attribution, 2026-10-31

The 160 Hz goal remains incomplete. Two additional traces identify concrete
work and waits in the actual production renderer. They are intrusive attribution
runs and do not replace the clean baseline or any original acceptance gate.

## Build and measurement scope

The diagnostic host links the same production CMake objects and libraries as
the normal portable app, replacing only main.cpp to attach the existing bounded
frame/input recorder. This version adds GUI/render thread IDs and a linker map.
There are no experimental renderer, decoder, gallery-controller or model object
overrides. Host SHA256:
8d55193782b33dc100bc77ab031da159f2a0edee1e4b91fdf2173bd2446fba2b.

Each input run lasts 30 seconds in the actual 192-entry Downloads folder,
Masonry density 152, on the local 3840 by 2160, 160 Hz display at DPR 1.75.
Every wheel receipt records the actual density, mode, entry count, refresh,
DPR, acceptance and foreground state. Both test panels show Downloads.

The kernel trace covers the beginning of each run and contains the complete
first eight input seconds. It deliberately stops before the 30-second input
run ends. The first trace records Profile stacks and scheduling; the second
also records CSwitch/ReadyThread stacks. No UI Automation or screenshots run
during input. Native signals, input timers, CPU sampling, context switches and
PresentMon all add observation overhead. Actual 10 ms wheel-timer delivery
continues to be irregular; it is not represented as an exact 100 Hz stream.

Neither trace establishes a cold cache, all-192-entry quiescent warm tour,
quarter-height middle motion, physical motion error, zero coverage holds,
middle velocity tolerance or static physical-pixel alignment. All those
original requirements remain unchanged.

## Trace and decoder verification

| Evidence | Profile trace a1 | Wait trace b1 |
| --- | ---: | ---: |
| Raw ETL bytes | 137625600 | 288882688 |
| Events decoded | 2808091 | 3466595 |
| Selected profile samples | 7285 | 8432 |
| Selected stack fragments | 7819 | 103238 |
| Event/buffer/schema errors | 0 | 0 |
| Profile rows independently checked against xperf | 279/279 | 354/354 |

TDH does not provide the sampled-profile schema here. The fallback prefix is
self-tested and independently checked against xperf by thread ID, instruction
pointer and timestamp, with no unmatched reference rows. The stack prefix is
also checked against TDH properties. Symbols use the hash-checked linker map,
PE image base and function/unwind ranges. Folded-code aliases remain explicit;
an arbitrary nearest public symbol is not accepted across a runtime function
boundary. System/driver frames retain module offsets rather than invented
function names.

The wait trace's CSwitch stack is for the incoming NewThreadId at switch-in.
All 17088 selected incoming switches have exact thread/timestamp stack matches;
only five outgoing switches happen to match. Each selected wait therefore joins
its resume stack at endQpc. No nearest-timestamp substitution is used. The
first attempted switch-out join, which found no selected wait fragments, is
retained as invalid-switchout-join evidence.

An initial scheduling analyzer also used the wrong CSV event letter C instead
of S and produced zero accounted time. That invalid output is retained. The
corrected analyzer asserts that on-CPU and off-CPU intervals cover every
analyzed range within 0.001 ms; the zero-accounting result cannot pass it.

## Concrete wait sites

Twelve waits of at least 0.2 ms overlap rendering ranges above 4 ms during
the first eight seconds of b1. Each has two exact resume-stack fragments.

| Rendering-range start after input | Wait overlap | Resuming application call |
| --- | ---: | --- |
| 593.52 ms | 1.9722 ms | QD3D11Texture::create, through QSGPlainTexture and ShaderEffect sampled-image update |
| 646.53 ms | 1.5819 ms | QD3D11Buffer::create, through batch unmap/upload |
| 673.57 ms | 9.3774 ms | QD3D11Texture::create, through QSGPlainTexture and opaque texture material |
| 694.71 ms | 2.5701 ms | Full dynamic geometry-buffer update, through batch unmap/upload |
| 752.78 ms | 2.6562 ms | Full dynamic geometry-buffer update, through batch unmap/upload |
| 1015.63 ms | 3.5333 ms | Heap allocation in f4UnmergedPositions ordering snapshot |
| 1315.20 ms | 1.7318 ms | Full dynamic geometry-buffer update, through batch unmap/upload |
| 1571.66 ms | 2.5042 ms | Heap/hash allocation in F4VectorGateFrame |
| 1690.93 ms | 5.6481 ms | QRhiD3D11::executeBufferHostWrites during shader-resource binding |
| 1827.82 ms | 1.8956 ms | Heap allocation in f4UnmergedPositions ordering snapshot |
| 1952.74 ms | 5.7051 ms | Full dynamic geometry-buffer update, through batch unmap/upload |
| 7653.00 ms | 0.2544 ms | Full dynamic geometry-buffer update, through batch unmap/upload |

The largest rendering range lasts 12.4526 ms, including the 9.3774 ms texture
creation wait. Its resume stack includes the graphics kernel and Nvidia/D3D11
resource path. This identifies the blocking API, not the owner of the kernel
lock or the exact image/file. The two texture waits use different material
paths. Describing both as proven thumbnail-shader extraction would be too
strong; source ownership and redundant extraction still need measurement.

These observed durations are time off CPU, not nine milliseconds of arithmetic
or a proven scheduling-ready delay. The first Profile-only trace separately
reproduces a 7.0583 ms rendering range with 2.5994 ms on CPU and 4.4589 ms off CPU.

## CPU work outside individual waits

During a1's first eight input seconds, all 2427 selected render samples and
2671 GUI samples have stacks. Inclusive render counts include prepareRenderPass
939, recordRenderPass 553, shader-resource binding 437, merged-batch preparation
338 and material-dynamic-data update 238. These overlap and must not be summed
as exclusive costs.

Notable leaf samples include F4VectorGateFrame construction (124), copying
stage-uniform bindings (119), node blocking checks (112), binding-batch cleanup
(71), scene traversal (63) and copying resource batches (59). Public symbols
inside folded template bodies retain their alias warning.

On the GUI thread, 776 samples include syncNativePose, 780 include contentY
publication and 744 include layout updateProperties. This shows substantial
publication work, but it does not by itself prove every update is redundant.

## What this changes next

1. Start with bounded reuse of the renderer's unmerged-order scratch storage.
   It is application-owned patch code and has two measured heap waits. Preserve
   painter ordering, exact membership comparisons, concurrent renderer isolation
   and fallback for oversized/nested rebuilds. Add a regression before changing
   it and verify native rendering and 175% captures. Keep portable recipe and
   the actual production build consistent.
2. Determine the source identities and dimensions of plain textures created
   during first scrolling. Check atlas-aware thumbnail sampling and avoid
   redundant atlas extraction where proven. Do not assume the opaque-material
   texture wait has the same owner as the ShaderEffect wait.
3. Reduce buffer allocation/renaming and per-draw binding copies using bounded
   ownership and GPU-completion evidence. Reuse may not overwrite in-flight
   buffers or change shader bindings. No driver/OS workaround is established.
4. Rerun the original clean cold/warm/middle gates and the residual warm
   presentation trace. These CPU traces do not explain every clean-baseline
   miss, and the steady-state DWM/driver question remains open.

The intrusive a1/b1 runs report 8/13 display intervals over 9.375 ms, maxima
12.529/25.858 ms. They remain failures, not new acceptance thresholds.

## Evidence and application state

Artifacts: artifacts/production-cpu-recorder-20261031, with b1 decoder and wait
analysis under wait-b1. Raw traces, scheduler CSVs, selected stacks and xperf
references remain in the owned C: temporary evidence folder
f4-production-cpu-20261031. The complete map, final executable and raw traces
are retained through external hash references; compact evidence is frozen
beside this report. No failed result, stack, outlier or trace was pruned.

The diagnostic process trees were stopped by exact executable path and owned
parent identity. The unchanged normal worktree application remains Go PID
103648 / Qt PID 17748, with Downloads left and its existing P: photo directory
right, and foreground focus restored. No external workload, VM, affinity or
desktop setting was changed. No product source change, commit or push was made
in this attribution step.

AI assistance has been used to create this output.
