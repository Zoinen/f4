# Maximized-startup CPU attribution, 2026-10-31

The corrected maximized-startup trace points to repeated CPU-side render work,
rather than the long standalone-texture allocation waits found in the earlier
small-startup diagnostic. This is attribution evidence; steady 160 Hz remains
unproven.

## Scope and provenance

Captured Downloads, 192 entries including parent, Masonry density 152, actual
DPR 1.75 and display refresh 160 Hz. Every input retains the scope fields and
foreground state. Startup restores maximized geometry before renderer creation,
using the owned diagnostic profile. The normal app and its settings remain
unchanged. No external workloads, VMs or affinities were changed.

The diagnostic Go launcher has the same bytes as the current packaged app.
The native host is linked from the current production objects and libraries,
including bounded ordering scratch reuse; only main.cpp carries the frame/input
recorder. A linker map adds no runtime renderer override. Host SHA-256:
cac89f44e50a880e300a4143527b46ea8357c10dc41c1a72ce9b64c085cd8320.

The native input recording is 30 seconds. A kernel Profile+CSwitch+ReadyThread
trace covers the first eight input seconds plus surrounding time. Stacks,
interrupts, context switches and PresentMon add overhead; this is not a clean
performance acceptance capture. No QML-debug profiler was used for these native
wait/copy questions. The normal app remained alive throughout.

## Trace validation

The ETL contains 2,606,871 events with zero lost events, lost buffers, schema
errors or ProcessTrace errors. Raw SHA-256:
0233e5efcec75180fd00bb470c07790392f3f650170023fe86b1c2055b0f324e.

The sampled-profile reader uses its documented prefix fallback because TDH did
not provide the profile schema. An independent xperf dump verified all 248
selected reference rows by thread, instruction pointer and timestamp. All 8678
selected thread switch-ins have an exact same-thread/same-QPC stack; joining
switch-outs instead produces only three coincidental matches. No nearest-time
stack matching was used. The first verifier invocation rejected the capture
because it inherited an arbitrary >10000 sample-count assertion from a longer
trace. Replaced that assertion with nonzero count and 100% exact coverage. No
capture was rerun or discarded. Zero selected waits is a valid result, not a
requirement to manufacture a nonempty wait list.

Symbol resolution checks the executable/map hashes and PE image/text ranges,
and follows PE unwind ranges. Identical-code-folded aliases remain labeled
ambiguous; symbols such as $$000000 are not presented as a known function.

## CPU and wait evidence

Seven render signal ranges exceed 4 ms in the traced first eight seconds:
4.1723, 4.9840, 4.1290, 4.4553, 4.3735, 4.0871 and 4.1214 ms.
None contains an off-CPU interval overlapping it by 0.2 ms or more. For example,
the 4.9840-ms range is fully on CPU; the 4.3735-ms range has 4.3699 ms on CPU
and 0.0036 ms off CPU. Scheduling reconstruction verifies on+off coverage of
the measured frame/range to within 0.001 ms.

There are 2250 render-thread and 2287 GUI-thread sampled profiles in those eight
seconds, with zero samples missing a stack. Counts are statistical evidence,
not exact exclusive duration. Inclusive counts overlap and must not be added.

| Render work | Leaf samples | Inclusive samples where available |
| --- | ---: | ---: |
| QSGNodeUpdater::isNodeBlocked | 110 | 123 |
| F4VectorGateFrame constructor | 106 | 123 |
| StageUniformBufferBatches copy constructor | 96 | 96 |
| ResourceBatches copy constructor | 54 | 150 |
| QRhiBatchedBindings<unsigned int> destructor | 59 | — |
| QRhiBufferData::assign | 57 | — |
| updateMaterialDynamicData | 54 | 268 |

The render profile has 907 inclusive samples in prepareRenderPass, 464 in
recordRenderPass and 390 in setShaderResources. Samples in the slow ranges
include scene traversal/render-list rebuilding, batch buffer uploads, material
preparation, binding snapshot copies, and driver/kernel work. Sampling does not
prove that any single one causes every missed refresh.

The GUI profile includes 808 samples in GalleryScrollAnimation::syncNativePose,
768 in MasonryLayout::updateProperties and 208 in bindDelegateRowVisual. These
are inclusive. The existing snapshot/revision guards are present in the source;
the profile alone does not prove every observed visual update is redundant.

## Display results and limits

The 30-second input interval contains 4798 present rows, one dropped row and
three display-change intervals above 9.375 ms. Maximum display-change interval
is 18.7546 ms. Render signal-range p99 is 3.0093 ms and maximum is 6.0248 ms.
All outliers and the dropped row are retained. One long display interval occurs
outside the kernel trace; this trace cannot attribute its cause.

PresentMon display-change notifications are not optical scanout proof. Frame
range, CPU execution, render duration and display interval are separate metrics.
This run does not verify cold cache, all-192-entry quiescent warm tour, physical
motion error, coverage holds, or quarter-height middle-button scrolling. No
claim of full performance acceptance or a reliable before/after gain is made.

## Next change to evaluate

Per-command D3D11 binding snapshots are a concrete optimization candidate: the
current Qt private header appends the complete ResourceBatches structure, and
the executing code checks stage present flags. The current profile establishes
that copies and their cleanup consume render CPU. An earlier owned prototype
copies only present stages while retaining command-owned snapshots; it must be
validated against the current production graph, including recording lifetime,
source mutation, all stages, graphics/compute, and rendered output, before
rollout. Do not replace retained snapshots with pointers to mutable builders.

The vector gate's per-frame ancestor walks are another measured candidate.
Hidden retained SVG surfaces deliberately revalidate residency, so skipping that
work without lifecycle/order tests would change correctness. No such policy
change was made here. No production code, normal executable, commit or push was
changed during this investigation.

Scripts, analyses, validation receipts and a hash index are frozen beside this
report; raw ETL/CSV/JSON and executable references remain externally preserved.

> AI assistance has been used to create this output.
