# Alpha-batch uploads dominate the remaining render-preparation outliers

A finer diagnostic split of the real native-scroll renderer places most of
the previously unexplained preparation time in the alpha-batch upload loop.
Resource-update execution, upload admission and sorting are small in the
same frames. The next attribution is inside uploadBatch/map/unmap, including
whether the data is merged and which material owns it. No production change
is justified by a guessed upload cause; constant 160 Hz remains unfinished.

## Diagnostic integrity

The complete batch-renderer object was copied from the currently linked patched
Qt source and instrumented with section scopes. It is linked explicitly ahead
of the normal Qt archive; no multiple-definition override is used. Existing
upload/order/retention behavior is retained. The input source SHA-256 is
`1d82e95a365287c68f9d5e0118d69fbdc7a16f4196aa172c92e31479da24bb9b`.
The generated source, compile command and object SHA are recorded together.
Both the public-phase buffer and the section buffer are pre-touched before
QGuiApplication construction. Section capacity is 262,144; checks reject
overflow, incorrect process/thread IDs and invalid public-phase/PresentMon
correspondence. Inclusive function spans are merged by interval union.

One fresh process supplied two 10-second cold and two 10-second warm blocks,
with the real GalleryScrollAnimation and unchanged production control logic.
Conditions remain Downloads/186 entries, density 152, 160 Hz, actual DPR 1.75,
2194 x 1186 logical window, 30 ms bouncing wheel input, default 5000 us GC budget
and the diagnostic 2 MiB pixmap-cache limit. General frame recording and UIA
observation stayed off. No build, test or analysis ran during input windows.
Instrumentation and extra scope boundaries have overhead; these are wall
intervals rather than exclusive CPU costs.

## Longest preparation frame in each block

| Block | Public preparation | prepareRenderPass | Alpha upload loop | Resource update |
| --- | ---: | ---: | ---: | ---: |
| Cold 1 | 6.4529 ms | 5.7840 ms | 4.0163 ms | 0.0431 ms |
| Cold 2 | 6.5961 ms | 6.1992 ms | 4.6404 ms | 0.0162 ms |
| Warm 1 | 5.3334 ms | 5.0213 ms | 3.7443 ms | 0.0145 ms |
| Warm 2 | 5.9927 ms | 5.6610 ms | 4.3868 ms | 0.0151 ms |

In these frames, pending vector admission took approximately 0.0004-0.0007 ms,
sorting 0.009-0.011 ms and gate ownership 0.001-0.061 ms. This does not support
blindly expanding admission bounds or changing pixel rounding. The ordinary
alpha upload loop includes uploads for batches without a vector gate as well
as already-ready gated surfaces; it is not enough to label the cause SVGs,
caption geometry, a buffer allocation or a graphics-driver wait.

The next probe records slow uploadBatch calls with material, merged state,
actual vertex/index byte counts and nested map/unmap/buffer-update timings.
It should capture only diagnostic metadata and preserve the current behavior.

Evidence: `profiler/experiments/native-scroll-alpha-upload-2026-10-20/manifest.json`.
Capture prefix: `real-scroll-renderer-parts`.
No production edits, commit, push or normal-app restart were made in this step.

> AI assistance has been used to create this output.
