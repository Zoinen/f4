# Metadata enrichment and Masonry geometry

## Result

Resolved local paths and timestamps for an unchanged opaque revision no longer
announce image classification or geometry changes. This removes a live redundant
layout-update path, with a regression that fails before the fix. Clean 160 Hz
comparisons still show similar outlier counts. Constant 160 Hz remains unfinished;
no consistent cadence gain is claimed.

## Attribution

An artifact-only copy of the current metadata translation unit records bounded
native timings and change masks. It is linked as a direct object ahead of the
current core archive, without duplicate-symbol overrides or stale core libraries.
The probe observes fields without changing decisions. Its thread-local vector
reserves 8192 records; the capture records 591 including startup, with no growth
or cap hit. JSON serialization happens after capture. Clock calls/recording add
overhead, so these numbers are attribution data, not clean performance evidence.

Downloads, 186 entries, density 152, extent 4101.142857, active 160 Hz, DPR 1.75,
30 ms wheel events, combined renderer and normal GC settings match prior runs.
The analyzer uses separate direction windows from the native frame records,
excluding the pause between directions.

133 metadata row applications occur inside those windows. 67 change only local
path and timestamp (mask 132), with changedSource false, dimensions unchanged
and metadata-settled unchanged. Another 66 have changedSource true and mask 508
(path, source identity/version/resource/key, timestamp and size); their final
dimensions and settled state also remain unchanged. The latter remain source
invalidations under the existing source-authority contract.

Row application totals 4.0695 ms, max 0.2320 ms. Its nested updateImageInfo
totals 1.8809 ms, max 0.2190. Source invalidation totals 0.1245 ms, max 0.0081.
24 synchronous changed-row emissions total 26.8775 ms, max 1.4979. Nested times
must not be added to enclosing times. Several longest frame intervals, including
14.0434 and 13.7795 ms, overlap zero row-application or notification-emission
work. Metadata therefore cannot explain every stall.

## Implementation and tests

Changed rows now carry a notification class: source replacement, local path,
timestamp and geometry, or their relevant combination. Same-source enrichment
announces LocalPathRole/LastModifiedRole plus VisualSnapshotRole. Actual size or
settled-state changes announce the dimension/settled roles. Source replacement
keeps the full existing invalidation list. Adjacent equivalent notifications
remain coalesced; all replacements share one class regardless of which source
fields changed.

The opaque-source materialization regression now loads an actual provider-backed
PNG, enriches the same revision with its backing path/timestamp and checks:
path/time/snapshot notifications remain; image type/dimension/URL notifications
are absent; dimensions, version and local path remain correct; repetition is
silent; a changed content version still invalidates the image. Before the fix,
it fails on the unwanted IsImageRole. Afterward it passes. The mixed-range test
also gives adjacent replacement rows different timestamp changes, verifying
they still coalesce into one replacement range.

Selected metadata/preview/video compatibility tests pass on Direct3D 11 and
OpenGL at DPR 1.75: 13 Gallery results, six Masonry mode results and four
resource-only import/shader results per backend. This change adds no static UI
layout or pixel-rounding operation.

## Clean comparison

Four fresh-process actual-wheel captures use A1/B1/B2/A2 order. A is the prior
production metadata-role fix; B adds enrichment-specific notifications. No WPR,
QML profiler, native field recorder, UI Automation observer, role counter,
object census, build or graphics tests run during these captures. Both hosts
use the same combined renderer/resources, active 160 Hz/DPR 1.75, 186 entries,
30 ms wheel input and zero caption measurements. Strict display intervals have
both endpoints inside the direction window.

| Run | Down p99 ms | Down max ms | Down >9.375 ms | Up p99 ms | Up max ms | Up >9.375 ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| A1 | 6.3095 | 12.5091 | 3 | 6.3034 | 6.3960 | 0 |
| B1 | 6.3468 | 18.7475 | 3 | 6.3029 | 6.3518 | 0 |
| B2 | 6.3060 | 12.5023 | 2 | 6.2924 | 12.4862 | 1 |
| A2 | 6.3296 | 12.5225 | 4 | 6.2730 | 6.3317 | 0 |

Six candidate long intervals versus seven controls does not establish a useful
gain, and candidate maximum is worse. After capture, replacement classes were
normalized for coalescing across differing source-field masks; all observed
replacement rows had the same mask, so that normalization does not explain a
cadence result. Compatibility tests cover the normalized final implementation.

## QML binding follow-up

A separate current-candidate QML profiler capture records binding, JavaScript
and signal-handler events. Client exits zero; the 11,529,235-byte XML has its
closing trace element. It is not a clean performance comparison. Two sustained
contentY-change clusters identify scrolling, not exact QPC frame boundaries;
the first cluster is about 3.0 seconds and the second 4.1 seconds, so it does
not cover every native scroll interval.

Recurring inclusive costs include iconPixelAlignmentActive (742 evaluations,
36.4160 ms total, max 0.5563), preview commitComponent (280 calls, 33.9398 total,
max 1.6312), its Loader sourceComponent binding (230, 32.9263 total, max 1.6163)
and folder-loader activation (308, 29.9569 total, max 0.6426). Nested costs must
not be summed. The longest chain begins with bitmapThumbnailSource, cascades
through thumbnail source and markerNeeded, and reaches 5.7728 ms inclusively.
This does not establish 5.77 ms of ordinary marker computation: GC and called
native work are included. Correlation with native GC phase timestamps is the
next necessary step before another binding refactor.

## Rollout

The normal static host builds and passes the Windows system-DLL audit. The
embedded payload and CGO_ENABLED=0 launcher are regenerated. Payload,
materialization/reuse, concurrent extraction, corruption and lookup tests pass.
The worktree launcher is restarted directly with --gui=qt; its extracted child
matches the built host hash and stderr is empty. No canonical process is
terminated, and no commit or push is performed.

Compact diagnostics, test results, timing summaries, source snapshots and build
identity are archived with SHA256 manifest under
profiler/experiments/metadata-enrichment-geometry-2026-10-18. Raw QML XML remains
under profiler/traces, and binaries/build products remain outside the bundle.
