# Remaining 160 Hz scrolling stalls

## Capture conditions

Windows, Qt 6.11.1 static D3D11 host; 160 Hz landscape display, actual DPR
1.75, 2194 × 1186 logical window. Downloads, Masonry, 184 entries, density 152.
Wheel delta 120 every 30 ms, four seconds down and four seconds up. No UI
Automation observer ran during capture. The diagnostic host records Qt frame
stages; PresentMon records actual presentation, and WPR CPU sampling supplies
stacks. This is native profiling, not a QML-profiler trace.

Baseline source: parent 25718f35e, gallery 0dbdc3c750. Diagnostic artifacts:

- `artifacts/modes-profile/spikes160-cpu-wheel-masonry.json`
- `artifacts/modes-profile/spikes160-cpu-wheel-masonry-present-summary.json`
- `artifacts/modes-profile/spikes160-cpu-wheel-masonry.etl`
- `artifacts/modes-profile/spikes160-cpu-correlation.txt`

ETW export had zero lost events/buffers. Sample stacks were joined by event
timestamp **and thread ID**, rather than assuming adjacent Stack records belong
to the preceding sample: the export interleaves other threads' stacks. Of 5469
process samples in the measured interval, 5456 had matched stacks. ETW relative
timestamps were correlated with the native steady clock using trace-start
FILETIME and a wall/QPC calibration; the calibration bracket was 1100 ns.
CPU sampling and stack walking themselves add overhead; use this run for
attribution, not as an uninstrumented performance claim.

## Findings

The first pass displayed a median interval of 6.2501 ms, p95 8.8805 ms,
p99 18.7542 ms, maximum 37.4937 ms. The return pass had p95 6.809 ms,
p99 8.8955 ms and maximum 18.7476 ms. All recorded presents were displayed,
but that does not mean every 160 Hz refresh received a new frame.

Three costs are directly visible:

1. **GUI-thread SVG loading.** The 27.08 and 29.89 ms native frame intervals
   contain respectively 12 and 15 samples in
   `QQuickVectorImagePrivate::loadFile`. Qt's local 6.11.1 source confirms that
   setting `VectorImage.source` synchronously calls `QQuickItemGenerator`.
   `asynchronousShapes` does not make this generation asynchronous. Shape
   preparation also runs on worker threads; this is a separate stage.
2. **Repeated icon classification during row delivery.** A 18.969 ms interval
   contains 16 GUI samples, 15 in `QtShellController::applyDecodeResult` and
   12 in `F4IconSet::fileIconSource` / `QDir::match`. The call path includes
   `F4GalleryRowsResponseReducer::normalizeRows` and inactive-panel comparison.
   Qt constructs and compiles a new regular expression for every wildcard on
   every match. These immutable manifest patterns were being recompiled for
   every file and every delivery.
3. **Render batch uploads.** A 19.3735 ms interval contains 18 render-thread
   samples, including nine in `QSGBatchRenderer::Renderer::uploadBatch`.
   Several first-pass long frames have `sceneRebuild == 65535`. Buffer work
   remains a separate problem; removing the GUI cost does not solve it.

An additional warmed attribution pass peaked at 11.17 ms and its largest
frames used incremental rebuild state 4. Warmed behavior cannot substitute for
the first pass through a folder.

## Implemented: immutable icon matchers

[F4IconProvider.cpp](../../qt/host/src/F4IconProvider.cpp) now prepares the
manifest filters once. Plain names use case-insensitive equality, simple
suffix patterns use case-insensitive suffix comparison, and complex patterns
reuse compiled regular expressions. Matching order and the existing priority
of exact configuration filenames are preserved. Separator-containing inputs
retain Qt's wildcard behavior. There is no per-file cache to grow without bound.

The regression in
[F4IconProviderTests.cpp](../../qt/host/tests/F4IconProviderTests.cpp) classifies
184 distinct CR2 names within one 6.25 ms frame budget. It failed before the
fix at **42.1305 ms** and passed after at **2.1288 ms**, with a subsequent full
suite run measuring **1.977 ms**. Constructor time is excluded from this
catalog-delivery test. The manifest test checks generated names, upper-case
variants and prefixed names against the old ordered `QDir::match` behavior.

Focused tests pass. The full icon suite reports 38 passes and one failure:
`preparedFileIconResourcesPreserveChromeOriginals` cannot open an original
Lucide resource at line 322. This is outside the matcher path; the suite is
not claimed green. Embedded-host extraction tests, QML import isolation,
compiled-host loading and the Windows system-DLL audit pass.

## Remaining work and limits

The post-fix native capture (`spikes160-icons-wheel-masonry`) still has a
37.5104 ms maximum display gap on the first pass. The return pass has median
6.2498 ms, p95 6.2977 ms, p99 10.0076 ms, maximum 12.4731 ms. The new capture
uses the same scroll extent 4161.142857 and entry count as the CPU capture,
but the latter had CPU sampling enabled; this is **not** a controlled frame
time before/after comparison. The isolated classification regression proves
the removed work; these end-to-end results prove constant 160 Hz is unfinished.

Next address GUI-thread vector scene generation without rasterizing SVGs,
discarding visible work, or deferring all loads until scrolling stops. Preserve
bounded viewport/prefetch acquisition and cancellation. Then attribute and
reduce batch uploads, followed by low-overhead cold/warm presentation captures.
No claim of spike-free scrolling is supported yet.

AI assistance has been used to create this output.
