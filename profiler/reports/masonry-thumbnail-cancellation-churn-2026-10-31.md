# Masonry thumbnail cancellation churn, 2026-10-31

## Result

The repeated cold-cache misses are real cancellations, predominantly of five
large PNG files and one JPEG. They are not successive larger thumbnail tiers,
source-version churn, or a stream of final decode failures. All 311 misses in
the measured reverse pass belong to these six immutable sources. Their
requested sizes remain constant at 293x257 or 293x258 physical pixels.

This identifies wasted source/decode admissions, not yet their exact cancelling
call site or their causal contribution to renderer preparation. No production
cancellation policy was changed in this pass. Constant 160 Hz remains unmet.

## Capture and observer

Case: `real-scroll-postsync-thumbnailrequestscold-b1-20261031-wheel-masonry`.
Real Downloads: 192 entries, Masonry density 152, actual DPR 1.75, 160 Hz,
default frame latency 2, isolated cold cache and normal initial viewport.
The native foreground observer reports no loss of foreground during either
measurement window. Prepared-root retention is off, original pipeline-key
mode is retained, typed registry and uniform word hash are on.

The copied ThumbnailMemoryCache records source identity, revision, source byte
size, transform, owner, requested size and successful decoded/published sizes.
Hits are aggregated; misses, pending coalescing, new/reused publications,
retry/final releases and cancellations receive in-memory timestamps. Output
is serialized after the capture, with no per-request logging during motion.
This is an attribution capture including probe overhead, not a throughput A/B.

All 168 source tuples and 1,938 non-hit events fit their 4,096/32,768 limits;
both overflow counts are zero. Per-source event totals exactly match recorded
rows except hits, which intentionally have no individual rows. The preserved
215,149,626-byte raw packet has SHA-256
`4368ac04b336b22333988945c08fd6cbd86e8e3227730edc92a30780e32772aa`.

## Measured requests

| Window | Misses | Cancellations | New publications | Reused publications |
| --- | ---: | ---: | ---: | ---: |
| First 30 seconds | 449 | 399 | 54 | 3 |
| Reverse 30 seconds | 311 | 310 | 1 | 0 |

First window: each of the six dominant sources has 51 misses and 51
cancellations, with no publication. Reverse window: the five PNGs each have
52 misses and 52 cancellations. The JPEG has 51 misses, 50 cancellations and
one successful 292x257 publication. The PNG source sizes range from 21.6 to
23.4 MB; the JPEG is 8.0 MB. The exact production OSVFS/local/path hash tuple
independently maps their source IDs to the current Downloads files; filenames
are preserved in the analysis artifact rather than inferred from byte size.

The whole capture has 944 misses, 717 cancellations, 226 new publications,
13 reused publications, 38 pending coalescences and no retry/final releases.
The six sources have stable revision, byte-size and transform tuples. Six
other source identities have two transform variants, corresponding to video
contact sheets and posters; they do not account for the reverse-pass churn.

The event API does not carry admission tokens. Do not infer exact request
lifetimes or an ABA race from same-owner cancel/resubmit chronology alone.
Window boundaries can cut across requests. A final release would mean
retryWaiters=false, not necessarily decoder failure; none was recorded here.

## Performance remains failing

| Window | Preparation median | Preparation p99 | Preparation maximum | Missed display intervals | Maximum motion error |
| --- | ---: | ---: | ---: | ---: | ---: |
| First | 0.5731 ms | 3.6692 ms | 9.1633 ms | 134 | 129.4057 px |
| Reverse | 0.5249 ms | 3.2382 ms | 8.9820 ms | 118 | 121.8340 px |

Coverage holds are zero. Both windows fail the unchanged 9.375 ms display
interval / 2 px maximum-motion gates. No missing-frame pair was excluded.

## Verification and existing test correction

The probe passes 43 helper tests, all 46 native renderer tests and seven
motion cases in enabled and disabled modes. Thirty rendered PNGs match
byte-for-byte between modes and against the preceding observer baseline.
Three isolated QML import checks and the dependency audit pass.

The existing ThumbnailMemoryCache test expected a miss when a 123x77 request
was followed by 123x78 after decoding 123x69 pixels. The unmodified archived
baseline fails that same sole assertion: extra unused height does not enlarge
the aspect-fitted pixels. The test now expects a hit at 123x78 and retains a
real larger-tier miss at 124x78. All 15 cache cases pass against the unchanged
baseline and the instrumented cache in both modes. Cache behavior was not
changed to satisfy the test. Original failure outputs and equality receipts
are retained.

Initial diagnostic build failures were an inherited older observer header
and a missing public include path. Both were corrected before live execution;
the failed logs are preserved. The compiled main uses the exact V2 batch
header, with a hash equality guard. No installed Qt SDK files were modified.

## Next investigation

Attribute the cancellation call site before choosing a policy change.
Masonry can cancel through reread/geometry, spatial-window rebasing and
request-key rebasing; ExternalCatalogModel also cancels on reset, source
invalidation, thumbnail disabling and shutdown. Its local geometry path
aggressively cancels work while expensive VFS work has a preservation policy.
The current event observer does not distinguish these callers, so attributing
all events to the geometry path would be premature.

Any candidate must preserve bounded work and immediately prioritize the
visible viewport/viewer; retaining an unbounded offscreen queue is not an
acceptable fix. Measure cold, verified warm and middle-scroll acceptance
after a caller-specific regression and correction.

The owned diagnostic host and Go process were terminated by exact path and
ancestry after analysis. The normal worktree app was left running unchanged.
No commit or push was made.

Evidence: `profiler/experiments/masonry-thumbnail-cancellation-churn-2026-10-31/`
contains 175 compact sources, scripts, results and images, plus a manifest of
13 hash-verified external build/raw receipts. Large binaries and the raw
packet remain outside that compact evidence directory.
