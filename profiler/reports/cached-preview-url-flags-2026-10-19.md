# Cached preview source predicates — 2026-10-19

Goal: constant 160 Hz without spikes. The goal remains incomplete.

## Implementation

GalleryEntryPreview now reuses thumbnailHasSource across visibility and
component-selection predicates. It caches modelIconSource text once and
derives primitive presence/classification values from that text. Icon helper
calls use the cached text rather than repeatedly boxing the URL. The colour
icon caches its sized source string, which also drives its provider-size
predicate. Actual model/thumbnail source properties and renderer Image.source
remain typed URLs. Empty-source checks retain their explicit text semantics.
No layout coordinates, transforms, source retention, loader staging, VFS
leases, GC policy or wire protocol changed.

## Regression and rendering checks

previewSourceClassificationTracksReplacement covers all five modes. It
replaces a real preview loader source with bundled, native-provider, custom
and empty URLs, checking text/presence/monochrome/system classification,
then exercises nonempty/empty/replaced thumbnail URLs. The old code fails
the new cached-text contract in each mode; the new code passes. These
assertions establish cache reactivity, not allocation counts.

Direct3D and OpenGL each pass QtTest totals of 27 layout tests, 22 selected
mode tests at actual DPR 1.75, 3 import tests and 3 staged-session tests.
The selected coverage includes real missing-thumbnail fallback, untinted
custom-icon pixels, SVG source/geometry retention while resizing, staged
replacement without a blank frame, all-mode navigation and static text/icon
leaf origins, unit transforms and raster pixels. Captured Columns text and
Masonry icons were visually inspected.

The first broad invocation included the staged replacement test in the
wrong executable; it belongs to GallerySessionTest. The SVG resize test
also read Loader.source even though that Loader uses sourceComponent after
the existing generated-vector migration. Its assertion now reads the owning
surface URL, checks it against session.vectorSourceAt(0), and watches that
surface for source changes across resizing. The initial failed log is
preserved; rendering code was not changed to address that test-owner error.

## Allocation census

The same extended value/variant census library runs before and after this
change on Downloads (186 entries), Masonry density 152, DPR 1.75, 160 Hz,
original 5 ms GC budget, boundary-bounce wheel input every 30 ms. Each
census covers two four-second scrolls plus intervening work through final
serialization. Caption measurements/model/snapshot changes and counter
overflow are zero. Aggregating native strings/hashes adds overhead, so
these are counts rather than timing measurements.

| Counted creations | Before | After |
| --- | ---: | ---: |
| QUrl variant wrappers | 14,091 | 7,402 |
| All variant wrappers | 18,631 | 12,050 |
| Value-type wrappers | 9,360 | 9,472 |
| Both instrumented routes | 27,991 | 21,522 |

QUrl variant creations decrease 47.5%; combined counted creations decrease 23.1%.
This is not a full-JS-heap allocation or memory-byte measurement. Remaining
value-type counts vary slightly with presentation work; source-site details
remain in the raw census JSON.

## Clean display comparison

Fresh-process A1/B1/B2/A2/A3/B3 order. A is the native-bookkeeping baseline;
B adds these preview predicates. Each uses two four-second input windows,
minimal recorder, actual 160 Hz/DPR 1.75, density 152, 186 entries, the
original 5 ms GC budget and continuous boundary-bounce wheel input. All
workload/active-window/motion checks pass. No census, WPR, QML profiler,
UIA observer, build or test overlaps acquisition. Both displayed endpoints
must be inside the input window; no internal stalls or runs are excluded.

| Run | Block 0 p99 / max ms | >9.375 ms | Block 1 p99 / max ms | >9.375 ms |
| --- | ---: | ---: | ---: | ---: |
| A1 | 6.2903 / 12.5053 | 2 | 6.2866 / 12.4911 | 1 |
| B1 | 6.2964 / 18.7441 | 4 | 6.3132 / 12.4978 | 2 |
| B2 | 6.3542 / 12.5033 | 4 | 6.3109 / 12.4957 | 1 |
| A2 | 6.3288 / 12.5024 | 2 | 6.3262 / 12.4740 | 1 |
| A3 | 6.3356 / 12.5018 | 2 | 6.3317 / 12.4979 | 2 |
| B3 | 6.3026 / 12.5027 | 2 | 6.3128 / 12.4809 | 1 |

Long displayed intervals: A 10, B 14 over 24 seconds of input each.
There is no demonstrated cadence improvement; the candidate has more missed
refreshes in this small cohort and one approximately 18.7 ms interval. The
allocation reduction must not be presented as proof of smoother scrolling.
The change is retained as reduced allocation work while investigation of
the remaining stalls continues. Constant 160 Hz is not achieved.

## Portable rollout and next step

The Windows system-DLL audit and installed-tree-independent import tests
pass. The compressed host payload is regenerated, extraction/reuse/
concurrency/corruption/upgrade-separation tests pass, and the Go launcher
is rebuilt with CGO_ENABLED=0 and the system Go cache. The worktree app is
restarted directly with --gui=qt; the archive verifies its cached Qt child
against the rebuilt host and records empty startup stderr.

Host SHA-256: `dc09edfd6175b34627c16ecac77dbdf614b777cddd7e43bb6640f995bffe5220`.
Launcher SHA-256: `9dc3a107692c716a291fa94e48ab2c95e75eda947ab08c2e4e43b202237c04fe`.

Next correlate GC and GUI/render stalls on this implementation. The remaining
QUrl/value-wrapper counts are candidates, not explanations for every missed
refresh. Preserve the original GC and prefetch settings until the stall
evidence justifies a change. No commit or push was performed.

Evidence, relevant source, initial/final test logs, tooling and rendered
captures are archived with SHA-256 hashes at
`profiler/experiments/cached-preview-url-flags-2026-10-19/`.
Executables, libraries, raw traces and CSV exports remain outside the archive.

AI assistance has been used to create this output.
