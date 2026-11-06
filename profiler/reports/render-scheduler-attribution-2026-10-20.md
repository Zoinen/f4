# Render scheduler attribution, 2026-10-20

The remaining scrolling spikes are not explained solely by render-thread
preemption. A quiet capture identifies an ordinary UI geometry-buffer upload
as one concrete remaining source of a frame-budget overrun. No production
code changed during this investigation. Constant 160 Hz remains unverified.

## Workload and identity

Downloads, 186 entries, Masonry density 152, DPR 1.75, 160 Hz display,
wheel input every 30 ms, two 10-second input windows per cold/warm stage.
Native scroll controller enabled for these diagnostic runs; production default
remains disabled. QV4 GC budget 5000 us; diagnostic pixmap cache 2 MiB.
General frame recorder disabled. This is a bounded diagnostic workload, not
proof for all folders, modes, or production defaults.

Diagnostic host SHA256:
`b3263bd66a15785169ddbe08749f10637f34a77454cd24afa9ddb87b11dd5565`.
The build-time actual-link receipt validates the linked response file, Quick
archive, explicit renderer object, controller, and source inputs before launch.
The Quick archive is the prepared-vector v2 archive (SHA256
`bf3d4d369a641d5422111d65bea858962d27be0902ecb14d427aaf24735382c7`).

## Method and checks

Kernel tracing captures context switches, ready-thread events, and process/thread
rundown using raw QPC timestamps. Offline intervals divide render-thread time
into scheduled, blocked, runnable, and unknown. Synthetic accounting checks
cover blocked-to-ready transitions and clipped interval boundaries.

The native reader matched 1950 of 1950 events independently compared against
xperf output. CSwitch v5 uses the checked Microsoft TraceEvent 24-byte prefix;
the fallback is explicit in metadata. Both accepted captures have zero lost
events, lost buffers, schema errors, and ProcessTrace errors. Trace coverage
contains every measurement window. Rundown confirms the render thread belongs
to the measured host. ETL archives were decompressed and hash-verified before
removing their uncompressed copies.

Scheduled time is not pure application CPU time: interrupts and DPC execution
are not subtracted. GetThreadTimes was rejected for per-phase attribution after
a calibration showed 15.625 ms accounting steps, too coarse for a 6.25 ms budget.
QueryThreadCycleTime is not converted into elapsed time.

## Results and observer overhead

Counts below use producer intervals whose endpoints are both inside input
windows. A long interval is greater than 9.375 ms, not the stricter 6.25 ms
target. Display intervals are reported separately from producer intervals.

| Run | Input seconds | Producer long intervals | Display long intervals | Maximum display gap |
| --- | ---: | ---: | ---: | ---: |
| Scheduler trace, cold | 20 | 111 | 2 | 12.4901 ms |
| Scheduler trace, warm | 20 | 84 | 0 | 6.5685 ms |
| Quiet control, cold | 20 | 23 | 0 | 6.5264 ms |
| Quiet control, warm | 20 | 10 | 0 | 7.7652 ms |

Both runs use the same diagnostic binary and bounded input pattern. The large
difference is evidence of scheduler-tracing overhead; a single paired run does
not quantify its invariant cost or exclude other system variation. Traced
phase durations cannot be presented as normal-production timings.

Examples in the traced run:

- Cold preparation: 10.8963 ms wall, 10.7338 ms scheduled, 0.1500 ms blocked,
  0.0125 ms runnable. Cold recording: 4.5045 ms wall, all scheduled.
- Warm recording: 2.6923 ms wall includes 1.6050 ms blocked. Warm submission:
  1.8287 ms wall includes 1.6041 ms blocked. Some stalls therefore do involve
  waits; blanket claims that all remaining cost is CPU work are unsupported.

The quiet cold run has an 8.9750 ms preparation phase. Renderer subscopes
attribute 7.7496 ms to ordinary alpha uploads. One QSGSmoothColorMaterial
batch, outside any vector gate, takes 6.6822 ms: 960 vertex bytes and 128 index
bytes. Its probes report 3.1919 ms in dynamic-update operations and 3.4882 ms
in buffer creation. This points to buffer lifecycle/driver operations rather
than a large SVG geometry payload for that frame. These are nested scopes and
must not be added to their parent upload duration.

Next investigation: count buffer creation, growth, destruction and reuse for
ordinary batches; inspect QRhi D3D11 allocation/update behavior. Establish
whether a small, reusable buffer pool or avoiding unnecessary recreation can
remove this spike before changing production code. Preserve draw order,
resource lifetimes, other backends, and vector geometry fidelity.

## Rejected captures and operational corrections

- `real-scroll-scheduler-v2` hit its 256 MiB ETL cap and stopped before full
  coverage. It is retained as rejected evidence, not used in results.
- `real-scroll-scheduler-v2-control` overlapped offline analysis. It is invalid
  for performance comparison and is excluded above.
- Accepted scheduler traces use a bounded 1 GiB temporary trace on C:, export
  only the two relevant threads, then verify/compress the evidence. No unrelated
  tracing session is stopped. All measurement input windows are free of builds,
  tests, offline analysis, and UI Automation observers.
- D: filled during later offline analysis. An older 994 MiB CSV was losslessly
  compressed, verified by SHA256, and retained; analysis was rerun after space
  was available. This did not occur inside the accepted capture windows.

## Evidence

The accompanying manifest records source, reader, actual-link receipt, full
and quiet capture JSON, PresentMon CSV, accepted compressed ETL, exported
scheduler CSV, statistics, metadata, and analysis hashes. Mutable working copies
are under `artifacts/modes-profile`; durable evidence is archived separately.
The normal worktree Qt application remains the previously verified embedded
build. No canonical checkout executable was replaced and no commit or push
was made.

AI assistance has been used to create this output.
