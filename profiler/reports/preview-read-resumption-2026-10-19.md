# Preview read resumption and scrolling — 2026-10-19

## Finding and change

The exact-QPC warm trace in `post-url-gc-budget-and-warm-stalls-2026-10-19.md`
identified a `suspendPreviewReads(false)` completion inside a missed producer
interval. The child catalog advertised `ImageFullSizeRole` for every row when
read authority resumed, although dimensions had not changed. Masonry interpreted
this as size invalidation and scheduled a lightweight rewrap.

`third_party/ZoinGallery/src/embed/ExternalCatalogModel.h` now publishes
`ImageIdUrlRole` instead. This uses the existing thumbnail-request path without
refreshing unchanged dimensions or invalidating ready visual snapshots. It
preserves current-viewport requests after a retained preview is resized while
its source reads are suspended. No GC budget, prefetch policy, setting, wire
format, or UI geometry changed.

## Regression evidence

- `GallerySessionTest::directoryPreviewSurvivesNavigationWithoutAuthority`
  failed before the fix: unchanged dimensions were advertised during resumption.
  The existing no-extra-decode, unchanged dimensions and retained URL checks
  remain. It now also requires the request-readiness role.
- `MasonryLayoutModesTest::resumedPreviewReadsFetchNewlyExposedCells` covers Grid
  and Masonry with 16 real distinct images. It resizes a retained view while
  reads are suspended, verifies its last cell has no thumbnail, resumes reads,
  and verifies that cell loads without geometry changes. In Masonry it first
  emits the old size-role payload and proves that it schedules a rewrap, then
  verifies actual resumption does not schedule one.
- Final Direct3D11 and OpenGL checks at DPR 1.75 each passed 27 layout-engine,
  25 mode, 7 session, and 3 embedded-import QtTest cases (totals include QtTest
  setup/cleanup). Selected coverage includes staged thumbnail replacement,
  folder navigation, source replacement, vector resize, visibility, and static
  leaf pixel-grid/transform checks. Captures of Columns small labels and Masonry
  captions/icons were inspected. No layout was modified by this change.
- An initial test compile used a nonexistent convenience getter; it was
  corrected to query the public model role. The failed build log is retained.

## Clean display comparison

Alternating fresh processes: A1, B1, B2, A2, A3, B3. A is cached-URL baseline;
B includes this notification change. Each run uses two 4-second wheel windows,
30 ms input repeat, boundary reversals, Downloads (186 entries), density 152,
DPR 1.75, 160 Hz primary output and the existing 5 ms GC budget. No native frame
recorder, tests, builds, UI Automation, or offline analysis ran during capture.
All runs had one swapchain, Composed: Flip, active input, validated motion,
zero caption measurements, and zero top-level model/snapshot notifications.
Those top-level counters do not count the child-preview notification being fixed.
Only display intervals with both display endpoints inside an input window count.

| Run | Display gaps >9.375 ms | Maximum display gap, ms | Per-block display p99, ms |
| --- | ---: | ---: | --- |
| a1 | 6 | 12.503 | 6.4122, 6.3199 |
| b1 | 3 | 18.7497 | 6.3216, 6.3303 |
| b2 | 3 | 18.7262 | 6.3404, 6.3524 |
| a2 | 6 | 12.5201 | 6.3203, 6.3346 |
| a3 | 4 | 12.6034 | 6.3417, 6.2739 |
| b3 | 3 | 12.5785 | 6.3129, 6.2967 |

A had 16 long intervals over 24 seconds; B had 9 over 24 seconds. This sample
favors fewer misses, but B1 and B2 each contain an approximately 18.7 ms gap,
while A maxima are approximately 12.5 ms. This is not evidence of constant
160 Hz or a guaranteed cadence improvement. The deterministic unnecessary
rewrap is removed; remaining spikes need fresh exact-QPC attribution.

## Follow-up evidence and remaining work

The previous same-process 5 ms/2 ms GC-budget cohort's misses were located
relative to wheel-window starts. They span 111–3944 ms, including late-window
misses. Therefore the theory that all remaining misses are first-scroll text
renderer switches is unsupported. No text-renderer or GC policy was changed.
The next measurement should attribute the remaining current-build misses to
GUI scheduling, component creation, renderer work, and any overlapping GC.
CPU on GUI and render threads must not be added as serial frame time.

## Portable worktree rollout

The normal static host was rebuilt and its DLL imports audited. Embedded-only
QML imports passed on both backends. The compressed payload was regenerated;
Go extraction/materialization tests passed and the launcher was built with
CGO_ENABLED=0, GOWORK=off and the system Go cache. Only the worktree launcher
process tree was replaced. Runtime identity verifies its direct --gui=qt launch,
extracted child hash equality with the new host, staged launcher equality and
zero startup stderr. The canonical checkout was untouched. Hashes and PIDs are
in `artifacts/preview-resume-identity.json` in the archived experiment.

Evidence archive: `profiler/experiments/preview-read-resumption-2026-10-19/`.
Executables, libraries, raw ETW traces, raw PresentMon CSVs and build outputs
are excluded. The full 160 Hz goal remains active. No commit or push was made.

AI assistance has been used to create this output.
