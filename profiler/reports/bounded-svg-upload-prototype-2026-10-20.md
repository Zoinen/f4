# Bounded SVG upload prototype, 2026-10-20

The 160 Hz goal remains active. A multi-node unmerged-geometry prototype now bounds
large vector transfers to 512 KiB per step. Native regressions and one real scrolling
capture pass. It is not in production: controlled on/off comparison, visibility latency,
and additional input paths remain required before promotion. The normal worktree app
still runs the visible-node-admission build documented in the preceding report.

## Starting evidence and copy experiment

The preceding candidate recorded a 2,241,216-byte stroke batch upload taking 3.9301 ms,
including 3.0696 ms in mapped copy. Its warm counterpart took 1.4481 ms, including
1.0626 ms copy. Limiting batches per frame cannot bound a single oversized batch.

An isolated D3D11 probe compared std::memcpy with aligned SSE2 streaming stores in
actual mapped Dynamic vertex buffers. Byte verification passed for 64 KiB, 1 MiB and
2,054,448 bytes. Each size had 256 alternating samples. Neither method consistently
improved the copy. At 1 MiB medians were 0.7923/0.7921 ms; at 2,054,448 bytes,
0.1081/0.1074 ms. These unused-buffer microbenchmarks do not model GPU contention
or prove why the real outlier occurred. They justify rejecting a speculative copy-routine
change, not a universal bandwidth claim. No SIMD change was added to production.

## Prototype and iterations

The prototype intercepts admission of large unmerged geometry. Existing buffer
selection and retirement budgets are retained. Transfer buffers use Static storage;
updates are queued through QRhiResourceUpdateBatch, preserving contents across frames.
Direct Dynamic mapping cannot be split safely because it may discard prior contents.
Qt 6.11 documentation confirms that distinction and permits offset static-buffer updates.

Progress walks the still-owned batch geometry, consolidating a bounded chunk from
its members into one resource update. It avoids both a persistent full CPU shadow and
whole-batch staging allocation. Only limited per-batch progress metadata persists.
Batch invalidation and renderer destruction cancel progress; dirty geometry restarts it.
The existing gate cannot prepare/draw the partial batch; the complete SVG becomes
visible only after all its batches finish. Priority sorting and aggregate admission byte/
time checks remain. Supported index sizes must already match the backend's effective
index format; conversion and merged geometry use the existing upload path.

- v1: single-node 256 KiB proof, completion and replacement tests passed.
- v2: bounded probes and aggregate ongoing-transfer byte accounting added. Full D3D11
  suite passed 44 checks but failed the two existing oversized completion cases:
  a 2 MiB plus 24-byte fixture needs nine 256 KiB steps, exceeding its eight-frame limit.
  Failing results are retained. No production test expectation was weakened.
- v3: 512 KiB steps to reduce visibility delay while keeping each copy bounded. Built,
  but superseded before running a full suite.
- v4: multiple unmerged nodes supported, targeting the actual large stroke batch shape.
  Uses transient bounded consolidation and avoids whole CPU staging allocation.

## Regressions and rendering

The new fixture covers completion, replacement during transfer, and multiple unmerged
nodes. Against the current production library, all three fail on both backends because
the entire large batch completes in one frame. With v4, all three pass, including no
partial drawing, correct replacement pixels, DPR 1.75 leaf origins and unit transforms.
Rendered complete/replacement/multi-node images are retained; the multi-node image
was inspected and has crisp expected rectangles. These fixtures contain no text.

Full v4 suites, each on D3D11 and OpenGL:

- Renderer: 47 passed, zero failures/skips, including existing eight-frame completion.
- Camera: 30 passed, zero failures/skips.
- SVG presentation/animation: 14 passed, zero failures/skips.

## Real scrolling capture

real-scroll-vector-chunks-v4 uses Downloads (186 entries), Masonry density 152,
DPR 1.75, 160 Hz screen, 30 ms bouncing wheel, two cold and two warm 10-second
windows. Production native scrolling/precise clock remain enabled. No build, test,
offline analysis or UIA observer ran during input. The successful-link receipt pins
prototype source, probe, generator, actual renderer object and underlying production
Qt library. Probes are bounded and pre-touched; no source tracing is enabled.

Transfer records stayed below capacity 8192: cumulative cold 400, warm 799.
Within input windows, cold 381 steps transferred 158,282,048 bytes; warm 380 steps
transferred 157,757,760 bytes. Every step was <=524,288 bytes. Step CPU p99/max:
cold 0.2231/0.3192 ms, warm 0.1894/0.2310 ms. This scope measures consolidation and
queuing, not the later QRhi/GPU work; the full pipeline metrics remain necessary.

Public preparation p99 by block: 2.1072/1.9604/1.8836/1.9135 ms.
Preparation maximum 3.3055 ms; render-to-frame-end maximum 4.0089 ms.
The immediately preceding production-source capture had preparation maximum 4.0084
ms and render-to-frame-end maximum 5.0526 ms. This is a different-binary temporal
comparison, not a controlled causal estimate. The earlier 6.9 ms outlier remains valid.

All four blocks had zero missed display intervals (>9.375 ms), zero out-of-presentation
camera commits, and passed p95 <=0.5 / maximum <=2 physical pixel motion-error gates.
Display maxima: 6.4772/6.3584/6.4047/6.3717 ms.
Motion error maxima: 1.1679/0.8011/0.8910/0.6253 physical pixels.
Producer-present interval spikes still exist; they are distinct from display misses.

## Next verification

Add a diagnostic-only same-binary off switch and bounded per-source transfer identity
and completion timestamps. Compare enabled/disabled captures, including complete
render/submit timing and source-to-visible delay. Check rapid wheel and middle-button
scrolling. Extend cancellation checks to geometry mutation, source generation changes,
priority preemption, and hidden/visible transitions. Audit memory bounds and unsupported
fallback cases. Only then consider the portable recipe, production library/host rebuild,
packaging/import/extraction gates and worktree restart. Do not promote merely because
one capture has no display misses. No commit or push was performed.
