# Desktop capture and presentation telemetry coexistence, 2026-10-31

## Result

The current host reproduces a measurement problem: while Desktop Duplication
is active, the standard display-tracking PresentMon session emits no F4 rows
in either measured window. A simultaneous submission-only session retains
all corresponding calls, and the desktop observer decodes F4's frame markers.
This narrows the loss to the display-tracking/reporting path; it does not
identify the exact missing event or prove a specific consumer-code defect.

Constant 160 Hz remains unachieved and unproven. Both duplication runs fail
the unchanged >3,000 target-record guard. The submission-only data is not a
replacement performance pass. The baseline without duplication also exceeds
the existing maximum-motion limit. Earlier valid no-duplication failures,
including driver-confirmed compositor misses, are not retrospectively dismissed.

The preceding goal turn is progress: it repeated the parent-mapping benefit,
completed cold/middle checks and joined 18 remaining intervals to early F4
readiness. This turn changes the next action by establishing a usable submission
and marker association despite missing display telemetry. No animation-clock,
pacing, product-source, default-backend or portable-payload change is made.

## Current host and verification

The separately named diagnostic host reuses the exact parent-mapping renderer
graph. Its only main/profile addition is a native 120x8 physical-pixel frame
marker at scene position (16,16), encoding a frame ID and CRC. It records the
latest native camera-sample index before/after rendering. The same marked host
is used throughout; the marker is present even when external capture is off.

Five marker tests pass: CRC/corruption, external desktop/red-raster calibration,
actual-DPR-1.75 scene origin and unit transforms, native pixel capture and marker
removal. Four rendered marker epochs are saved; the final barcode image was
inspected. Three installed-tree-independent import cases and the Windows
system-DLL audit also pass. The older full renderer/motion image tests remain
inherited graph verification, not newly executed tests in this step.

The external observer is the existing hash-verified executable/source. It copies
only the marker region in the live runs, keeps bounded records in memory, and
writes after capture. Its finite watchdog and exact stop file remain in force.
There is no gallery-content raster capture in these runs.

## Workload and controls

Real Downloads, 192 entries, Masonry density 152, 3840x2160, 160 Hz and actual
DPR 1.75. Ordinary window, four workers, parent mapping enabled, original queue
depth and renderer settings. Each fresh process verifies all indices 0..191
displayed with normal visibility preserved, then two 30-second wheel windows.
Native foreground stays stable and observer overflows are zero. No screenshots,
accessibility inspection, builds or offline analysis run during timed input.

PresentMon 2.6.0 uses the current display-enabled no-GPU-duration protocol in
the original session in all arms. B2 adds a separate uniquely named session
using `--no_track_gpu --no_track_display --no_track_input`; all other application
behavior is unchanged. These flags deliberately remove display/input tracking
from that second diagnostic stream, not from the original acceptance stream.
See the [version-pinned PresentMon documentation](https://github.com/GameTechDev/PresentMon/blob/v2.6.0/README-ConsoleApplication.md).

| Case | Desktop Duplication | Original display-tracked F4 rows | Second submission-only F4 rows | Produced markers | Original capture code |
|---|---|---:|---:|---:|---:|
| A1 | Off | 10,035 | Not running | 10,037 | 0 |
| B1 | On | 178 | Not running | 10,029 | 1 |
| B2 | On | 220 | 10,058 | 10,060 | 1 |

Every B1/B2 standard F4 row occurs before the desktop observer begins. There
are zero during its lifetime and zero inside either measurement window. This
is not a >3,000-row count barely missed or a wrong target PID. The observer's
window/PID matches the native activation handshake and decoded marker source.
The DWM process continues to have reported rows. No extra file-based DxgKrnl
trace runs here; missing rows reproduce with GPU-duration tracking already off.
That rules out those older capture additions as necessary for this symptom,
without proving which lower-level event changes under duplication.

A1's display maxima are 6.5074 / 6.4380 ms with zero intervals above 9.375 ms,
but maximum motion errors are 4.46877 / 2.82922 physical pixels. Its full
smoothness result therefore remains failed. B1/B2 have insufficient original
display data and are not assigned favorable display/motion results.

## Frame identity and submission association

B2's submission-only stream joins **9,602 measured marker frames** to unique
native Qt frames and exact submission brackets: 4,801 per window. Before/after
camera indices agree. There are 4,772 / 4,773 fresh camera associations and
29 / 28 carried latest samples, all at declared endpoints. Carried poses remain
explicit; they are not called fresh samples or silently deleted.

The desktop observer decodes 4,800 / 4,800 measured rows in B1 and 4,800 / 4,801
in B2. Every selected marker has a valid CRC, known produced ID, successful
readback and the expected foreground owner. All `AccumulatedFrames` values
are one. Non-unit marker steps are retained:

- B1/0: one repeat and one ID step of two, at 7.7506 and 14.2116 ms.
- B1/1: one repeat at 7.3885 ms.
- B2/0: one repeat and one ID step of two, at 7.3063 and 13.7304 ms.
- B2/1: one repeat at 7.0327 ms.

All occur within the first 15 ms. First-window repeated codes precede the first
native motion sample; second-window repeated codes retain an endpoint pose.
Every later observed adjacent ID step is one. These boundaries remain in the
data and do not become exceptions to the original performance gates.

The marker proves that its code pixels reached the duplicated desktop surface.
It does not independently prove gallery-image displacement, refresh/image
identity, physical scanout or click-to-photon latency. The observer's output
vblank-wakeup records are retained, but wakeups are not substituted for driver
refresh counters or exact hardware timing.

## Observer costs and limitations

Copy/map wall-time medians are 1.4759 / 1.4779 ms in B1 and 1.49695 / 1.4935 ms
in B2; maxima range 2.4024-2.6425 ms. This GPU/CPU observer is intrusive. B2 also
adds a second ETW consumer. Neither run is an uninstrumented release benchmark
or an isolated measurement of one PresentMon option's overhead.

The desktop-global masked-content flag is one in all measured capture rows.
Microsoft defines it for possible already-blacked-out protected content in the
desktop image, not as a per-marker visibility verdict. The marker's independently
validated CRC is visible, but visibility of the whole gallery image remains
unverified. Capture protections remain unchanged; only the deliberately
rendered marker strip is copied in the live runs.
[DXGI frame-info documentation](https://learn.microsoft.com/en-us/windows/win32/api/dxgi1_2/ns-dxgi1_2-dxgi_outdupl_frame_info).

The analysis initially assumed every marker had a fresh same-frame camera
sample and that the desktop-global masked flag had to be zero. Those additional
diagnostic assumptions are corrected by recording endpoint carries and the raw
global flag. No original >3,000-row, 9.375-ms, 2-pixel, native-velocity or
coverage requirement is relaxed. Full-image and physical-timing fields remain
explicitly unproven.

The first submission-control receipt mistakenly labels the foreground wrapper's
zero exit code as the underlying capture's code. Both the original receipt and
correction are preserved: wrapper completion is 0, original display capture is
1. The source now records them separately. No failed capture is promoted.

## Final state and next discriminating work

All owned diagnostic Qt/Go, desktop observer, foreground observer and named
PresentMon processes are terminal. The normal worktree app remains alive at
Go PID 23488 and Qt PID 84780. No installed Qt SDK, normal binary, OS policy,
external workload/affinity, commit or push changes. Raw packets and failed
original captures remain hash-preserved; only verified duplicates are retired.

Next: combine the retained submission/marker association with independent
driver refresh ordinals and calibrated actual-image displacement. Retain
endpoint records, observer cost and uncertainty, and keep ordinary no-duplication
captures for the original display checks. Do not fit the animation clock to
desktop/DPC notification jitter or claim success from consecutive code IDs alone.
The full cold/warm/middle and portable-rollout requirements remain outstanding.

Compact sources, tests, corrected/original receipts, failed guards, all marker
anomalies and raw-location hashes are indexed beside this report. Large raw
packets, CSVs, marker images and binaries remain at their hashed external paths.

AI assistance has been used to create this output.
