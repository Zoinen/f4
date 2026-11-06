# Cold scrolling: geometry uploads and synchronous video metadata

## Workload and limits

Downloads, 184 entries, Masonry density 152, initial extent 4161.142857,
actual DPR 1.75 and 160 Hz. Wheel delta 120 every 30 ms, four seconds in each
direction. Captures run without compilation or UI Automation. Clean timing
runs have only the basic native hooks and PresentMon; diagnostic runs add
explicit observers and must not be treated as production frame measurements.
The interval/batch-retention renderer remains an unshipped experiment.

Constant 160 Hz is still unproven and contradicted by the cold-scroll tails.

## Per-source geometry census

A temporary after-sync census traverses generated vector subtrees and records
geometry storage as vertex count times vertex stride plus index storage.
It does not measure actual driver allocation or GPU residency. It costs scene
traversal time and is disabled for clean captures.

| SVG | Geometry bytes | Geometry nodes |
|---|---:|---:|
| FileIcons_Dark+_nb_pro_1.svg | 4,525,920 | 73 |
| FileIcons_Dark+_nb_pro_1 (1).svg | 2,680,848 | 73 |
| FileIcons_Dark+_nb_pro_1 (2).svg | 2,680,848 | 73 |
| PlayerControl+FileIcons_Dark++_nb_pro_5+.svg | 2,243,088 | 69 |
| FileIcons_Dark+.svg | 653,904 | 22 |
| FileIcons_Dark+64.svg | 558,432 | 22 |

Two sources can finish geometry in one sync. One source can also publish
partial geometry before its remaining paths finish. These observations explain
why assigning only one new source per frame does not bound later upload work.
Artifacts: `vector-geometry-baseline-census.jsonl`,
`vector-geometry-census.jsonl`, and the `vector-census160-wheel-masonry` capture.

## Two experiments not retained

Serial admission waits for the currently admitted surface to leave Loading
before assigning another source. It separates the observed heavy geometry
completions. The clean run still has 17 downward display intervals over
9.375 ms, the same count as the earlier clean interval-renderer capture.
Downward display p99 is 12.5016 ms; upward has five such intervals. There is
no sufficient end-to-end benefit to retain it. The four-line QML change is
reverted; its patch is `artifacts/vector-serial-admission.patch`.

A separate renderer trial creates initial vertex/index buffers as Dynamic
instead of Immutable. On this D3D11 implementation those map to different
driver allocation/update paths. Its clean downward display p99 is 12.5012 ms,
maximum 18.761 ms, with 14 intervals over 9.375 ms; upward has two. This single
capture does not establish a reliable improvement over the prior 17/two result,
and does not justify changing memory policy across backends. It remains local
under `artifacts/qt-dynamic-initial-clean`; no production change is retained.

Clean captures: `vector-serial-clean160-wheel-masonry` and
`dynamic-initial-clean160-wheel-masonry`, including PresentMon CSVs and summaries
under `artifacts/modes-profile`.

## GUI-thread attribution

A temporary QGuiApplication notify wrapper records slow outer event delivery.
GetThreadTimes samples are quantized to 15.625 ms here and cannot establish
whether individual events are CPU-bound or waiting. Use the event wall times
only. Nested scoped probes then measure the actual callback chain.

The first probe finds DecodeManager completion events lasting 4–6 ms. Subsequent
probes isolate onImageReady deliveries up to 8.1338 ms; queue scheduling in
that active capture peaks around 0.266 ms. Per-request tagging establishes
that the slow cases are successful panel video thumbnails, not background
viewer resampling or null-image failures.

The final two diagnostic captures establish the synchronous chain:

| Stage | One representative callback |
|---|---:|
| onImageReady | 7.0344 ms |
| ExternalCatalogModel::handleImageReady | 7.0267 ms |
| handleImageInfo / handleImageInfos | about 7.0 ms |
| publishImageInfoBatch | 6.9924 ms |
| emit dataChanged for metadata | 6.9883 ms |

In the next run, the same path takes 6.4089 ms in metadata dataChanged,
6.4064 ms in Masonry onDataChanged, and 6.3992 ms in
handleLightweightMasonryDelta. Other video deliveries take about 2.9 and
4.8 ms. These are inclusive timings, not additive independent costs.
Some separate visual snapshot updates also reach 6.4682 ms.

Artifacts: `dispatch-profile160-wheel-masonry`, `decode-stages160-wheel-masonry`,
`decode-details160-wheel-masonry`, `decode-details-v2-160-wheel-masonry`,
`decode-details-v3-160-wheel-masonry`, and `metadata-delta160-wheel-masonry`.
Their JSON contains bounded guiDispatch and decodeStages arrays. Probe sources
and separately linked diagnostic hosts are under artifacts; product sources
are unchanged.

## Source explanation and next regression

ExternalCatalogDecode.cpp handles video imageReady by copying request.info,
updating dimensions, setting isLast, then calling handleImageInfo. The cached
request's isCached flag survives. applyImageInfoToRow correctly detects new
geometry, but also propagates that flag into allChangedMetadataCached.
publishImageInfoBatch consequently emits CachedMetadataBatchRole.

MasonryLayoutModelDelta.cpp interprets that role as an atomic metadata-restore
batch and calls flushLightweightRewrap immediately. Ordinary late geometry
completions use scheduleLightweightRewrap instead. Thus late video thumbnail
completion can enter the synchronous startup-restoration path. This conclusion
follows from both the measured callback nesting and the role-selection code;
the requested cache-origin tag has not yet been separately logged.

The next change should distinguish late decoded-video geometry from atomic
startup metadata restoration while preserving the truth of isCached, source
authority, revision checks, fields, and correct aspect ratio. First add a
regression for a cached video request completing with new dimensions: it must
not acquire CachedMetadataBatchRole or force an immediate rewrap. Keep the
existing cached startup batch regression. Then measure callback cost and actual
display intervals; merely moving identical work into a timer is insufficient
proof of improvement. Large first-time vector uploads remain a separate cost.

AI assistance has been used to create this output.
