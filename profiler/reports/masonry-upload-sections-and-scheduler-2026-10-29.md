# Masonry upload sections and scheduler attribution — 2026-10-29

## Scope and result

Two accepted diagnostic captures exercised Downloads (191 entries), Masonry
density 152, at actual 3840×2160 / 160 Hz / DPR 1.75. Each used a fresh application
gallery cache and two ten-second, 100-Hz wheel gestures with reversal. These are
cold/loading observations, not verified warm-route comparisons. No optimization
is promoted and constant 160 Hz remains unachieved.

Cases:

- `real-scroll-upload-sections-scheduler-20261029-wheel-masonry`
- `real-scroll-upload-sections-scheduler-r2-20261029-wheel-masonry`

Both pass capture guards. Kernel scheduling traces have zero event/buffer loss,
schema errors, and ProcessTrace failures. Analysis runs after gestures. No UI
Automation observer, build or offline analysis runs during gestures.

## Instrumentation and verification

The existing thirteen preparation marks remain. A reversible renderer probe
adds main-thread upload scopes separating CPU-storage allocation, merged geometry
assembly and the QRhi update. Routes distinguish ordinary opaque, ordinary alpha
and pending vector batches. Metadata time is recorded separately where possible;
this is only partial observer-cost accounting, not a claim of zero overhead.
The 524,288-entry buffer does not overflow. Source reversal checks recover the
input renderer exactly. Existing first-use geometry mapping is enabled in both
captures; there is no new upload or readiness policy in this probe.

Seven probe tests, 48 native regressions, 27 shader regressions at DPR 1.75,
four embedded import checks and the Windows DLL dependency audit pass. Sources,
build receipts, command lines and test logs are retained in the experiment
snapshot. This is a development host override, not a portable production rollout.

## Same-frame findings

The preceding experiment's 14.4369-ms upload/admission span was **not reproduced**.
Neither current capture establishes its cause. In particular, absence of a long
RHI call in that older frame does not establish CPU copying as its cause.

In the first capture:

- The largest preparation span is 7.5233 ms: 2.3745 ms scheduled, 0.0085 ms
  runnable and 5.1403 ms blocked. Alpha-material preparation accounts for
  6.2452 ms. A same-frame native `CreateBuffer` takes 5.1945 ms for a
  **1,024-byte dynamic constant buffer** (bind flags 4). It is not a texture
  creation and not a multi-megabyte geometry allocation.
- The largest individual upload is 3.9436 ms for curve-fill geometry
  (599,412 vertex bytes, 54,492 index bytes). Its QRhi update takes 3.8689 ms,
  including 3.6558 ms blocked. The same-resource used-buffer DISCARD map takes
  3.7880 ms. First-use mapping deliberately does not bypass this used-buffer guard.
- A curve-stroke transfer with 2,054,448 vertex bytes and 186,768 index bytes
  takes 1.2340 ms, entirely scheduled: 0.2739 ms CPU-storage allocation,
  0.3166 ms geometry assembly and 0.6431 ms QRhi update.
- Other preparation spans of 6.6834 and 5.4685 ms are entirely scheduled.

In the repeat:

- The largest preparation span is 8.8395 ms: 8.8256 ms scheduled, 0.0139 ms
  runnable and no blocked time. Costs are spread over before-body work
  (0.9421), render lists (1.4665), batching (0.6469), uploads (2.2435), and
  alpha materials (2.6336 ms). It is not one long blocked driver call.
- The other block peaks at 4.9226 ms, with 4.9196 ms scheduled.
- The largest individual transfer is a curve-stroke upload of 1,136,916 vertex
  bytes and 103,356 index bytes: 1.2245 ms entirely scheduled, including
  0.0941 ms CPU-storage allocation, 0.3941 ms assembly and 0.7361 ms QRhi update.

Scheduled time includes execution inside driver/kernel code. These traces do
not measure CPU frequency or explain all variability. The machine reports an
AMD Ryzen 9 5950X (16 cores / 32 logical processors); an Intel hybrid-core
explanation does not apply. No affinity, QoS, priority, power-plan or unrelated
application change was made.

## Presentation remains below acceptance

| Case/block | Display misses above 9.375 ms | Maximum display interval | Maximum motion error |
| --- | ---: | ---: | ---: |
| First / 0 | 9 | 18.8125 ms | 181.4636 physical px |
| First / 1 | 1 | 12.4891 ms | 4.0431 physical px |
| Repeat / 0 | 3 | 12.5020 ms | 76.8476 physical px |
| Repeat / 1 | 6 | 12.6574 ms | 113.9556 physical px |

The first block has three coverage holds; the other three have none. All four
blocks fail the existing two-physical-pixel maximum motion gate. Small median
frame times do not change this result. Loading and source-residency differences
prevent attributing between-capture differences to a policy change.

## Next bounded experiment, not implemented by this report

The fresh-buffer pool retains two buffers per constant-buffer size class. In the
first trace the render thread requests up to thirteen 256-byte constant buffers
and three 1,024-byte buffers in one preparation frame. The actual pool reports
286 uniform hits and 67 misses. This is evidence of burst-capacity mismatch;
it is not evidence of worker starvation (the worker already uses round-robin).

Test increasing only the 256-byte reserve from two to sixteen and the 1,024-byte
reserve from two to four. This adds sixteen entries and 5,632 reserved bytes,
staying below the existing 32-MiB limit. Preserve control behavior, fresh-buffer
ownership, descriptor validation, worker lifetime and driver-concurrency guards.
Add a regression for the measured bursts and retain GPU readback tests. Run
interleaved captures with matched loading conditions before any promotion.

This targets the confirmed synchronous constant-buffer allocation tail. It
does not claim to fix the used-buffer DISCARD wait, distributed CPU preparation,
the unreproduced 14-ms span, or the remaining motion/coverage failures. Verified
warm wheel, quarter-height middle scrolling and final fidelity checks remain.
