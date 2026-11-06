# Remaining misses after the allocation reductions

Date: 2026-10-19. Objective: constant 160 Hz without spikes. **Not achieved.**

## Capture and validity

This captures the current scalar-preview and valid-index implementation. Downloads has 186 entries, density 152; the window is active at 160 Hz and actual DPR 1.75. Actual wheel events arrive every 30 ms and reverse near the boundaries, in two four-second blocks. Caption measurement count is zero. All input-sampled trajectory steps move at least one physical pixel, with repeated reversals. This is coarser than proving movement on every rendered frame.

The per-frame GUI/render recorder and allocation census are disabled. Native GC phase scopes, WPR CPU sampling, and PresentMon remain enabled. No QML profiler, UI Automation observer, build, or tests overlap the capture. The GC budget is the production default of 5 ms; timed drain chunk remains 10241. This is an attribution capture, not an uninstrumented release timing guarantee.

The exact raw-QPC join matches 52,556 context switches, with 900 ns offset spread at 10 MHz. The trace reports zero events/buffers lost. The bounded GC log contains 2,522 records, below its 32,768 cap. Host names are offline nearest-linker-map symbols with ICF aliases preserved. DLL/kernel/inlined symbols remain unresolved; a decorated host symbol is not proof of an inlined instruction location.

Display metrics require both display endpoints and the current present inside an explicit input window. Producer CPU attribution uses complete intervals between PresentMon submissions inside those windows. These intervals are not native render/frame CPU durations. All five display misses have an exact producer timestamp match. Context includes preceding and following producer intervals; a display miss must not automatically be blamed on the current producer alone.

## Display cadence

| Block | Display intervals | p99 (ms) | Maximum (ms) | >9.375 ms |
| --- | ---: | ---: | ---: | ---: |
| 0 | 634 | 6.3480 | 12.5000 | 3 |
| 1 | 635 | 6.3043 | 12.4972 | 2 |

## Miss attribution

| Present QPC (ns) | Display gap (ms) | GPU active (ms) | Producer context (ms) | GC-state overlap in that context (ms) |
| --- | ---: | ---: | --- | --- |
| 129808332883800 | 12.5000 | 4.0096 | 5.860, 6.179, 6.571, 5.717 | 0.000, 0.000, 0.000, 0.000 |
| 129808651209400 | 12.4858 | 0.4646 | 5.075, 8.519, 16.049, 5.091 | 0.000, 0.000, 0.000, 3.047 |
| 129808684435400 | 12.4740 | 0.5107 | 9.068, 6.867, 12.200, 3.502 | 5.082, 3.217, 5.541, 1.850 |
| 129814657233000 | 12.4972 | 0.4393 | 5.952, 5.610, 10.705, 5.859 | 0.000, 0.000, 6.576, 4.031 |
| 129814682195200 | 12.4948 | 0.4534 | 5.859, 9.544, 9.560, 1.801 | 4.031, 5.654, 6.595, 0.000 |

Three misses coincide with substantial GC activity. MarkDrain reaches 5.4574/5.3906/5.1678 ms in individual slices. The longest DoSweep is 5.3659 ms, including 5.1594 ms in BlockAllocator::sweep. Stacks during marking contain QObjectWrapper and QQuickItemWrapper traversal. The sweep also contains deleted-QObject wrapper cleanup and QVariant/QUrl destruction. It is no longer justified to describe all GC cost as QColor/value-wrapper destruction.

The longest producer interval is 16.0495 ms, with zero GC overlap. GUI scheduled execution is 6.6788 ms and render scheduled execution 7.6555 ms, within that same interval; these execute partly in parallel and must not be added as serial wall time. GUI stacks include nearby delegate preparation and incubation. Render stacks include command-buffer resource destruction, alpha-batch preparation, buffer data assignment, new shader-resource binding construction, merged/unmerged preparation, and D3D driver command execution. Full available stacks for this and six other long intervals are retained.

The first display miss has no large producer gap or GC overlap, while GPU active time reaches 4.0096 ms and render completion latency 5.0052 ms. Typical other miss rows have GPU time around 0.44–0.51 ms. This is a separate GPU-side outlier; the current data does not resolve its driver/DWM cause. Do not attribute all five misses to GC.

GUI ready-delay maximum is 0.186 ms; render maximum is 0.044 ms. The trace does not support general scheduler starvation as the common explanation. A process named after a switch-out is merely the next scheduled process, not the cause of a wait.

## Source-level render lead

Qt 6.11.1 QRhiD3D11 retains a ResourceBatches snapshot per bind command and destroys the retained values in resetCommands(). ResourceBatches contains stage-specific QRhiBatchedBindings with implicitly shared vectors. The observed constructor/destructor stacks make this churn worth measuring, but do not establish deep copies or its total cost. A separate stack constructs a new shader-resource binding from updateMaterialDynamicData; this is not merely a retained-snapshot copy.

The batch renderer already has a layout-compatible QMultiHash SRB pool, default threshold 1024. It is not a single-entry QHash and no replacement/leak bug is established. Before changing the threshold, copy strategy, or lifetime, measure pool hits, misses, evictions, layout distribution, and snapshot counts/costs. Preserving command-buffer snapshots across later SRB updates is required.

## Next actions

1. Run a budget-only comparison on this reduced-allocation build. Current evidence shows why 5 ms marking competes with a 6.25 ms frame, but earlier budget trials used a different allocation path and were mixed. Keep defaults unchanged until matched clean captures support a change. An atomic 5.16 ms block sweep remains a separate constraint.
2. Instrument SRB pool pressure and D3D command snapshot churn before considering a renderer change. Keep the existing GPU resource lifetime and portable dependency gates.
3. Continue distinguishing GPU outliers from GUI/GC/render work in the same exact-QPC timeline.

No production source, SDK library, default setting, or portable payload was changed in this experiment. The normal worktree app is restored and its cached Qt child hash is verified against the current host. Startup stderr is empty. Five new GC-overlap accounting tests pass, covering nested events, clipping, sweep subphases, empty intervals, and excluding the inter-block gap.

Compact JSON, full selected stacks, source snapshots, reproduction tools, and SHA256 manifest are under `profiler/experiments/reduced-allocation-attribution-2026-10-19/`. Executables, raw ETL/CSV/QTD, libraries, and linker maps are excluded; their relevant identities remain recorded.

> AI assistance has been used to create this output.
