# Native scrolling: individual SVG upload costs, 2026-10-20

Goal remains constant 160 Hz without spikes; this capture does not establish completion.

## Workload and probe

Current real GalleryScrollAnimation controller; Downloads (186 entries), Masonry,
DPR 1.75, 160 Hz, default GC budget 5000 us, diagnostic pixmap cap 2 MiB.
Cold and warm captures each contain two 10-second wheel windows, 30 ms input spacing.
General frame recorder disabled. Public phase events and renderer scopes use bounded,
pre-touched native arrays. Individual uploadBatch calls are timed after early guards;
only calls >= 0.1 ms are recorded. Nested map/unmap/create/update times must not be
added together. RTTI material labels and byte counts are collected after the timer.
The diagnostic renderer object is linked before the current patched Qt Quick archive;
there is no alternate renderer in the normal application payload.

## Observations

Only actual input windows count. Cold: 399 slow uploads; warm: 388.
CurveFill + CurveStroke account for 167,423,472 bytes cold and 165,492,144 bytes warm
in these thresholded records. Cold fill/stroke time totals 81.8582 ms; warm 84.3463 ms.
These are thresholded totals, not all upload traffic.
An unmerged stroke batch repeatedly contains 2,054,448 vertex bytes and 186,768 index
bytes. Another contains 1,344,420 + 122,220 bytes.
The largest active warm upload is 1.2226 ms, with 1.056 ms in unmap, including
0.4525 ms full dynamic update and 0.6019 ms buffer creation.
The 17.1629 ms SmoothColor startup sample lies outside measurement windows and is
excluded from scroll findings.

Public preparation maxima per block: 5.9006, 5.9926, 6.6819, 5.8352 ms.
In the worst warm frame, alphaUploads takes 4.6358 ms, alpha render preparation
0.5183 ms, pending admission 0.0008 ms, and resourceUpdate 0.0206 ms.
There are 71 producer intervals above 9.375 ms over the four windows. Display cadence
is recorded separately in real-scroll-phase-findings.json; producer gaps are not
proof of displayed missed refreshes. No constant-160 claim follows from this run.

## Next experiment

Exact viewport root culling removes ready subtrees as soon as they leave the visible
region. Re-entry can reconstruct renderer elements and require upload again. This is
a source-based hypothesis, not yet a measured causal result. Test retaining only roots
already visited by the native camera within its existing proven prepared range.
Never reveal prepared-but-unseen roots early; leave SVG presentation priority and
readiness tied to the exact visible region. Check recycling, ownership, final pixel
alignment, rendered equivalence, and backend behavior before measuring again.

The baseline archive includes source, link/build identity, raw captures, and analyzers.
The normal application remains on the previous verified build with native motion off.
