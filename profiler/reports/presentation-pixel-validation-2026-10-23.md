# Masonry presentation validation — 2026-10-23

## Finding

In four ten-second rapid-wheel blocks, the desktop observer decoded 6,395
consecutive rendered frame IDs. There were no repeated or skipped IDs, no
accumulated desktop frames, and each decoded frame belonged to a distinct
interval between consecutive output-specific vertical-blank wakeups.
This supports steady delivery to the desktop image for this diagnostic run.
It does **not** establish physical scanout timing or complete the constant
160 Hz goal. No scrolling-clock adjustment or acceptance-threshold relaxation
was made.

## Setup and method

- Worktree `D:\Code\f4-zoin`, branch `zoin_branch/cached-pixel-grid`.
- Actual application, Downloads Masonry, 190 entries, density 152, direct D3D11,
  selected MSI MAG322UPF output at 160 Hz and DPR 1.75. Secondary monitor remains
  enabled; no display configuration was changed.
- Prefix `real-scroll-pixel-vblank-rapid-20261023`; same Qt process 97128 for
  cold and warm passes. Each pass contains two ten-second input blocks.
- Diagnostic host uses the minimal production-library baseline and current
  unpromoted native preview staging changes. The merged-span renderer
  experiment is absent. This is not an uninstrumented release measurement.
- A small native scene-graph marker encodes frame ID plus CRC in a 120 by 8
  physical-pixel strip at scene coordinate (16,16). It changes on the render
  thread immediately before rendering. The observer copies only that region
  from Desktop Duplication and validates the CRC and produced IDs.
- A separate observer thread calls `IDXGIOutput::WaitForVBlank` on the same
  selected output and records QPC immediately before and after each call.
  Qt's render and UI threads never wait on it. Records are bounded and written
  after capture. The external process has a finite watchdog.
- No UI Automation polling, builds, tests, or offline analysis ran during the
  input intervals. Foreground PID stayed 97128 throughout all four blocks.

Microsoft documents [WaitForVBlank](https://learn.microsoft.com/en-us/windows/win32/api/dxgi/nf-dxgi-idxgioutput-waitforvblank)
as blocking the calling thread until the next vertical blank. Its return QPC
includes scheduling/wakeup latency and must not be treated as an exact hardware
scanout timestamp.

## Measurements

All times below are milliseconds. The vertical-blank column is the gap between
observer wakeups, not app CPU time. Desktop update timestamps are the raw
Desktop Duplication `LastPresentTime` values.

| Block | Decoded frames | ID/refresh steps | Desktop gap p50 / p99 / max | Vblank wake gap p50 / p99 / max |
| --- | ---: | --- | --- | --- |
| Cold 0 | 1,599 | all 1 / all 1 | 6.2435 / 6.9016 / 7.6219 | 6.2499 / 6.3019 / 6.4731 |
| Cold 1 | 1,599 | all 1 / all 1 | 6.2479 / 7.0525 / 7.8508 | 6.2501 / 6.2873 / 6.3648 |
| Warm 0 | 1,598 | all 1 / all 1 | 6.2456 / 6.9542 / 7.6167 | 6.2501 / 6.2892 / 6.3495 |
| Warm 1 | 1,599 | all 1 / all 1 | 6.2472 / 7.0487 / 7.4262 | 6.2497 / 6.2959 / 6.3668 |

Every desktop row had `AccumulatedFrames == 1`, a valid marker, and the expected
foreground process. All output waits returned S_OK. Desktop-update timestamps
fell about 0.47–2.67 ms after the preceding output-wait return. This variable
offset explains why desktop-update spacing alone cannot identify missed
refreshes in this run. It does not prove that every prior PresentMon anomaly
was observer error.

The marker usually followed the latest native motion sample by about 4 us.
There were 45 marker frames with a sample age above 1 ms. Every one had the
last camera position equal to the destination at an endpoint, with finish and
start events adjacent. Two warm-pass endpoint pauses lasted two frames. No
sample older than 1 ms was found in the interior of a moving segment.
This identifies endpoint pauses; it does not silently exclude them from future
user-experience checks.

The external region copy/map wall time was nontrivial: median 1.37–1.38 ms,
maximum 2.14 ms. It can affect GPU/compositor work despite running externally.
Consequently these measurements are delivery evidence, not a claim of zero
observer overhead or a release performance benchmark.

## Earlier counter and pixel runs

The Oct 21 DXGI frame-statistics capture succeeded with stable swapchain,
S_OK API calls, windowed flip-discard, two buffers and sync interval 1. However,
its refresh/QPC fields were sometimes stale relative to completed IDs. The
mixed-monitor setup makes those fields insufficient as independent scanout
proof. Microsoft explicitly describes multi-monitor limitations for
[GetFrameStatistics](https://learn.microsoft.com/en-us/windows/win32/api/dxgi/nf-dxgi-idxgiswapchain-getframestatistics).

The earlier Oct 23 marker-only capture also decoded consecutive IDs in all
four blocks, with no observer overruns. The new output-specific wait records
add a refresh association rather than changing those old timestamps.

## Verification and limits

- Extended marker tests: 5 passed, no skips, Direct3D11 at actual DPR 1.75.
  This includes CRC corruption rejection, native rendered pixels, leaf origin
  and unit transforms, marker removal, external decode calibration, and
  successful ordered output-wait records.
- Initial harness attempts failed because an extra `QT_SCALE_FACTOR=1.75`
  multiplied actual desktop DPR to 3.0625 and a CSV assertion retained CRLF.
  The override was removed and the parser trims the last field. These were
  harness corrections, not application fixes.
- Existing diagnostic host's four import-isolation cases and system-DLL audit
  passed before these captures. Only the external observer changed for the
  output-wait run; the Qt host was not relinked.
- No source migration, portable payload rebuild, normal-app replacement,
  commit, or push was done in this investigation step.
- Physical display scanning, actual image-content displacement, and the
  broader slow-folder/startup/input matrix remain unproven. Do not mark the
  160 Hz goal complete from this capture.

## Next discriminating work

Associate rendered camera positions with the decoded IDs and output refresh
ordinals, retaining raw timestamps and explicit endpoint pauses. Compare that
sequence with PresentMon's displayed-event assignment before considering any
clock change. If physical presentation remains ambiguous, preserve that limit
instead of fitting or smoothing the observer clock. Native staging and
merged-span retention remain unpromoted until repeatable performance benefit
is demonstrated.

## Reproducible evidence

Snapshot: `profiler/experiments/presentation-pixel-validation-2026-10-23/`.
The manifest records hashes of copied source, scripts, build receipts, tests,
summaries and original capture packets/CSVs. Executables and native object
files remain in artifacts and are identified by the receipts; they are not
included in the snapshot.

> AI assistance has been used to create this output.
