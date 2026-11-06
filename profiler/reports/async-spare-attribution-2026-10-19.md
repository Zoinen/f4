# Remaining misses with asynchronous delegate spares — 2026-10-19

The 160 Hz objective remains unachieved. This is diagnostic attribution of the
isolated spare-reserve prototype, not a production change or a clean comparison.

## Capture and validation

Cold and warm captures use the same PID 42332, GUI thread 45656, render thread
62860, engine identity 15cff8 and 2194×1186 window. Downloads contains 186 entries,
Masonry density is 152, actual DPR is 1.75, and both active/primary displays report
160 Hz. Each capture has two 10-second boundary-bounce wheel windows with 30 ms
input spacing. The original collector uses a 2 ms budget. No builds, tests, offline
analysis or UI Automation overlap acquisition. Per-frame native recording and
object census are disabled; caption measurements and model/snapshot-only signals
are zero. Native spans, scheduling and PresentMon use the aligned QPC clock.

Cold means first scrolling pass in a fresh diagnostic process with warm disk
caches. Warm means the next pass in that same engine. Extent changes from
4101.142857 to 4118.142857 logical pixels, so these are not identical settled-layout
workloads. The cold run consumes 72 completed reserve delegates. The warm run
consumes/starts/completes none: existing slots suffice. Both have zero reserve
failures and maximum ready count two.

Final cumulative buffers contain cold/warm counts of 2019/2581 GC records,
59713/114820 preparation records and 421/731 incubation records, below their
32768/262144/32768 capacities. Raw context-switch alignment matches 108069 cold
and 102184 warm switches at 10 MHz, with 900 ns offset spread in both. ETW reports
zero lost events and buffers. Misses map to producer timestamps within 1 µs.
Host map symbols preserve ICF aliases; DLL/kernel PCs and inlined functions remain
partly unresolved. Sparse CPU samples do not give exclusive function durations.

The cold capture has five display intervals >9.375 ms, maximum 12.5150 ms. Warm
has four, maximum 12.5114 ms. These instrumented counts are not a clean A/B result.

## Native scope costs

Counts/maxima below include complete calls wholly inside input windows. They are
wall time, not scheduled CPU. Parent/child scopes overlap and must not be summed.

| Scope | Cold count / max ms | Warm count / max ms |
| --- | --- | --- |
| Nearby callback | 939 / 3.9615 | 918 / 3.9082 |
| Property commit | 1433 / 2.9194 | 1389 / 2.9174 |
| Row preparation | 54304 / 1.4741 | 52131 / 0.3627 |
| Controller incubation | 384 / 3.3220 | 302 / 4.2158 |

All observed incubation requests specify 2 ms. The largest warm incubation call
lasts 4.2158 ms with no GC overlap. A requested incubation budget is not a hard
upper bound on completing its next operation. The largest warm nearby call lasts
3.9082 ms with no GC; moving root creation ahead of demand does not remove
assignment, nested component creation or other per-row work.

Cold GC DoSweep steps reach 6.3344 and 6.1660 ms inside input. Some cold misses
have GC overlap. All four warm misses have zero GC overlap throughout their
recorded surrounding producer intervals. The collector alone cannot explain the
remaining warm misses.

## Warm miss attribution

| Present QPC ns | Producer ms | GUI running ms | Render running ms | Preparation union ms | Controller incubation ms |
| --- | ---: | ---: | ---: | ---: | ---: |
| 171045734241500 | 7.2347 | 5.2233 | 3.7537 | 3.4442 | 0 |
| 171048766864400 | 9.6406 | 1.8903 | 9.5506 | 0 | 0.0676 |
| 171048914865100 | 8.6094 | 3.8934 | 4.6724 | 1.9898 | 0 |
| 171056265165000 | 9.6126 | 1.7867 | 4.7876 | 0.0911 | 0.2544 |

GUI and render running times overlap; do not add them as serial frame time.
Producer intervals, display intervals, CPU time and GPU-active time are distinct.
Controller incubation scopes only cover incubateFor; direct nested creation can
appear in stacks inside another scope without appearing as controller incubation.

The 9.6406 ms producer interval has 9.5506 ms render running time, only 0.0670 ms
waiting and 0.0220 ms ready-but-unscheduled. It has no GC or preparation work, and
the warm run creates no reserve roots. Its display interval is 12.5114 ms, display
latency 11.4316 ms and GPU-active duration 0.7038 ms. This is actual render-thread
CPU work, not primarily CPU starvation or steady GPU saturation in that interval.

Its 11 sparse render samples include scene traversal/batch preparation, texture
atlas enqueueTextureUpload/commitTextureOperations, texture subresource upload
construction and QByteArray copying, shader-resource binding, D3D endFrame and
driver/kernel activity. Atlas/copy functions appear in two inclusive samples;
that does not establish their exclusive cost or that every miss has this cause.
The longest controller incubation call ends about 0.328 ms before this producer
interval begins, so it must not be reported as work inside this interval.

## Next experiment: measure texture-factory churn

Repeated atlas upload/copy work during a warm pass makes raster residency a
specific candidate. Qt 6.11.1's [pixmap-cache implementation](https://raw.githubusercontent.com/qt/qtdeclarative/v6.11.1/src/quick/util/qquickpixmapcache.cpp)
sets a 2 MiB budget for currently unused image data and shrinks that cache when a
pixmap is unreferenced. This is not a limit on all referenced images or the
application's decode cache. Eviction may lose texture-factory identity even when
our own decoded QImage remains cached; whether it explains these uploads is still
a hypothesis.

An older raster-factory pin experiment did not improve timing. Its current
observer implementation scans panel image descendants in frame callbacks and
only pins zg-thumb sources. Do not treat that negative result as proof about an
event-driven cache covering the actual repeatedly uploaded sources; do not adopt
a cache on this hypothesis alone either.

Before changing retention, record bounded native factory creation and atlas-upload
counts/bytes, correlate image cache keys/factory lifetimes where possible, and
identify which raster sources are recreated. Avoid per-frame UI enumeration.
Then compare a bounded event-driven residency prototype against the original
collector/scheduler/root path, measuring upload reduction, memory and first-visible
latency as well as clean alternating display cadence. Keep SVG geometry, immutable
revision identities, cancellation and 175% leaf alignment intact. GUI assignment,
incubation, atomic GC and other render work remain independent concerns.

## Tooling and preserved state

A target-thread scheduling exporter now retains GUI/render switches, readiness and
samples plus metadata and interrupts. Its regression against a previous complete
trace preserves all 152361 target switches and their original 900 ns alignment.
The new cold/warm exports are 146013707 and 126352719 bytes. Original ETL remains
complete. Earlier generated CSVs were gzip-compressed and independently SHA-256
verified after decompression before removing uncompressed copies; no original
trace was discarded. Export/analysis occurs outside acquisition.

Only diagnostic source copies/libraries/hosts changed. The normal worktree app
remains on host SHA-256
509c4b470b89b3171bb4f8f7e31faf9d9fc9011800c2e1bc1bdebfb50aa893ca.
The diagnostic panel was restored and only its exact-path launcher tree stopped.
No commit, push or production rollout occurred. Compact tooling, native data,
analysis and identities are archived under
profiler/experiments/async-spare-attribution-2026-10-19; executables, libraries,
raw ETL/CSV and dumps remain excluded.

> AI assistance has been used to create this output.
