# Masonry: presentation chain and simultaneous scheduling

## Finding

The constant-160-Hz goal remains unachieved. A new cold/loading capture contains
92 missed application updates across two ten-second wheel windows. Every one
spans two consecutive driver refresh intervals; the output refresh counter itself
never skips. These failures are not explained solely by delayed DPC notification
timestamps.

The corresponding compositor cycles are long: median 5.7811 ms, maximum 8.8724 ms.
They are almost entirely scheduled CPU work, with zero blocked time in the
analyzed cycles. Sampled stacks show compositor expression evaluation, visual
tree/occlusion traversal, rendering and present work, plus NVIDIA user-mode driver
work. This is simultaneous attribution evidence, not proof that all compositor
cost is caused by F4 or that removing one app-side function will fix it.

The previous goal turn made progress by completing a verified warm route and
measuring both wheel and quarter-height middle scrolling. This turn adds the
missing same-run composition and scheduling timeline. No product source, Qt
package, visual quality, animation clock, portable payload, commit or push changed.

## Capture and checks

Case: `real-scroll-displaycpu-b1-20261031-wheel-masonry`, Qt PID 43516.
The pinned diagnostic host is unchanged from the small-dependency experiment,
SHA256 `631b87d7976664b5f55b3400d0ef23cc48576a58b85d7aa788000b8f4c608a11`.
The compact dependency candidate and preceding diagnostic options remain enabled.
This is not a production-equivalent renderer claim.

Actual Downloads, 192 entries including the parent, Masonry density 152,
3840×2160 output, actual DPR 1.75 and 160 Hz. The isolated gallery cache starts
empty; the warm tour is explicitly skipped for cold measurement. Two ten-second
wheel windows deliver 1695 active events, zero inactive events. Native foreground
ownership remains stable throughout both windows. The same native buffer,
resource-error and content checks pass. No screenshot, UIA observation, offline
analysis or build runs during gestures.

Two owned QPC ETW sessions run simultaneously with the existing PresentMon
recorder:

- Kernel: process/thread, loader, context switch, ready thread and CPU profile;
  stacks for Profile, CSwitch and ReadyThread.
- Graphics: DxgKrnl keyword 0x1 level 5, Dwm-Core 0x10008b level 5,
  DXGI 0x2 level 5 and Win32k 0x1000 level 4. ForceVsync is not enabled.

Both sessions stop successfully after ownership/path checks. The graphics trace
contains 1,467,328 events, 1,052,381 selected exported rows and zero lost events,
lost buffers, metadata/property errors or ProcessTrace errors in the final decode.
The scheduler trace contains 9,892,898 events and also has zero lost events,
buffers, schema errors or ProcessTrace errors. These are intrusive attribution
captures: broad graphics providers and CPU stack sampling can add overhead.
They do not replace a clean acceptance run.

## Decoder regressions and retained failures

The new reader separates schema keys by provider, event ID and version; it
retains raw bytes and runtime TDH property descriptions. It handles zero-count
arrays, simple structures and parameter-count arrays of structures through TDH
member descriptors rather than guessed offsets. Provider isolation, malformed
prefix, raw-byte, empty-array and escaping checks pass in the relevant readers.

Against the previously preserved graphics trace, it recovers all 397 target
DXGI starts/stops, all 397 token creations, their 397 InFrame transitions and
397 matching compositor surface updates. These counts match the independent
earlier token-chain reference. Final property errors are zero.

Initial decoding of that old trace exposed 901 unsupported structure fields;
the first new-trace decode exposed 27 structured DirtyRegions arrays. Both initial
outputs are retained. The decoder was corrected and the same raw traces replayed;
there was no replacement capture selected for favorable timing. A shell quoting
error in an offline scheduler-export attempt was corrected in a saved Python
script; it did not rerun input or the trace.

The CPU profile reader uses its documented prefix fallback on this Windows
schema. An independent xperf export matches all 173 selected profile records by
thread, address and timestamp within the export's microsecond quantization.
Profile-based findings are published only after that check passes.

## Presentation identity and deferred submission

The trace contains 3442 application DXGI presents and 3442 composition tokens on
one swapchain and one surface. PresentCount values are consecutive. The first
three tokens are produced on render thread 86172; the remaining 3439 arrive on
D3D driver worker 92452 after the API may have returned. A same-thread,
inside-Present-only token join is therefore invalid here.

The analyzer follows the oldest unmatched process-present ordering used by the
locally pinned PresentMon source, additionally requiring complete equal counts,
one swapchain, one surface and consecutive PresentCount. Token keys link InFrame
and SCHEDULE_SURFACEUPDATE events directly; each update is contained in the
recorded compositor process-frame bracket. The worker's ownership is checked
against kernel thread rundown, then its scheduling is exported from the original
kernel trace, without another capture.

PresentMon contributes 3440 target rows. Each measured row and its raw DXGI API
start/stop must occupy the same unique native Qt submission bracket on the same
render thread. Two raw API presents have no PresentMon row and remain explicit.
Rows outside native recording at startup/after packet publication are outside
measurement. Cross-session start timestamps differ by 0.4–62.4 microseconds;
those offsets are retained and are not fitted away or used to alter motion gates.

Topology before and after agrees: one active MSI MAG322UPF on DISPLAY1,
adapter LUID 87088, source 0, target 200965, 160/1 Hz. VSync records match the
target/source and one unique adapter pointer. The separate adapter-LUID-to-kernel-
pointer mapping event was not captured; this limitation is recorded. All 7618
driver frame counters are consecutive, 148614365 through 148621982. This is
driver/event attribution, not independent pixel identity or physical scanout.

## Results

| Window | Display intervals | Misses >9.375 ms | Display max, ms | Preparation p99 / max, ms | Motion error max, px |
| --- | ---: | ---: | ---: | ---: | ---: |
| 0 | 1556 | 40 | 12.7421 | 3.3788 / 8.7462 | 117.9464 |
| 1 | 1545 | 52 | 12.8613 | 3.6983 / 5.5592 | 127.2012 |

Every missed displayed update advances two driver refresh numbers; every other
qualifying interval advances one. All original performance gates remain intact
and fail. Coverage holds remain zero. Preparation still exceeds the 6.25-ms
budget in the first window, so compositor findings do not eliminate the need
to reduce cold app-side work.

For the 92 compositor cycles directly associated with the missed tokens:

| Quantity | Median, ms | p99, ms | Maximum, ms |
| --- | ---: | ---: | ---: |
| Cycle wall time | 5.7811 | 8.0888 | 8.8724 |
| Scheduled CPU | 5.7788 | 8.0888 | 8.8686 |
| Runnable | 0.00245 | 0.0495 | 0.0841 |
| Blocked | 0 | 0 | 0 |
| Unknown | 0 | 0 | 0 |

The first example's app render-complete timestamp is about 12.16 ms before the
expected next display change. Its composition cycle consumes 5.4580 ms, including
5.4501 ms scheduled CPU. The app render thread's preparation-to-frame-end span
for that submitted image is 0.8912 ms scheduled CPU. Nearby app frames and other
threads are retained in the timeline; the named image's CPU time alone cannot
explain its eventual display interval.

Across the entire capture, API-start-to-driver-token delay has median 0.25485 ms,
p99 1.0899 ms and maximum 4.8776 ms. These are separate submission stages, not
an assertion that every long driver delay produces a miss.

## Sampled work and symbol limits

There are 527 DWM CPU samples in the missed composition cycles: 348 have their
sampled instruction in dwmcore.dll, 58 in nvwgf2umx.dll, and the remainder include
kernel, D3D11, D2D, runtime and five unmapped instructions. Counts are sampled
instructions, not a precise duration decomposition.

Selected full stacks include CExpressionManager::UpdateExpressions,
CVisualTreeIterator traversals, CalcOcclusionAndCollectOverlayCandidates,
CDDisplayRenderTarget::RenderDirtyRegion, CD2DContext::EndDraw, and PresentMPO.
Selection is bounded to the first twelve samples per module, so inclusive
function counts from that subset are not representative percentages.

Public Windows symbols are loaded for checked disk images. Existing-process
module rundown has timestamp zero for these modules; the resolver therefore
requires matching nonzero PE checksum and SizeOfImage, and records that identity
method explicitly. It does not claim an independently captured PDB identity or
a nonzero trace timestamp. NVIDIA private function names are not invented.

The current machine reports an RTX 4090 and a Parsec virtual display adapter;
that inventory alone does not assign this work to the virtual adapter. The
sampled compositor work is not proof that another application, the GPU driver,
or F4 is solely responsible.

## Next discriminating control

The broad Dwm-Core/graphics capture may perturb the compositor it observes.
Before changing app presentation policy, repeat a matched capture with event-ID
filters limited to the required frame/surface/present/token/vsync events. Verify
the emitted IDs and decoder equivalence, then compare compositor scheduled time
and missed delivery. A clean no-extra-trace run remains necessary afterward.
Do not disable composition, force fullscreen, reduce scene quality, change the
animation clock, or replace the existing motion thresholds to obtain a pass.

Also retain the independent cold upload/preparation optimization target.
The prior warmed middle pass and nearly uniform warm wheel cadence are not
substitutes for the complete cold/warm/middle acceptance matrix.

The diagnostic launcher tree is stopped, the display hold is released and
terminal, and both owned ETW sessions are stopped. The normal worktree app is
preserved. Evidence is pinned in
`profiler/experiments/masonry-presentation-scheduling-2026-10-31/`; raw traces
and large exports remain local at externally hashed paths.

> AI assistance has been used to create this output.
