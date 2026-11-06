# Masonry pacing: accepted live comparison and latency tradeoff

## Outcome

Foreground capture is working again. An accepted A/B/B/A comparison at actual
160 Hz and DPR 1.75 confirms that disabling the D3D11 frame-latency waitable
object moves several milliseconds of waiting away from synchronization. It also
adds roughly two frames of software-visible presentation latency and does not
eliminate display spikes. Keep the production default unchanged. The original
constant-160-Hz goal remains active and unachieved.

Parent HEAD: `b2318c4b5c10a526c0bb7271626609ee54ea5b15`.
Branch: `zoin_branch/cached-pixel-grid`.
ZoinGallery HEAD: `23d0a5f35a7ea793f944d570c828633b9f2c90dd`.
Existing dirty implementation changes were preserved. No commit or push.

## Matched protocol

Cases: `real-scroll-frame-pacing-stable-{a1,b1,b2,a2}-20261030-wheel-masonry`.
Each case starts a fresh process and empty isolated cache, uses the same pinned
diagnostic executable, real Downloads folder, Masonry density 152, 4K window,
and two ten-second wheel windows. The catalog reports 192 entries in all cases;
do not silently substitute an older report's entry count. Both arms enable the
bounded preprocessing cache and overlay fast path. A uses
`QT_D3D_MAX_FRAME_LATENCY=2`; B uses 0. Zero disables the waitable object; it does
not promise a zero-length frame queue.

The display is held at 3840×2160/160 Hz throughout the four cases. Inner wrappers
verify the mode and skip redundant display-mode application. A finite owned
power/display hold is released afterward, restoring its prior mode when needed.
Every measurement window has stable native foreground ownership and no
foreground callbacks. Wheel events: A1 1668, B1 1907, B2 1895, A2 1683; inactive
events zero. This is matched input-generation policy and duration, not identical
delivered event counts or identical traversal distance. No screenshot or
accessibility observation runs during measured gestures. All four original
capture-validity checks pass, with 3406–3433 target PresentMon rows per case.

The original gates remain: zero displayed intervals over 9.375 ms, maximum
motion error at most two physical pixels, and zero coverage holds. These cases
are cold-cache launches; their second direction is not a verified warm-tour
replacement. Verified-warm wheel and quarter-height middle coverage remain due.

## Results

| Arm/window | Intervals >9.375 ms | Maximum display interval | Maximum motion error |
| --- | ---: | ---: | ---: |
| A1/0 | 48 | 18.7425 ms | 117.39 px |
| A1/1 | 39 | 24.8603 ms | 128.98 px |
| B1/0 | 38 | 18.7793 ms | 130.47 px |
| B1/1 | 1 | 12.4654 ms | 25.16 px |
| B2/0 | 44 | 12.6978 ms | 130.82 px |
| B2/1 | 43 | 12.8038 ms | 128.49 px |
| A2/0 | 37 | 18.5853 ms | 116.27 px |
| A2/1 | 47 | 18.6866 ms | 128.28 px |

Coverage holds are zero throughout. Every window fails both the display-spike
and maximum-motion gates. B1's favorable second window does not repeat in B2;
do not promote that subset into a success claim or infer a robust improvement
from pooled miss totals.

For the accepted runs, median frame-begin-to-sync-begin time is 4.61–4.99 ms in
A and 0.045–0.093 ms in B. Median render-end-to-frame-swapped time moves the
opposite way: about 0.16–0.22 ms in A versus 4.36–5.01 ms in B. This confirms a
relocation of backpressure, not its removal. GUI dispatch timings are unavailable
in this minimal recorder; the render brackets are not mislabeled as direct GUI
CPU measurements.

Median PresentMon present-to-display latency is 16.78–17.20 ms in A and
29.12–29.80 ms in B. This is software-visible presentation latency, not measured
click-to-photon latency. The roughly 12 ms increase is material despite the
larger delivered input count. Do not adopt B as a scrolling fix on these results.

## Recorder investigation and corrected protocol

The first four foreground captures delivered sufficient input and recorded
thousands of Qt frames, but PresentMon display tracking returned only 13–157
target rows. Those runs remain explicitly rejected for full performance claims.
Their native phase records support component attribution only.

An API-only/display calibration, a no-input variant, and raw ETW recording
isolated the issue from missing application presents. The raw trace has zero
lost events/buffers and contains 397 target DXGI present starts/stops, 397 matching
Win32k tokens, 397 matching InFrame transitions, and 397 DWM surface updates.
Offline API decoding recovers 395 rows; offline display decoding recovers none.
Both normal and frame-type display variants fail on that trace. This is a
reproducible display-decoding/finalization failure, not proof of missing raster
frames. Raw token keys and DWM thread identity were checked, including the
structured LUID representation.

The early calibration packets report about 60 Hz. Later full-GPU and no-GPU
calibrations at 160 Hz both produce usable display records. They also use
different process/pacing states, so they do not prove GPU-event tracking fixes
the recorder. The reliable comparison above keeps 160 Hz stable, skips repeated
mode application, and retains the existing no-GPU-duration recorder in both arms.
The exact root cause of the earlier finalization failure remains unresolved.

Initial raw xperf commands rejected brace-wrapped GUID syntax with error 0x3ec
(invalid flags). Removing the braces allowed the owned trace to run. This was
not an approval rejection or a Windows security-policy failure. A path-literal
escaping error in the new foreground-release helper also initially rejected a
valid window; it was corrected to use resolved Paths before release. Failed
attempts and successful native checks remain distinguishable.

## Preservation and next work

Large packets, CSVs, captures, executables and the raw ETL remain at their
original paths and are hash-pinned. Compact source, scripts, validity checks,
results, source references and observer diagnostics are frozen under
`profiler/experiments/masonry-frame-pacing-live-2026-10-30/`.

The owned diagnostic launcher was stopped; the ordinary worktree application
remains alive at PID 43936. The display hold was released. No production source,
Qt package or embedded payload was replaced.

Next: test a one-frame limit with the same stable-display protocol and examine
remaining cold preparation spikes. Preserve display-latency measurement along
with frame cadence and motion, then cover verified-warm wheel and quarter-height
middle scrolling before any completion claim.
