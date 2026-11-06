# Bounded managed-thumbnail texture residency

The worktree now uses an event-driven, per-QML-engine LRU retaining up to 64 MiB
of nominal thumbnail factory data. It removes warm thumbnail atlas uploads in
the native capture with Qt's original 2 MiB unused-image allowance. It has **not**
demonstrated a frame-pacing improvement or completed the constant-160-Hz goal.

## Implementation

GalleryThumbnailImage subclasses the existing Qt Quick Image implementation.
After pixmapChange, it pins a ready pixmap only when caching is enabled, its URL
matches the requested source, and the source is an immutable `image://.../zg-thumb-*`
ID. It copies the pixmap reference, not pixels. The pool is keyed by factory
identity; hits update the LRU. Source reads, frame callbacks and scene traversal
are not involved. Oversized factories are not retained.

The pool belongs to QQmlEngine. Runtime shutdown disables retention and clears
the extra references. Engine destruction destroys the pool. Existing active
Image references and Qt's own unused cache are separate; 64 MiB is a bound on
this pool's nominal factory cost, not total process/GPU memory.

Only Masonry managed thumbnails and folder mosaic child thumbnails enable this
retention. Grid/Icons behavior and mutable local-file caching are unchanged.
The production Qt dependency library and its unused-image budget are unchanged.

## Verification

The new native test exercises the 8 KiB boundary with 4 KiB factories, duplicate
touches, actual LRU eviction, reference survival through Qt cache purging,
release after shutdown, mutable/unrelated/empty sources, oversized sources,
zero budget, revisions, separate engines and engine destruction. The ordinary
thumbnail rendering test checks that the native image participates in the pool
and runtime shutdown clears it. Folder leaves assert their caching contract.

The selected native rendering suite and installed-tree-independent QML import
check pass on D3D11 and OpenGL at DPR 1.75. The rendering suite checks source
replacement, failed loads, bitmap retention, folder grid pixels, contained
geometry, physical leaf origins and effective unit vectors, Details batching,
Lucide raster sizing and Columns text. Captures include folder captions, mosaics
and bitmap replacement; the Masonry capture was inspected. These are selected
checks, not a claim that the full existing suite is green.

The CMake-generated long test response files cause truncated-path link errors
with the installed Windows tools. Tests were linked from the same compiled
objects/libraries using the existing helper that normalizes one argument per
line. No linker-symbol override or ignored unresolved symbol was used.

## Native resource capture

Two 10-second wheel windows per stage, same process/engine for cold and warm;
160 Hz, DPR 1.75, 2194 × 1186, Downloads 186 entries, density 152, 30 ms wheel
input, 2 ms GC. The bounded native ring has no dropped/incomplete records or
unmatched source/factory joins. Snapshot confirms Qt's allowance is 2,097,152
bytes, not the earlier diagnostic 64 MiB allowance.

| Stage | Managed factories born | Managed atlas uploads | Managed upload source bytes | Uncached releases | Long displayed intervals |
| --- | ---: | ---: | ---: | ---: | ---: |
| Cold | 175 | 57 | 22,962,220 | 0 | 7 |
| Warm | 6 | 0 | 0 | 0 | 2 |

Warm UI resources still create nine factories / ten atlas uploads, 842,452 source
bytes. Ten cache eviction events account for 1,977,508 nominal bytes. Evictions
record shrink operations; the event does not distinguish pressure from periodic
aging. Six warm managed factories have no atlas upload, so factory births alone
must not be counted as repeated rendered uploads.

Both warm misses occur without an atlas upload: 12.4814/12.5048 ms displayed,
11.5769/10.0171 ms producer intervals. Removing uploads does not explain away
the remaining misses. The cold maximum is 18.7495 ms, also without an upload.

## Clean cadence comparison

Control is the previous bounded-Details clean host; candidate includes the
native residency change. A–B–B–A order, fresh process per run, two 3-second
settling windows followed by two 10-second measured windows. No texture probe,
native frame recorder, UI Automation observer, build or analysis runs during
measurement. Host hashes are retained in the identities JSON.

| Run | Measured extent | Intervals >9.375 ms | Maximum displayed interval |
| --- | ---: | ---: | ---: |
| A1 | 3649.142857 | 2 | 12.4905 ms |
| B2 | 4118.142857 | 2 | 12.5112 ms |
| B3 | 3649.142857 | 2 | 12.4975 ms |
| A4 | 3649.142857 | 2 | 12.4791 ms |

Each version has four long displayed intervals over 40 measured seconds.
One candidate extent differs, despite identical entries/density/window/DPR;
therefore this is not a fully matched-layout A/B test. No cadence win is claimed.
Misses occur from 163 ms to 8.386 s after window start, not solely at the initial
frame or a block boundary.

## Portable rollout

The Windows system-DLL audit passes. Embedded QML import isolation passes.
The native host was compressed into the Go payload and the worktree launcher
rebuilt with CGO_ENABLED=0, GOWORK=off and the system Go cache. Embedded-host
payload/extraction/reuse/concurrent-launch/corruption/lookup tests pass.

Only the worktree launcher tree was stopped. The previous executable was kept
as an exact rollback file; the staged launcher replaced D:/Code/f4-zoin/f4-zoin.exe
and was launched directly with --gui=qt, without F4_EXT_UI_PATH or a sibling host.
The cached host PID 34000, launcher PID 123096, has SHA-256
3c762b11207cf01c85c8587c5f69e93d7e75b2644db986ecdd9bf3c2e30bb12b,
matching the native build. The window responds and the launch error log is empty.
The canonical D:/Code/f4/f4.exe was not modified. No commit or push was performed.

## Remaining work

Attribute the non-upload misses with aligned native thread scheduling/GC/
incubation records. Explain and control the 3649/4118 extent variation before
claiming matched cadence comparisons. Investigate the six unrendered warm factory
births and remaining UI resource churn if their measured cost is material.
The unchanged missed-refresh count means constant 160 Hz remains unachieved.

AI assistance has been used to create this output.
