# Masonry middle scrolling: application, interrupts, GPU queues and DWM

Constant 160 Hz remains unachieved. A new full-workload middle-scroll capture
reproduces two genuine missed refreshes. For these two misses, the application
frames were already GPU-ready well before the missed refresh. The associated
DWM composition work runs late. This establishes a more precise delivery
bottleneck than the preceding Qt phase timings alone; it does not prove which
application or desktop state makes DWM expensive, or that all remaining problems
are outside F4.

No production source, installed Qt archive, portable payload, normal application,
Windows setting, commit or push changed. The typed-registry diagnostic remains
unpromoted. Cold-wheel and warm-wheel requirements remain in scope, alongside
middle scrolling, motion precision, coverage, quality and static pixel alignment.

## Capture and validation

The diagnostic uses the existing typed-registry host, actual Downloads with
192 entries including parent, Masonry density 152, 3840×2160 desktop, actual
160 Hz and DPR 1.75. A complete all-entry tour precedes two ten-second
quarter-window-height middle-scroll passes. The tour visits and displays all
192 entries and reaches normal-content quiescence. Native foreground remains
stable throughout both measured windows. No screenshots, UI Automation, builds
or offline analysis run during timed input.

The trace adds kernel scheduling, CPU samples/stacks, DPC/ISR timing, filtered
GPU queue/DMA events and DWM events. PresentMon supplies delivery measurements.
This is **intrusive attribution**, not a clean acceptance run. Preparation
observer maximum is 155.4 microseconds. Trace overhead is not assumed zero.

Kernel and graphics traces report zero lost events/buffers and zero decoder
errors. The kernel trace contains 12,469,531 events; graphics contains 732,895.
All observed DWM thread identities are included, including threads discovered
from rundown rather than just the initial live list. Affected thread identities
are checked against process ownership; no reuse affects the analyzed spans.

The new interrupt reader initially omitted PerfInfo opcode 50. It now handles
the observed 24-byte version-2 variant as well as 20-byte ISR opcode 67 and
16-byte DPC variants. Unknown versions/widths and reversed time ranges fail.
Both ISR registrations and the shared QPC/routine prefix are independently
documented in [Microsoft's TraceEvent parser](https://github.com/microsoft/perfview/blob/main/src/TraceEvent/Parsers/KernelTraceEventParser.cs).
The extra opcode-50 bytes are not interpreted. The measured timestamp, CPU,
routine and duration fields are checked against xperf rather than accepted from
schema assumptions: all 39,482 first-preflight and 37,359 rebuilt-preflight
records match one-to-one within 1.1 microseconds. The controller rebuild changed
its binary hash; the stale receipt was rejected before a logger or application
started, then a fresh preflight validated the exact rebuilt controller.

CPU sample prefixes are independently compared with xperf for 267 selected
application/DWM samples. Windows symbols require matching nonzero timestamp and
image size, or nonzero PE checksum and size when the trace lacks a timestamp.
NVIDIA private names remain unresolved; module offsets are retained.

## Delivery result

| Measured window | Display intervals | Maximum interval (ms) | Intervals >9.375 ms |
| --- | ---: | ---: | ---: |
| First middle pass | 1594 | 12.4902 | 2 |
| Second middle pass | 1596 | 6.6047 | 0 |

Original strict motion analysis stops at `Missing presented motion frame,
929, 931`. A second missing pair is 1084–1086. Presents 930 and 1085 are
actually dropped. Motion acceptance is **unavailable/failed**, not zero and not
silently recalculated after excluding the discontinuities. The analyzer's
initial wrapper incorrectly expected a populated motion-result file after this
intentional early assertion; the wrapper failure is retained and a separate
failed verification receipt records the original continuity failure.

PostSync medians are 0.1971/0.1949 ms; p99 values are 0.3608/0.3295 ms.
These local CPU statistics do not establish smooth display delivery.

## Surface-token and refresh chain

The entire trace contains 3719 application API presents and 3719 consecutive
surface tokens on one swapchain/surface; PresentMon records 3717 rows. Startup
rows outside native phase recording are handled separately. Every measured
PresentMon row joins a unique API start inside the same native submission
bracket. Deferred driver-thread tokens use checked oldest-unmatched ordering,
not an arbitrary enclosing API bracket.

Each affected token joins its DWM surface update and composition cycle. Matching
target/source IDs select the actual monitor. Consecutive driver frame numbers
span **two refreshes** at both misses: driver periods 12.5088 and 12.4832 ms.
This rules out notification-time jitter as the explanation for these particular
double intervals. It does not redefine the existing motion metric.

| Application present | DWM cycle | App GPU-ready before missed refresh (ms) | DWM present-scope scheduled CPU (ms) | DWM DMA completion after missed refresh (ms) |
| --- | ---: | ---: | ---: | ---: |
| 929 | 3904 | 12.0889 | 4.1281 | 0.8394 |
| 1084 | 4059 | 11.6199 | 3.6683 | 1.0172 |

The last column follows DWM queue submissions inside the associated present
scope, their matching DMA starts and completion notifications. Queue/DMA joins
use context, queue sequence and submission/completion identities; 206 DMA
segments are checked. Missing/non-DMA submissions remain explicit. No adapter
LUID-to-engine mapping or optical scanout measurement is invented.

The application has only 1.0201 and 2.2989 ms of scheduled render-thread time
in the whole frames for presents 929 and 1084. Their preparation/recording/
submission wall times are respectively 0.3611/0.2128/0.1488 ms and
0.6817/0.7094/0.3106 ms. The larger Qt frames occur later: present 931 takes
10.1226 ms wall time and present 1086 takes 9.6849 ms. These later costs remain
real problems, but are not labeled the cause of the earlier missed refresh.

## CPU scheduling and interrupts

All 190 analyzed application/DWM costs conserve scheduled + runnable + blocked
time, with zero unknown scheduling time. Interrupt cost is the union of DPC/ISR
intervals on the CPU while the selected thread is scheduled; nested interrupts
are not double-counted. Three synthetic union cases cover nesting, overlap and
empty/zero-width spans.

The two long DWM present scopes take 4.1307 and 3.6683 ms wall time. Scheduled
time is 4.1281 and 3.6683 ms, runnable delay 0.0026 and 0 ms, and interrupt union
0 and 0 ms. Their delay is not explained by preemption or interrupt occupation
in these spans. Scheduled time is not a measurement of CPU clock frequency or
retired instructions, so this does not rule out hardware/power effects.

A later Qt frame (931) includes 0.4990 ms of interrupt occupation inside its
4.6137 ms scheduled interval. The subtraction is retained separately rather
than charging the entire scheduled interval to application instructions.

Resolved DWM stacks in the long scopes include:

- `COverlayContext::ComputeOverlayConfiguration` and overlay-candidate/occlusion
  traversal.
- `CBatchCommand::InitializeForRendering`, `CDrawListEntry::Render`,
  `CDrawListCache::Render` and `CDrawingContext::DrawVisualTree`.
- `CCommonRenderingEffect::EmitGeometry`, Direct2D deferred operations and
  layer flushing.
- The DDisplay/DXG present path.

These are statistical samples, not exact function durations. They do not name
the originating desktop visual or prove that F4's SVG geometry is being drawn
inside DWM; F4's QML scene is rendered in its own process.

## Next work

Investigate the source of the extra desktop composition work and whether the
application's presentation/surface behavior can reduce it without changing the
window UX or rendering quality. The existing opacity/fullscreen controls already
exclude a simple alpha-buffer/fullscreen switch; do not repeat those unchanged
or alter Windows settings as an application fix. CPU-frequency/retired-work
evidence would distinguish executing more compositor work from executing the
same work more slowly; this trace does not contain that evidence.

Keep investigating the independent later application CPU spikes, including
constructor/destructor bookkeeping from the experimental registry. A lower
postSync median does not exonerate those paths. No renderer change should be
promoted based solely on this attribution capture. Final verification still
requires the complete normal-window cold wheel, all-entry warm wheel and
quarter-height middle tests with the original delivery, motion, coverage and
quality gates.

Sources, checks and compact results are preserved in
[the experiment manifest](../experiments/masonry-middle-critical-path-2026-10-31/manifest.json).
Raw ETLs, decoded streams and packets remain on C: with hashes. Only the exact
owned diagnostic process tree was stopped. Normal Go PID 23488 and Qt PID
84780 remain alive. All owned trace loggers are stopped.

> AI assistance has been used to create this output.
