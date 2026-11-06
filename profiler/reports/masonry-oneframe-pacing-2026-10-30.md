# Masonry: one-frame queue limit

## Outcome

`QT_D3D_MAX_FRAME_LATENCY=1` reduces median software-visible present-to-display
latency by roughly one 160-Hz frame, but does not consistently remove scrolling
spikes. Keep the production default unchanged. Constant 160 Hz remains unachieved.
No product implementation, installed Qt package, or embedded payload was changed.
No commit or push was performed.

## Protocol and validity

A/B/B/A compares the default limit 2 against limit 1 using the same pinned
diagnostic host and bounded preprocessing cache plus empty-overlay fast path.
Cases are `real-scroll-oneframe-{a1,b1,b2,a2}-20261030-wheel-masonry`.
Each starts with an isolated empty cache, actual Downloads folder, 192 catalog
entries, Masonry density 152, 3840×2160 window, actual DPR 1.75 and 160 Hz.
Each contains two ten-second wheel windows. The second direction is not a
verified-warm-tour substitute. Verified-warm wheel and quarter-height middle
coverage remain outstanding.

All four original capture-validity checks pass. Target PresentMon rows are
3460, 3291, 3352 and 3386. Delivered wheel events are 1674, 1683, 1734 and 1707,
with zero inactive events. Native foreground ownership stays stable throughout
all eight measurement windows, with no foreground callbacks inside them.
The policy and duration match; delivered event counts and traversal distance
are not identical. No screenshots or accessibility observation run during gestures.
PresentMon uses the same no-GPU-duration, display-enabled QPC protocol in both arms.

The finite owned display/power hold completed normally at its 300-second bound;
its release receipt still reports 3840×2160/160 Hz. Every capture independently
verifies the actual mode. The later explicit release found the process already
gone; no unrelated process or display state was modified.

## Results

| Arm/window | Intervals >9.375 ms | Maximum interval | Maximum motion error | Median present-to-display |
| --- | ---: | ---: | ---: | ---: |
| A1/0 | 33 | 12.8579 ms | 128.21 px | 16.9127 ms |
| A1/1 | 51 | 12.8313 ms | 126.56 px | 17.1974 ms |
| B1/0 | 83 | 18.7601 ms | 176.82 px | 10.8397 ms |
| B1/1 | 59 | 31.2318 ms | 215.56 px | 11.0707 ms |
| B2/0 | 68 | 24.9635 ms | 235.29 px | 10.8951 ms |
| B2/1 | 0 | 6.4535 ms | 2.3973 px | 11.2332 ms |
| A2/0 | 46 | 18.8431 ms | 117.76 px | 16.9175 ms |
| A2/1 | 47 | 12.6433 ms | 129.91 px | 17.2304 ms |

Coverage holds are zero throughout. Every window fails at least one original
gate. Even B2/1 exceeds the maximum two-physical-pixel motion error limit; its
spike-free cadence does not repeat in the other candidate windows.
Present-to-display measures software presentation latency, not click-to-photon.

Median frame-begin-to-sync-begin waits remain roughly 4.73–5.29 ms in both arms;
this limit does not remove the synchronization wait as limit 0 did. Median
submission brackets remain about 0.13–0.20 ms. Scene preparation maxima range
4.86–8.83 ms, exceeding the 6.25-ms frame budget in several windows. Remaining
work includes correlating those outliers with cold delegate/resource preparation
and display misses; these brackets alone do not establish a causal fix.
GUI dispatch duration is unavailable in this minimal recorder.

## Native contracts and evidence

The native fixture passes all nine QtTest results across limits 0, 1 and 2.
For the waitable swapchains it reads back `IDXGISwapChain2::GetMaximumFrameLatency`
on the render thread and confirms actual limits 1 and 2. Vsync interval 1,
two buffers, no tearing, and recording-frame availability during resource
callbacks are retained through resize and hide/show. At actual DPR 1.75, scene
leaf origins, transforms, dimensions and one-physical-pixel stroke assertions pass.
All three corresponding rendered epochs have identical PNG hashes across the
three limits; the captured thin stroke was visually inspected. This isolated
fixture does not replace full application visual regression coverage.

Compact scripts, fixture, logs, results, native observer receipts, and hashes
are preserved in `profiler/experiments/masonry-oneframe-pacing-2026-10-30/`.
Large source packets, presentation CSVs, executables and PNGs stay at their
original paths, pinned by the manifest. The owned diagnostic launcher was stopped;
the ordinary worktree app remains alive at PID 43936.

Next: retain the default queue while isolating preparation outliers, then
repeat the original cold, verified-warm and quarter-height middle gates before
claiming success. Capture readiness questions are no longer needed under the
user's standing authorization.
