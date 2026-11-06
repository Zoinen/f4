# Post-URL-cache stalls and GC slices — 2026-10-19

The constant-160-Hz goal remains incomplete. This investigation changes only
diagnostic builds and measurement tools. Production GC/prefetch settings
remain unchanged.

## Exact-QPC native attribution

`post-url-preparation-{cold,warm}-sampled-wheel-masonry` uses the current URL
predicate implementation, with regenerated current-core instrumentation.
Both passes share PID 45624, engine `15cff8`, GUI 62600, render 17388 and
a 2194×1186 window. Actual/primary refresh is 160 Hz, DPR 1.75, density
152, Downloads 186 entries. Wheel input is 120 units every 30 ms, boundary
bounce, two four-second moving windows per pass. First pass is a fresh
engine with warm disk cache; second reuses the engine/heap. No frame recorder
or UIA observer runs. Caption/model/snapshot-change counters are zero.

| Pass | Matched QPC switches | Offset spread ns | MarkDrain maximum ms | DoSweep maximum ms | Displayed misses >9.375 ms |
| --- | ---: | ---: | ---: | ---: | ---: |
| cold | 52,732 | 900 | 5.4840 | 5.0057 | 5 |
| warm | 44,720 | 900 | 5.2966 | 5.2029 | 2 |

Raw-QPC alignment uses a 10 MHz counter; lost-event/buffer checks pass.
Native event arrays remain below their caps. Phase times include preemption
and instrumentation; they are not exclusive CPU durations. Differences from
earlier runs do not establish a causal sweep-time improvement.

## Warm misses do not overlap GC

| Producer end QPC ns | Producer interval ms | GUI running ms | Render running ms | GPU active ms | GC overlap ms |
| --- | ---: | ---: | ---: | ---: | ---: |
| 137426351801300 | 9.7043 | 2.5480 | 3.6613 | 0.5450 | 0.0000 |
| 137431715187600 | 10.4391 | 6.3165 | 4.7391 | 0.5040 | 0.0000 |

GUI and render execution overlap; their running durations must not be added
as serial frame time. The first warm miss includes shader-effect node data,
batch/hash bookkeeping and D3D resource handling on the render thread. The
second includes 1.6969 ms incubation plus nearby preparation/property commits
and child-model publication on the GUI thread, with D3D resource snapshots,
glyph preprocessing and driver work on the render thread. No single leaf is
assigned a precise CPU percentage from these sparse samples.

Full available GUI stacks in the second warm miss show:
- `qEnvironmentVariableIsSet` under `MasonryLayout::rewrap`, while
  QQmlObjectCreator is installing bindings during incubation.
- `qEnvironmentVariableIntValue` under `diagnosticMasonryDelayMs`, reached
  from model-state updates after a directory-preview child model resumes.
- Model publication through `ExternalDirectoryPreviews` and the child
  catalog/proxy model update path.

The environment-query samples are separate callers. They do not prove that
environment lookup alone explains the miss. ICF aliases and unresolved
kernel/driver symbols remain explicit in the archived full stacks.

## Clean same-process 5 ms versus 2 ms experiment

The clean URL-cache host, without native timing probes, receives one
prespecified 5 ms warm-up, then ABBA and mirrored BAAB passes. A is 5 ms;
B is 2 ms. All nine captures share PID 69272, engine `15cff8`, GUI 6220,
render 41444, window 2194×1186, DPR 1.75, actual 160 Hz, density 152 and
186 entries. Every capture verifies the requested budget against actual
MemoryManager microseconds. There is no forced GC or heap reset between
passes, so live GC state carries across budgets.

All active windows use the same single swap chain (`0x000000002D650980`)
and Composed: Flip presentation. Both display endpoints must lie inside
the input window. Active-window/motion checks pass; caption/model/snapshot
counters are zero. No build, test, census, WPR, offline analysis or UIA
observer overlaps these clean captures. All runs, including the prespecified
warm-up, are retained.

| Run | Budget ms | Block 0 p99 / max ms | >9.375 ms | Block 1 p99 / max ms | >9.375 ms |
| --- | ---: | ---: | ---: | ---: | ---: |
| warmup | 5 | 6.3750 / 18.7481 | 4 | 6.3031 / 12.4976 | 1 |
| a1 | 5 | 6.3405 / 12.4936 | 1 | 6.2940 / 12.5057 | 1 |
| b1 | 2 | 6.2879 / 12.4844 | 1 | 6.3141 / 12.4860 | 1 |
| b2 | 2 | 6.3402 / 12.4749 | 1 | 6.2801 / 6.3957 | 0 |
| a2 | 5 | 6.3527 / 12.3187 | 1 | 6.2880 / 12.4759 | 1 |
| b3 | 2 | 6.3268 / 12.5311 | 1 | 6.4424 / 12.4997 | 3 |
| a3 | 5 | 6.3355 / 12.5616 | 1 | 6.3242 / 12.4920 | 1 |
| a4 | 5 | 6.3344 / 12.3247 | 1 | 6.3041 / 12.4697 | 1 |
| b4 | 2 | 6.3540 / 12.4898 | 1 | 6.3229 / 12.4976 | 1 |

Excluding the prespecified warm-up, 5 ms has 8 long displayed intervals and 2 ms has 9,
over 32 seconds of input per budget. This does not justify changing the
production default. A shorter slice alone has not removed the misses.

## Concrete next target

ExternalCatalogModel::suspendPreviewReads(false) currently emits
`ImageFullSizeRole` for every child row solely to re-publish read readiness.
The method itself changes read authority, not dimensions. The size role
enters Masonry model-size refresh/layout handling; it also queries the
diagnostic row-delay environment variable inside row processing.

There is already an image-URL-only delta path that plans viewport thumbnail
requests without refreshing dimensions. Next verify that read resumption
can use this readiness/request path instead of advertising a size change.
First add coverage for unchanged metadata, resumed cache misses and resized
retained previews; then measure publication/model-delta work and scrolling.
Also measure construction-time rewraps before changing lifecycle batching.
Do not assume every publication or environment read is redundant.

The normal portable worktree app is restored, with host/child hash agreement
and empty startup stderr verified by the archive. No production source, SDK,
payload or default setting changed in this experiment. No commit/push.
Evidence and reproduction tooling are archived with SHA-256 hashes under
`profiler/experiments/post-url-gc-budget-and-warm-stalls-2026-10-19/`.
Executables, libraries, raw traces and CSV exports are excluded.

AI assistance has been used to create this output.
