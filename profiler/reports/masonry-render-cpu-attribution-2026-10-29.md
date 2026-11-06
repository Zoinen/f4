# Masonry render CPU and observer controls — 2026-10-29

## Result

The previous goal turn made concrete progress through a source-admission fix,
verified warm captures and upload-phase attribution. This continuation adds
lossless scheduling evidence, independently verified sampled CPU stacks and
two observer controls. Constant 160 Hz without spikes is still not achieved.
No new performance change is promoted, no portable payload is replaced, and
no commit or push is performed.

All five additional captures use the real Downloads folder, Masonry density
152, all 191 entries, actual 3840×2160 at 160 Hz and DPR 1.75. Each has two
ten-second rapid wheel blocks, a fresh isolated cache, normal viewport
persistence and cold loading. There is no hidden/reduced scene or warm-tour
substitution. Capture guards pass. No build, offline analysis or UI Automation
observer runs during input. The user's standing permission covers all captures.

## Scheduler evidence

`real-scroll-vector-admission-scheduler-20261029-wheel-masonry` enables only
PROC_THREAD, CSWITCH and DISPATCHER in an owned kernel session. The decoded trace
reports zero lost events/buffers, schema errors or ProcessTrace errors and
covers both complete measurement windows. It includes render, GUI and DWM
threads. The session is stopped only after checking its exact trace ownership.

The largest preparation spans are 5.4836 ms (5.4800 scheduled, .0036 runnable,
zero blocked) and 6.6004 ms (entirely scheduled). Render-to-frame-end peaks are
8.8476 and 9.8129 ms, also almost entirely scheduled. The largest submission
span in block 0 is different: 5.7043 ms, including 5.2309 ms blocked and a
5.4228 ms recorded Present call. Scheduled time may include driver/kernel work;
it is not synonymous with pure application CPU instructions or GPU time.

The earlier 3.0271 ms unmap for an 11,520-byte smooth-color vertex batch does
not recur. The largest unmaps in this scheduler capture are .3526 and .3416 ms,
both entirely scheduled. Their thread states cannot be transferred to the
earlier untraced frame. Source inspection also corrects a previous shorthand:
unmap includes pool selection, possible buffer creation/recreation, and buffer
update enqueue/copy; its full wall span must not be called isolated enqueue cost.

## Observer controls and display results

The RHI probe records millions of calls, so a same-executable runtime control
sets `F4_PROFILE_DISABLE_RHI_CALLS`. Its zero call/resource/producer records are
asserted. All rendering/ownership/admission behavior and other probes remain.

A second diagnostic host restores the exact original Qt 6.11.1
`qsgnodeupdater.cpp`, removing only the two injected ancestor-walk counters.
The build script proves that removing those annotations from the instrumented
source reproduces the original source bytes. The capture additionally asserts
all walk counters remain zero. Four embedded import tests and the Windows
system-DLL-only audit pass. No production Qt source or QML layout is changed.

| Capture | Preparation p99 / maximum ms, block 0 | Block 1 | Misses, blocks 0 / 1 |
| --- | --- | --- | --- |
| Detailed RHI + scheduler | 3.3795 / 5.4836 | 4.2394 / 6.6004 | 2 / 11 |
| RHI recorder disabled | 3.0735 / 5.2596 | 4.0170 / 7.1024 | 6 / 4 |
| RHI disabled + sampled CPU | 3.6311 / 7.2867 | 3.9857 / 7.1494 | 8 / 9 |
| RHI disabled + original node updater | 3.0761 / 9.0312 | 3.7827 / 6.1011 | 2 / 1 |
| RHI disabled + original updater + scheduler | 3.6834 / 5.9711 | 3.6474 / 8.1865 | 5 / 3 |

A miss remains a display interval above 9.375 ms. Every capture has zero coverage
holds but fails the existing maximum 2-physical-pixel motion requirement.
These single controls have varying tails and do not establish a repeatable
performance benefit. They do establish that stalls persist without detailed
RHI timestamps and without per-ancestor counter writes. Neither observer can
be treated as the sole explanation for the remaining slowness.

The original-updater capture contains a 9.0312 ms preparation span with 7.8405 ms
in uploads. One text-mask batch accounts for 7.2986 ms, including 7.2977 ms in
unmap, despite only 1,840 vertex bytes and 276 index bytes. No scheduler trace
covers that exact event, so its CPU/wait split is unknown. This is evidence
against explaining every upload tail as a large SVG memory copy.

The lean scheduler repeat does not reproduce that single long upload. Its
largest preparation span is 8.1865 ms, entirely scheduled; the corresponding
13.2412 ms render-total span has 13.2156 ms scheduled and .0256 ms runnable.
That preparation frame uploads 113 batches: 3.5377 ms upload-phase wall time,
including .3677 ms geometry assembly, 2.3202 ms combined unmap and .1676 ms
post-scope diagnostic metadata. There is also 1.7582 ms alpha-material setup.
The largest individual unmap in that block is only .1805 ms. Separately, a
7.3147 ms submission span has 6.8819 ms blocked. Multiple causes remain.

## Independently checked CPU attribution

`real-scroll-vector-admission-cpu-20261029-wheel-masonry` enables sampled
PROFILE stacks alongside scheduling and image-load records, with detailed RHI
timestamps disabled. Native decoding reports zero lost events/buffers or schema
errors. The sampled-profile prefix fallback is independently checked against
xperf: **197 of 197 reference records match**, with no errors. The linked
symbol-map executable's 42,081,546 code bytes and runtime function table are
byte-identical to the captured host. Unwind bounds and identical-code-folding
aliases remain part of resolution; unknown driver addresses are not assigned
invented function names. Two of 7,123 selected render-thread profiles have no
matching stack fragments.

Preparation self-sample counts over both measured blocks:

| Resolved function | Samples |
| --- | ---: |
| QSGNodeUpdater::isNodeBlocked | 386 |
| F4VectorGateFrame constructor | 220 |
| Updater::visitNode | 145 |
| QRhiBufferData::assign | 129 |
| Renderer::updateMaterialDynamicData | 129 |
| Renderer::prepareRenderMergedBatch | 80 |
| Updater::visitTransformNode | 79 |
| Renderer::unmap | 79 |

Recording samples also include shader-resource binding copies and command setup.
Counts overlap with inclusive stacks and are not exact milliseconds. The largest
7.2867 ms preparation frame has eight samples spread across node-block checks,
transform visits, render-list construction, vector-gate construction, memory
copy, draw-set growth, shader-resource update and beginPass. It is not an isolated
single-function spike. The original-node-updater control is not CPU-sampled;
the 386 count must not be claimed for that variant.

## Consequence for the next optimization

The evidence supports reducing repeated scene preparation and update bookkeeping,
while separately investigating occasional resource/submission stalls. It does
not justify another unmeasured buffer-cache enlargement. Prior frame-local
owner-cache and shader-binding-copy experiments already had mixed live results;
their sampled counts alone do not justify promoting them again.

Before changing unmap behavior, add timings within its pool-selection,
create/recreate and data-update branches, and correlate a reproduced tiny-batch
tail with scheduling. Before caching ancestor-block results, establish the
invalidation contract: a custom node's `isSubtreeBlocked()` can change, and a
preprocess callback can mutate the scene. A cache that silently reuses stale
visibility would violate the content/fidelity requirement. Existing strict
display, motion, coverage and warm-route requirements stay unchanged.

## Artifacts and limitations

All scripts have a `vector-admission` and `20261029` stem. Primary compact
outputs are `cold-vector-admission-20261029/{live-scheduler-attribution,
upload-scheduling,scheduler-summary,no-rhi-summary,cpu-summary,lean-summary,
lean-live-scheduler-attribution,lean-upload-scheduling,lean-scheduler-summary,
lean-scheduler-frame-cost}.json`, plus the CPU map/prefix verification and
preparation attribution JSON at the artifacts root. Capture stems distinguish
`scheduler`, `no-rhi`, `cpu`, `lean` and `lean-scheduler`; caches are y, z, za,
zb and zc respectively.

The first attempt to run two offline analyzers concurrently collided on their
shared motion-output file. The lean analysis failed with a missing case key;
it was rerun sequentially and the final per-case output is correct. That failed
analysis contributes no performance conclusion. Future dependent analyzers must
run sequentially even when the underlying captures are independent.

Raw CPU and lean scheduler archives are retained on C: with exact locations and
SHA256 in their scheduler metadata, avoiding D: exhaustion. Five older profiling
JSON files were NTFS-compressed losslessly, with paths unchanged. Snapshot
manifests record local executable/trace hashes; compiled files and raw traces
are not intended for commits. No new static UI layout was introduced.

AI assistance has been used to create this output.
