# Managed thumbnail texture lifetime investigation

The 160 Hz goal remains unmet. Native lifetime records confirm that decoded
thumbnail reuse does not imply texture-factory reuse. A bounded cache experiment
removes most redundant warm uploads, but a displayed-frame miss remains without
an atlas upload in its interval.

## Method

Qt 6.11.1, Windows D3D11, DPR 1.75, 160 Hz, 2194 × 1186 window,
Downloads with 186 entries, Masonry density 152. Each cold/warm stage contains
two 10-second boundary-bounce wheel windows, with 30 ms input spacing. Cold and
warm stages share one process and engine. Original delegate roots, nearby
scheduler and collector remain enabled; GC budget is 2 ms.

An isolated native probe records texture factory construction, source association,
texture creation, destruction, unused-cache eviction, refcount-zero releases and
atlas enqueue scopes. It uses a bounded atomic ring of 65,536 records and writes
the snapshot after measurement. These captures have no dropped or incomplete
records, unmatched joins or source conflicts. No UI Automation observer, native
build or offline analysis ran during capture.

Qt source provenance and hashes are retained in the accompanying experiment
archive. The input QImage cache keys distinguish repeated factory creation from
new decoding. Atlas byte counts represent nominal source image bytes, not GPU
memory use, exclusive CPU time or total transfer traffic. Source attachment and
texture ownership use pointer lifetime joins.

## Results

Warm-stage managed thumbnails:

| Configuration | Factory births | Atlas enqueues | Source bytes enqueued | Resource factory births |
| --- | ---: | ---: | ---: | ---: |
| Original cache disabled | 816 | 425 | 114,055,524 | 5 |
| Managed cache enabled, Qt default 2 MiB unused budget | 783 | 341 | 89,241,792 | 212 |
| Managed cache enabled, diagnostic 64 MiB unused budget | 36 | 37 | 1,724,708 | 0 |

The original warm capture reuses identical managed source URLs, requested sizes
and input QImage cache keys across repeated factory creation. All 818 thumbnail
refcount-zero releases are uncached. This is redundant presentation-resource work.

The 2 MiB candidate incurs 936 warm cache evictions, including collateral churn
of embedded UI resources. Its warm scroll extent is 3649.142857 rather than the
4118.142857 original/64 MiB extent, so it is not a matched settled-layout cadence
comparison and should not be promoted on displayed-frame counts.

The 64 MiB candidate records no cache evictions in either stage. Cold managed
thumbnails produce 120 factories and 77 atlas enqueues (22,599,132 source bytes).
Warm releases are 800 cached and 40 uncached; 36 factories still recur for six
managed URLs. Total warm factory construction scope union is 0.2023 ms and atlas
enqueue scope union is 0.3985 ms over the measurement windows.

Displayed intervals exceeding 9.375 ms are 2 cold/2 warm for the original,
3 cold/0 warm for the 2 MiB candidate, and 5 cold/1 warm for 64 MiB.
The 64 MiB warm miss is 12.4961 ms displayed / 10.9481 ms producer interval,
with zero atlas enqueues in that interval. These instrumented short captures
establish resource churn reduction, not a clean cadence win or completion.

## Sources and current implementation state

`ThumbnailMemoryCache::nextProviderIdLocked` embeds owner, immutable revision,
source size, decoded dimensions and a unique serial in each managed provider ID.
Published IDs retain their pixels; source changes publish new IDs. Consequently
these IDs can be cached independently of mutable local file URLs.

The working-tree experiment enables Image.cache only for managed image-provider
IDs in the shared Masonry thumbnail shader layer. Grid and mutable file URLs
remain uncached. An isolated copy of parkedBitmapKeepsReadySource fails before
this edit and passes afterward on D3D11 and OpenGL at DPR 1.75, including existing
leaf origin/transform checks and rendered captures. The assertions have not yet
been transferred to the production test source.

The 64 MiB budget is diagnostic-only, selected before Qt initialization by
F4_DIAGNOSTIC_PIXMAP_CACHE_MIB in a separate patched Quick library/host. Snapshot
metadata confirms 67,108,864 bytes. This is the unused Qt pixmap-cache allowance,
not a limit on referenced images, Gallery decoding, total RSS or VRAM.

The normal running worktree application remains launcher PID 22772 / cached host
PID 42012, host SHA-256
509c4b470b89b3171bb4f8f7e31faf9d9fc9011800c2e1bc1bdebfb50aa893ca.
The native build artifact contains the pending QML candidate, but the Go embedded
payload has not been regenerated or deployed. Diagnostic processes were stopped
by their exact worktree-specific launcher path. No commit or push was performed.

## Next work

1. Resolve source/cache initialization ordering: the cache binding currently
   depends on Image.source and can permit an initial uncached load. Prove the
   remaining 40 warm uncached releases with a focused regression before changing
   the component or introducing a native cache-policy setter.
2. Choose and test bounded production residency, including engine teardown,
   revision replacement, mutable-file invalidation, memory limits and expiry.
   Do not ship the QML-only candidate with the thrashing 2 MiB default.
3. Repeat clean interleaved cadence captures with matching settled layout and
   collect native scheduling evidence for remaining misses without uploads.
4. Run portable import/dependency gates, rebuild the embedded payload and restart
   the worktree application only after a production candidate is validated.

## Follow-up: remaining uncached sources

The initialization-order hypothesis was tested and rejected. Making the cache
policy depend on the retained URL and making source publication read that policy
first increases uncached warm releases from 40 to 96, with 92 factory births
and 91 atlas enqueues. It passes the focused rendering test but does not solve
the native lifetime issue. That dependency edit was reverted.

Source IDs identify the actual remaining work as folder preview mosaics:
`zg-thumb-f4-left-folder-*` IDs, with small decoded sizes such as 83 × 94 and
55 × 100. Their Image lives in GalleryFolderPreview.qml and has a separate
cache:false setting. The warm source lists and source size associations are
retained in the native event JSON; this is direct attribution, not an inference
from matching file extensions.

A focused assertion in folderPreviewGridAndPixels fails on the original
Masonry child Image.cache=false and passes for the candidate. The candidate
caches immutable managed IDs in Masonry folder mosaics, leaving Grid/Icons
and mutable file sources unchanged. Existing leaf origin, effective unit-vector,
contained-grid and rendering checks pass on D3D11/OpenGL at DPR 1.75. Captures
were retained and the Masonry caption and child images inspected. The cache
assertions for both ordinary and folder thumbnails now also exist in the real
test source, though the final production test target has not been rebuilt yet.

The folder-cache diagnostic uses the same 64 MiB limit and original scheduling:

| Stage | Managed factory births | Managed atlas enqueues | Managed source bytes | Cached/uncached releases | Long displayed intervals |
| --- | ---: | ---: | ---: | ---: | ---: |
| Cold | 92 | 48 | 21,042,488 | 767 / 0 | 3 |
| Warm | 0 | 0 | 0 | 724 / 0 | 1 |

Warm scrolling records 51 cache eviction events totaling 8,890,964 nominal bytes,
but no new factories or atlas uploads. The probe records cache shrink operations;
this alone does not distinguish pressure from Qt's periodic unused-cache aging.
No dropped/incomplete records or unmatched joins occur. Window, entries, density,
DPR and warm scroll extent match the original/64 MiB run. The one warm displayed
interval is 12.4622 ms, producer 10.9227 ms, with no atlas upload.

The pending source now includes both Masonry managed-cache changes and the
regressions. The cache budget remains diagnostic-only. The normal worktree
launcher PID 22772 / cached host PID 42012 remains alive on the unchanged embedded
payload. The folder diagnostic was stopped by its exact launcher path.

Next: implement and validate bounded production residency; run clean interleaved
captures and attribute the remaining non-upload miss. Zero warm texture churn
does not prove constant 160 Hz, and the QML changes should not be deployed alone
with the default 2 MiB unused cache allowance.

AI assistance has been used to create this output.
