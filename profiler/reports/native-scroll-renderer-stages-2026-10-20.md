# Narrow native-scroll preparation delays inside the Qt renderer

Function timers on the real native-scroll workload place most of the slow
preparation interval inside `QSGBatchRenderer::Renderer::prepareRenderPass()`.
Opaque/alpha batch preparation and render-list construction account for only a
small part of its longest wall spans. Further attribution is required before
choosing a production change. Constant 160 Hz remains unproven.

## Provenance and workload

The function-instrumented Quick library was built from the same patched source
and base library currently linked by the worktree: base SHA-256
`6c628e09d98f9e03452d46cd1a31c473049c2ecbc746c47fd263e6227a5b4380`,
source SHA-256
`1d82e95a365287c68f9d5e0118d69fbdc7a16f4196aa172c92e31479da24bb9b`.
Existing upload, ordering and retention behavior is preserved. The probe header
matches the header used to build the instrumented library. Timing storage is
pre-touched before QGuiApplication construction; no first-touch work is
deliberately deferred into measured scrolling.

This is the existing real GalleryScrollAnimation path, with fixed-buffer
controller and public-window-phase probes plus function scopes. Inclusive
function spans are combined by interval union, not summed across nested calls.
All collected renderer events came from the expected render thread; buffer
reservations were 25,786 after cold capture and 51,067 after warm, below 131,072.
The public phase order and PresentMon correspondence checks also passed.

One fresh process supplied two 10-second cold and two 10-second warm blocks.
Conditions were Downloads/186 entries, Masonry density 152, 160 Hz, actual
DPR 1.75, 2194 x 1186 logical window, 30 ms bouncing wheel input, default
5000 us GC budget and the diagnostic 2 MiB pixmap-cache limit. General frame
recording and UI Automation observation were disabled. No builds, tests or
analysis ran during input windows. Instrumentation has nonzero overhead;
these are wall spans, not exclusive CPU measurements.

## Longest preparation spans

| Block | Public preparation | Renderer prepareRenderPass | Alpha preparation | Tagged render lists |
| --- | ---: | ---: | ---: | ---: |
| Cold 1 | 5.3335 ms | 4.9179 ms | 0.4950 ms | 0.3086 ms |
| Cold 2 | 5.6095 ms | 5.1591 ms | 0.5791 ms | 0.3493 ms |
| Warm 1 | 5.9308 ms | 5.5078 ms | 0.6072 ms | 0.2663 ms |
| Warm 2 | 5.6395 ms | 5.3717 ms | 0.5146 ms | 0.2380 ms |

Opaque preparation was approximately 0.0001-0.0002 ms in these examples. Full
render-list reconstruction did not occur in these selected intervals. This
does not say every batch/list operation is free; it shows these particular
long spans are not explained by the measured batch/list scopes.

The remaining body includes cleanup/sorting, vector-gate ownership/admission,
uploads, per-batch render preparation and resource-update execution. The next
probe splits those sections while preserving the same source and behavior.
It must still distinguish CPU work from scheduling or driver waits before
making a causal claim.

Evidence is preserved at
`profiler/experiments/native-scroll-renderer-stages-2026-10-20/manifest.json`.
Capture prefix: `real-scroll-renderer-stages`.
Analyzers: `artifacts/analyze-real-scroll-phases.py` and
`artifacts/analyze-real-scroll-renderer-stages.py`.
No production edit, commit, push or replacement of the normal app was made.

> AI assistance has been used to create this output.
