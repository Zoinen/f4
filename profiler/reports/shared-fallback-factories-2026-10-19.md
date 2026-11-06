# Shared fallback factories — 2026-10-19

## Result and scope

The previous turn made progress by measuring marking groups and identifying
1,180 per-preview QQmlComponent factories. Its ownership experiment was rejected.
This turn removes those per-instance factory definitions, extends rendering and
physical-grid coverage, rebuilds the portable worktree application and measures
the result. Constant 160 Hz without spikes remains unachieved.

Four factories move from GalleryEntryPreview.qml to the existing per-panel
GalleryEntryVisualFactories.qml: aspect raster, tinted raster, source-colour
icon, and fallback marker. GalleryFallbackIconHost.qml retains the original
Loader metadata, geometry, source classification, deferred component exchange
and synchronous creation behavior in a typed instance. GalleryPreviewLoader.qml
gives the failed-thumbnail Loader typed preview context. Ready renderer retention,
the incremental thumbnail host, source reads and scheduling stay as before.
Only immutable factory definitions are shared; source, tint and identity state
remain local to the created instance. No QObject ownership, GC budget, settings,
wire format or native Qt library change is promoted.

The marker leaf gains the existing cached iconPixelAlignment correction. The
new 175% regression first recorded physical y=17.5 in Masonry/Grid/Icons,
y=14.5 in Columns and y=70.5 in Details. It now verifies whole physical origins
and unit transforms for the actual marker and icon leaves. No per-frame rounding
traversal was introduced.

## Regression and rendering evidence

- fallbackFactoriesAreSharedAcrossEntries fails on the original per-instance
  factories in all five modes, before implementation. The final seven cases
  also cover provider-backed aspect raster in Grid and Icons, no local
  QQmlComponent children, identical factory pointers across entries, independent
  selection colours, row-specific marker A/B text and colour, and actual leaf
  scene coordinates/unit transforms at DPR 1.75.
- Existing source-colour rendering coverage now also asserts the source-colour
  raster and Lucide leaf transforms/origins. Captures retain the source RGB,
  marker text, captions, thin Lucide/checker strokes and provider raster paths.
  Images were inspected, including small captions and secondary Details labels.
  A 16x16 interior provider-raster crop alternates exactly two colours at every
  physical pixel on both backends; the pixel summary is archived.
- Final selected suites on Direct3D11 and OpenGL at 175% each pass 27 layout
  engine, 32 mode, 7 session and 3 embedded-import QtTest cases. Totals include
  setup/cleanup. Coverage includes all modes, source replacement, thumbnail
  readiness/retention, SVG resizing and physical-grid/transform checks.
- The first moved-factory invocation caught a missing ZoinGallery.Native
  import (GalleryIconImage unavailable); the explicit import fixes it. The
  failed log is retained. Stronger marker tests then caught the half-pixel
  origins; these failures are retained separately. Visible-leaf lookup handles
  retained hidden renderers; it still requires an actual visible row leaf.
- An additional legacy detailsDelegateMatchesClassicFileListVisualContract
  invocation fails its logical coordinate expectation: actual x=13.14285755,
  expected 13. compactModesCanUseMarkedTextWithoutSelectionBorder passes. That
  legacy failure is not labelled a verified baseline failure here and the full
  suite is not claimed green. The focused golden selection-colour regression
  passes independently on both backends at actual 175%.

## Native census

Before: 28,554 QObjects and 1,426 QQmlComponents; 1,180 components have
GalleryEntryPreview as parent. After: 27,356 QObjects and 257 QQmlComponents;
zero components have GalleryEntryPreview as parent. Per-panel factory owners
hold 52 components across the four retained panels, versus 36 previously.
This confirms removal of the four local definitions per preview. Overall live
factory count falls about 82%, QObject count about 4.2%. Other pool/lifecycle
counts differ slightly between the two snapshots, so not every object delta
is attributed exclusively to this edit.

Census is serialized after input windows, not run as a per-frame scene observer.
Both snapshots use Downloads 186 entries, density 152, 160 Hz and DPR 1.75.
The existing native-object/visual-child walk deduplicates objects; creation URL
counts are context attribution, not exclusive allocated bytes or CPU cost.

## Clean display comparison

A is the preceding uninstrumented scene-context-census host; B contains shared
fallback factories and marker correction. Fresh processes run in A1/B1/B2/A2/A3/B3
order. Each run has two four-second 30 ms wheel windows with boundary reversals,
the same folder/density/DPR/output and 5 ms GC budget. Native frame recording and
census are disabled. No tests, builds, offline analysis or UI Automation run
during these captures. Every run has one Composed: Flip swapchain, active input,
validated motion, zero caption measurements and zero top-level model/snapshot
notifications. Display gaps count only when both endpoints lie inside an input
window; producer intervals are not substituted for display cadence.

| Run | Display gaps >9.375 ms | Maximum display gap, ms | Per-block display p99, ms |
| --- | ---: | ---: | --- |
| a1 | 3 | 12.4957 | 6.3431, 6.3323 |
| b1 | 8 | 12.5655 | 6.4173, 6.3203 |
| b2 | 4 | 12.5016 | 6.3403, 6.2806 |
| a2 | 8 | 12.5332 | 6.3652, 6.339 |
| a3 | 9 | 12.5192 | 6.3749, 6.4885 |
| b3 | 6 | 18.735 | 6.3353, 6.3678 |

A totals 20 long gaps over 24 seconds; B totals 18 over 24 seconds. B1 worsens
the first comparison and B3 includes an 18.735 ms gap. This does not establish a
reliable cadence improvement, and certainly does not prove constant 160 Hz.
The retained factory/object reduction is verified; frame spikes remain.

## Marking follow-up and remaining stalls

Full-source bounded marking/root observers are reused with unchanged Qt and
native preparation code. Original root-traversal capture cold/warm marking:
398,535 / 142,461 visits; shared-factory capture: 400,645 / 142,123. Both have
three cold collections and one warm collection, with the same 40/14 original
drain-batch counts. Class counts account for every recorded visit. Marked heap
volume is essentially unchanged; extra typed QML contexts offset some removed
native factory work. Do not equate the 82% factory reduction with a similar GC
heap or marking reduction.

Warm recursive QObject descendant visits fall 27,924 -> 26,560; recorded root
walk totals 2.9416 -> 2.4565 ms and maximum 1.1399 -> 0.8879 ms. These are
separate instrumented captures with timing variation, not a clean speed proof.
New warm MarkDrain maximum remains 5.1036 ms and sweep maximum 5.4279 ms.
Observers stay below array limits; raw-QPC alignment has sub-microsecond spread
and traces report zero lost events/buffers. GUI and render CPU times overlap.

The new warm trace has two long displayed intervals with zero overlapping GC.
One exact producer interval (end QPC ns 142644285381000) is 10.1226 ms: GUI
running 0.669 ms, render running 5.9956 ms, GPU active 0.5873 ms. Native work
union is 0.1134 ms and incubation zero. Samples include scene-node visits,
QRhiBufferData::assign and the D3D driver. Another (142649390936300) has GUI
2.4621 ms, render 4.2171 ms, GPU 0.4971 ms, incubation 0.2404 ms, and no GC.
Do not add GUI/render running times as serial work or interpret sampled symbol
counts as exclusive duration. This makes retained scene/render resource work a
stronger next target than another arbitrary GC constant.

Next inspect preview subtrees that exist while unused, especially the always
created video-badge subtree on ordinary/non-video entries. Measure its node,
object and binding contribution before introducing lazy shared creation. Keep
ready thumbnails, video badge timing, source identity and alignment regressions.
The existing context/factory reduction alone does not remove the root scene.

## Portable worktree rollout

The normal static host was rebuilt and audited for system-only DLL imports.
Embedded-only QML imports pass on both backends with the new QML files bundled.
The payload was regenerated and extraction/materialization tests pass. The Go
launcher uses CGO_ENABLED=0, GOWORK=off and the writable system Go cache.
Only D:/Code/f4-zoin/f4-zoin.exe and its process tree were replaced; direct
--gui=qt launch, staged launcher equality, extracted-host equality and empty
startup stderr are verified. Canonical D:/Code/f4/f4.exe is untouched. Native
source after packaging changes only in test coverage; production payload is
the tested implementation. No commit or push was made.

Archive: profiler/experiments/shared-fallback-factories-2026-10-19. It contains
source snapshots, before failures, final test logs/captures, all six clean-run
summaries, censuses, compact observer data, exact scheduling/attribution results,
builders/analysis scripts and binary/source hashes. Executables, libraries,
build outputs, raw ETW/PresentMon CSVs and the generated payload are excluded.
The full goal remains active.

AI assistance has been used to create this output.
