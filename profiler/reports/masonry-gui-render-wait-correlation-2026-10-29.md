# Masonry GUI/render wait correlation and follow-up checks

## Status

The constant-160-Hz goal remains active and unproven. This turn extended native
regressions, tested an isolated catalog lookup optimization, and correlated GUI
waits with render-thread activity in the existing validated CPU/scheduler trace.
It did not produce an accepted new scrolling capture. No production source,
embedded payload, dependency package, commit, or push was changed.

## Long GUI waits overlap substantial render-thread work

Source case: `real-scroll-current-renderer-cpu-20261029-wheel-masonry`.
Trace SHA-256:
`22ad147a0d830668c91ac4071031c483dc2ae02eb75a57adc054e49a91ad0a24`.
GUI thread 122760; render thread 13492; process 17624. This is the earlier
renderer trace, not a new measurement of the bounded-cache/overlay candidate.

The analysis clips render-thread scheduler intervals and recorded frame phases
to the actual GUI blocked intervals. GUI switch-in stacks identify Qt's
`QWaitCondition::wait` / `QSGThreadedRenderLoop::polishAndSync` path. Missing
phase coverage is reported explicitly rather than assigned to a guessed stage.

| GUI blocked interval | Render scheduled | Render blocked | Recorded work overlapping the interval |
| --- | ---: | ---: | --- |
| 19.2617 ms | 19.2617 ms | 0 | Preparation 13.1281 ms; recording 2.3220 ms; submission 1.0534 ms; synchronization 0.9847 ms |
| 16.3280 ms | 13.3938 ms | 2.7775 ms | Preparation 8.1258 ms; a render wait resumes in `QRhiD3D11::beginFrame` |
| 16.0674 ms | 5.0553 ms | 10.9350 ms | Render waits resume in `QRhiD3D11::beginFrame`, including one full 8.7476 ms wait |
| 15.4070 ms | 1.3978 ms | 13.9947 ms | Three clipped waits resume in `QRhiD3D11::beginFrame`; 14.1884 ms lacks emitted phase coverage |
| 14.9936 ms | See raw analysis | See raw analysis | Includes 7.9291 ms of a geometry-map wait and a separate 4.1250 ms uniform-map wait |

Scheduled time includes interrupt/DPC execution and is not a pure instruction
time measurement. Stack locations identify where a wait resumes, not the
driver's private lock object or GPU execution duration. These correlations
support investigating render synchronization/pacing and expensive preparation;
they do not prove that every spike has one cause or quantify the latest cache's
effect. Buffer-map waits had already been identified separately; this analysis
connects them to specific long GUI waits.

## Additional GUI attribution

The same trace attributes all 106 `PanelCatalogModel::rowLoaded` self samples to
`F4GalleryBridge::requestPanelCatalogRows`. Of the `QMap<QString,QVariant>::value`
self samples, 64 occur beneath `MasonryLayout::prepareDelegateRow` during native
scroll pose updates. Meta-property lookup samples also include pixel-grid chain
registration and delegate preparation. These are measured code paths to inspect,
not evidence that rounding should be added or alignment disabled at rest.

## Catalog lookup prototype

An isolated helper reads stored `QVariantMap` and QString values by const
reference, avoiding temporary map/value/string copies on the usual path. It
retains the original conversion fallback and sparse-row bounds checks.

Five QtTest results pass, including comparison with the original implementation
for dense and sparse catalogs, missing/empty/converted IDs, malformed entries,
replacements, and invalid sparse offsets. The test executable links the original
`PanelCatalogModel.cpp` separately from the candidate helper.

The interleaved ABBA microbenchmark uses 24 batches per catalog, 200,000 lookups
per batch (12 batches per implementation), 191 populated entries, and 40 extra
metadata fields per entry:

| Catalog | Baseline median | Candidate median | Median lookup reduction |
| --- | ---: | ---: | ---: |
| Dense | 89.276 ns | 71.062 ns | 20.40% |
| Sparse | 99.020 ns | 73.777 ns | 25.49% |

These are isolated per-lookup measurements, not scrolling frame improvements.
This small hotspot cannot by itself explain the multi-millisecond waits above.
The helper has not been integrated into the production model or live host.

## Overlay behavior coverage

The expanded candidate suite passes 16 QtTest results. It sends actual wheel
events through the window as well as exercising the modal filter directly.
With no popup, normal delivery still receives the wheel once while containment
queries fall from two to one. The filter-only query falls from one to zero.
The installed baseline and feature-off replacement fail precisely those two
new zero-redundancy assertions.

The suite covers modal/modeless popups, all eight two-popup modality/dimming
combinations, inside/outside positions and closing popups. An initial assertion
about closing two dimmed modal popups failed in both the installed baseline and
candidate. The final test records that installed Qt edge behavior independently
and compares the replacement against it; it does not silently declare the edge
behavior correct. Initial and settled failure outputs are retained.

## Capture activation and storage

A new capture handshake waits for explicit activation before queuing gestures.
Computer Use selected the unique returned diagnostic window and refreshed it
after failure. Both activation attempts returned exactly:
`failed to activate captured window`. The handshake timed out; no scrolling
gesture was queued. The foreground observer and capture failure records remain
available. Qt's active flag alone is insufficient, as the preceding B2 capture
demonstrated. No performance acceptance guard was relaxed and no UI action was
repeated after the documented activation retry failed.

The full D: drive was recovered without losing evidence. Two inactive old
executables were backed up as gzip files on C:, their decompressed SHA-256 values
verified, temporarily removed, and restored at their original paths after other
inactive executables were losslessly compressed. All restoration/content checks
passed. The earlier experiment's 13 hash-pinned external outputs were rechecked
and still match. About 919 MB remained after this turn's evidence freeze.

The exact diagnostic launcher tree was stopped. The ordinary worktree application
at `D:\Code\f4-zoin\f4-zoin.exe` remains alive, PID 43936.

## Evidence and next action

Frozen evidence:
`profiler/experiments/masonry-overlay-focus-and-gui-waits-2026-10-29/`.
Its manifest identifies 60 compact files and 12 external test binaries/objects.
It includes the analysis scripts/results, native tests and original failures,
microbenchmark batches, capture handshake, activation observer output, and
verified storage recovery receipt/script. The earlier trace remains referenced
through its existing evidence manifests.

The next substantial investigation is why GUI synchronization waits span D3D11
frame pacing and the preceding render work, alongside the remaining preparation
spikes. The catalog and overlay changes are bounded optimizations to validate,
not substitutes for the original cold/warm wheel and quarter-height middle-scroll
gates at 160 Hz and actual DPR 1.75.
