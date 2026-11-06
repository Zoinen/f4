# Remaining masonry upload stalls after timer correction — 2026-10-18

## Scope and capture validity

Current production gallery includes the nearby-preparation settling fix. The renderer uses the normal buffer-fit policy; GC remains 5000 us. Active 160 Hz window, Downloads with 184 entries, density 152, actual DPR 1.75. Each capture explicitly injects 268 wheel events at 30 ms. These are attribution captures, not clean performance comparisons. The normal app was restored afterward; no further production changes were made.

`nearby-fixed-sampled-wheel-masonry` records ETW context switches, CPU samples and PresentMon. No lost events or buffers were reported. Raw-QPC context-switch alignment matches 50,725 event identities/order, with 900 ns offset spread. WPR stopped successfully and reports no active recording. Wall-clock calibration would have been off by 110.5 us at the beginning, reinforcing use of the validated raw-QPC mapping.

## Execution versus scheduling

The worst native frame period was 16.9416 ms; its render scope was 13.0205 ms. Context switches attribute 13.0185 ms to scheduled execution and 0.002 ms to ready time. Scheduled execution includes kernel/driver work and is not synonymous with pure user-mode CPU time. Sampled stacks include prepareRenderPass (10), uploadBatch (7), unmap (6), buffer create (4), and memcpy_repmovs (2). Counts overlap and are not summed into time.

Across the measured windows, GUI ready delay p99/max was 0.022/0.169 ms; render ready delay p99/max was 0.009/0.029 ms. This capture supports reducing work rather than a priority increase. A separate slow GUI interval includes GC; neither thread's bottleneck explains every display gap. The sampled capture had additional roughly 9.6 ms display intervals that are not used to claim clean-run cadence or attributed wholesale to the same render stall.

## Focused renderer scopes

`nearby-fixed-upload-wheel-masonry` rebuilds the batch-renderer translation unit with bounded scopes, leaving resource policy intact. The worst render scope is 11.969 ms. It includes an order-exhaustion event and a full render-list build (0.4928 ms), followed by 29 geometry uploads (6.0632 ms inclusive) with about 9.92 MB of vertex-buffer sizes plus index work. Dynamic-copy scopes total 3.0370 ms for 9,021,324 bytes. Eighteen growth/recreation calls total 1.4457 ms; three new-buffer calls total 0.2209 ms. Individual buffer creation maximum in those groups is 0.2588 ms. Nested upload/copy/create durations must not be added together.

Large first-node identities recur in multiple nearby frames, including 1,344,420-byte and 2,054,448-byte groups. Recurrence alone does not prove geometry contents were unchanged.

## Membership evidence changes the next experiment

`nearby-fixed-upload-counts-wheel-masonry` extends only the observer: upload records encode merged state in bit zero and member count in higher bits. Of 29 recorded uploads larger than 100,000 bytes, 23 are unmerged groups containing multiple elements; only six are single-element groups. The largest repeated 1,344,420-byte groups each contain 36 elements and are uploaded twice. Other groups contain 4, 6, 11, 12, 22 or 34 elements. The worst render scope in this capture is 14.8522 ms, with 26 recorded uploads and 9,314,712 vertex-buffer bytes. No observer event overflow occurred in the inspected worst frames.

The earlier single-element unmerged-retention experiment cannot cover most of these large groups. Its inconclusive cadence result is not evidence against retaining unchanged multi-element unmerged groups, nor does this new capture prove that a broader retention patch is safe.

## Next required experiment

Record geometry invalidation and member identity/order across these rebuilds. Build a regression with a multi-element unmerged batch that fails on redundant uploads when only render-order numbering changes. Retain its raw buffers only if every member survives with unchanged geometry, compatible material/opacity classification, and the same safe ordering/membership. Exercise geometry mutation, insertion/removal, clipping, root lifetime, overlapping alpha drawing, and both native backends at 175%. Avoid geometry-buffer handoff between unrelated owners; previous experiments showed pixel regressions there. If correctness passes, measure upload bytes and repeated clean actual-wheel display cadence before deciding whether to promote.

Constant 160 Hz without spikes remains unachieved. Evidence is archived in `profiler/experiments/remaining-masonry-upload-stalls-2026-10-18/`; raw ETW/CSV/capture JSON remain under `artifacts/modes-profile/nearby-fixed-*`.

AI assistance has been used to create this output.
