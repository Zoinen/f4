# Attribute native scrolling delays after camera commit

The real native controller's remaining producer delays concentrate in the
preparation before Qt's main render pass. This narrows the next profiling
target; it does not identify one internal function or establish constant 160 Hz.
The normal worktree app is unchanged and still uses GUI scrolling by default.

## Probe and verification

The diagnostic host records Qt's frame-begin, synchronization, before-rendering,
render-pass recording, after-rendering, queued-for-presentation and frame-end
signals into a pre-touched fixed buffer. A separate GUI after-animating timestamp
identifies GUI participation. The callbacks record only a clock, phase ID and
thread ID; JSON export occurs after measurement. There is no scene traversal,
per-frame allocation or queued GUI callback from this probe.

The actual signal semantics were checked against the Qt 6.11
[QQuickWindow reference](https://doc.qt.io/qt-6.11/qquickwindow.html).
Before-rendering to before-render-pass-recording includes preparation and any
offscreen layer passes; it is not exclusive time in a named batching function.
After-rendering to frame-swapped brackets submission/queueing, not physical
display. PresentMon remains the source for displayed intervals.

Analysis checks actual GUI/render thread IDs, phase order and complete frame
boundaries. Each matched PresentMon call-start timestamp must fall inside that
frame's render-end to queued-for-presentation interval. Missing initial frames
before PresentMon starts observing are counted separately; no unmatched frame
inside the observed range is silently accepted. Buffers stayed below capacity.

## Workload and controls

Four fresh processes ran phase recording on, off, on, off. All used the same
diagnostic executable and the real native controller. Each supplied two
10-second cold and two 10-second warm blocks, giving 80 measured seconds with
the phase probe and 80 without it. The control has no phase callbacks or records.
The existing controller probe remains present in both.

Conditions were Downloads with 186 entries, Masonry density 152, 160 Hz, actual
DPR 1.75, 2194 x 1186 logical window, 30 ms bouncing wheel input, default 5000 us
GC budget, minimal general recorder and the diagnostic 2 MiB pixmap-cache cap.
No builds, tests, analysis or UI Automation observation ran during input windows.
Build SHA sidecars, process IDs, the single swapchain and active-window input
were checked. The diagnostic probes have nonzero overhead; these measurements
are not exact uninstrumented production latencies.

## Findings

All phase-on and control blocks recorded zero displayed intervals above
9.375 ms. Worst displayed intervals were 8.342 ms with phases and 9.175 ms in
the controls. This does not prove zero instrumentation cost, zero presentation
jitter or a permanent improvement over earlier runs with occasional long
displayed intervals. No production optimization was applied in this step.

Across the phase-on blocks, 137 intervals between Present calls exceeded
9.375 ms. Their median current-frame preparation time was 4.7085 ms. The
largest preparation span was 6.6983 ms. Block p99 preparation spans ranged from
2.261 to 3.097 ms; main-pass recording maxima ranged from 0.899 to 2.048 ms,
and queueing/submission maxima from 0.816 to 1.881 ms. The longest total
before-rendering to frame-end span was 8.4903 ms.

Native commits precede before-rendering closely in these samples. Together
with the prior commit-to-Present correlation, this supports investigating
preparation before the main pass rather than expanding thumbnail admission or
changing scroll retargeting again. Wall spans still include scheduling delays;
these clocks alone cannot distinguish expensive CPU work from preemption.

The next probe should time the actual renderer's render-pass preparation,
opaque/alpha batch preparation and render-list construction. Its input must
match the currently linked Qt library, preserving existing upload/order fixes.
The verified production Quick library SHA is
`6c628e09d98f9e03452d46cd1a31c473049c2ecbc746c47fd263e6227a5b4380`;
its patched batch-renderer source SHA is
`1d82e95a365287c68f9d5e0118d69fbdc7a16f4196aa172c92e31479da24bb9b`.

Evidence: `profiler/experiments/native-scroll-frame-phases-2026-10-20/manifest.json`.
Capture prefixes: `real-scroll-phases`, `real-scroll-phases-control`,
`real-scroll-phases-repeat`, `real-scroll-phases-control-repeat`.
The analyzer is `artifacts/analyze-real-scroll-phases.py`.
No commit or push was made. The 160 Hz goal remains active.

> AI assistance has been used to create this output.
