# Masonry: repeat cold wheel checks and resolve renderer CPU cost

Cold continuous-wheel scrolling is a reproducible remaining problem. Two
ordinary captures miss many refreshes; a CPU-profiled capture reproduces it and
shows preparation dominated by scheduled CPU work rather than waits. Fully
warmed wheel scrolling meets the refresh-interval check in this run, but still
fails the original 2 px maximum motion-error check. The full goal remains unmet.

## Workload and capture validity

The original scope is retained: actual `C:\Users\xs\Downloads`, all 192 entries,
Masonry density 152, actual 3840×2160 / 160 Hz / DPR 1.75, normal window behavior,
native SVGs and two 30-second continuous-wheel passes. Commands request wheel
ticks every 10 ms with delta 120 and the existing endpoint bounce behavior.
The actual event counts are reported below; timer delivery is not assumed to
equal its requested cadence.

All captures used the same verified word-hash diagnostic host, typed discovery
enabled, snapshot observer disabled, frame latency 2 and unchanged renderer/cache
flags. Each had a fresh isolated cache and stable foreground for both measured
windows. The cold fixture used its normal initial viewport without an all-entry
tour; its normal scrollbar method and unsuppressed persistence were verified.
The warm fixture verified every entry ready or terminal through the existing
normal-visibility tour before measurement. Captured initial windows were viewed:
the Downloads panel and the neighboring CR2 Details panel are present in both.

The sequence was cold B1, warm B1, cold B2, then cold CPU B1. The first three
had no additional kernel/graphics trace. PresentMon and bounded in-host probes
remain, so none is an uninstrumented measurement.

| Capture | Actual wheel events | Refresh misses, pass 0 / 1 | Preparation p99, ms | Preparation max, ms | Maximum motion error, px |
| --- | --- | --- | --- | --- | --- |
| Cold B1 | 5,192 | 85 / 111 | 2.9558 / 3.2308 | 5.9036 / 5.5946 | 133.2834 / 122.7844 |
| Warm B1 | 5,377 | 0 / 0 | 1.6609 / 1.6501 | 2.6915 / 2.9807 | 2.5632 / 3.3536 |
| Cold B2 | 5,160 | 132 / 149 | 3.2577 / 3.5285 | 10.7939 / 10.3884 | 123.3788 / 132.4947 |
| Cold CPU B1 | 5,100 | 123 / 122 | 3.4056 / 3.5320 | 7.0547 / 7.7272 | 123.6299 / 133.8641 |

Every pass has **zero coverage holds**. All four captures fail the unchanged
strict check: display interval ≤9.375 ms, maximum physical curve-step error
≤2 px and no coverage holds. The warm run's largest display intervals are
6.4104 and 6.4702 ms; its motion maximum still rejects it. No pair or file type
was removed to force a pass. The strict analyzer's only adaptation is a
preserved-packet path fallback after hash-verified archival.

The ordinary cold captures' misses were not individually joined to driver
refresh counters in this turn. The preceding [correlated delivery capture](masonry-lean-delivery-stall-2026-10-31.md)
showed both real compositor misses and notification timing distortion. Do not
assign every current over-budget notification to one of those causes without
the corresponding event chain.

## Cold stage breakdown

In cold B1, p99 render-list construction is .5585/.5656 ms, geometry upload
stage 1.0265/1.0191 ms and alpha-material preparation .8455/.8454 ms. The warm
counterparts are approximately .298, .565 and .46–.50 ms. These phase percentiles
are not additive.

The first cold pass has 52 admitted curve-upload calls. The largest copies a
2,054,448-byte curve-stroke vertex buffer and takes 3.2661 ms. This is a concrete
first-use cost, but it is not the complete explanation: the second cold B1 pass
has no admitted curve uploads and still misses 111 refreshes.

Ordinary route-1 uploads remain frequent: about 191,000–198,000 per pass in both
cold and warm cases. Many slow preparation frames also rebuild render-list
state; their counters include subtree unblocking and cannot simply be read as
fresh object-allocation counts. The existing metadata observer's ancestor
lookup is measured separately: approximately 62 ms total per cold pass and
49 ms per warm pass, rather than being mistaken for product work or for the
whole multi-millisecond upload stage.

Loading remains active at the end of cold B1 (three pending entries and 235
stored-frame observations, versus zero pending and 192 in warm B1). These
runtime-wide cache counters do not identify the responsible files or panel;
they do not prove decoder starvation, retries or a particular cache bug.

## Bounded CPU trace

The CPU-only capture retains the cold fixture and records process/thread
lifecycle, loader, switches, dispatcher, sampled profiles, DPCs and interrupts.
Only profile stacks are enabled; there are no performance counters, graphics
event stream, CSwitch stacks or ready-thread stacks. The kernel file has a
1 GiB limit and requires at least 3 GiB free before starting, plus a size-based
decode-space check.

The trace is 542,638,080 bytes, with no lost events/buffers or schema errors.
The stack reader records 46,474 profile records and 48,695 stack records across
the requested threads, with module records retained. Its profile prefix uses
the known fallback format; **378 reference CPU samples were independently
matched against xperf's decoder**, including timestamps, thread and IP.

A map-only relink of this exact host has identical `.text` and `.pdata` bytes.
The relinked executable was never launched. The verified map resolves the
actual host, rather than an older experiment's addresses.

The attribution covers 9,593 complete measured frames, with 7,400 CPU samples
inside preparation and zero unjoined preparation stacks. Thread-state overlap
conserves each frame/phase interval. Overall preparation accounts for:

| State | Total |
| --- | --- |
| Elapsed | 7,785.30 ms |
| Scheduled | 7,776.45 ms |
| Runnable, unscheduled | 5.61 ms |
| Blocked | 3.24 ms |
| Unknown | 0 ms |

Scheduled time includes any interrupt work inside a scheduled interval; this
table does not label every scheduled cycle as a user instruction. It does show
that millisecond waits/descheduling do not explain most preparation time in
this trace. Several worst preparations are fully scheduled, including 7.7272,
7.3884 and 7.0547 ms intervals.

### Where scheduled preparation time goes

| Phase | Scheduled total |
| --- | --- |
| Alpha materials | 2,853.74 ms |
| Geometry uploads | 1,645.59 ms |
| Before renderer body, including scene preprocessing | 1,249.65 ms |
| Render lists | 830.07 ms |
| Batching | 391.64 ms |
| Vector gate setup | 326.64 ms |

These are measured interval overlaps, unlike function samples. The leading
unambiguous self-sample labels include material dynamic-data updates (430),
node traversal (252), graphics-pipeline key construction (234), merged-batch
preparation (227), buffer-data assignment (175), unmap (169) and transform
traversal (163). Shared template bodies with ICF aliases are not assigned to a
specific logical hash owner solely from their first map label. Sampling counts
are statistical evidence of hotspots, not exact function durations.

## Next implementation target

Material preparation is the largest measured phase. One bounded opportunity
is repeated graphics-pipeline descriptor construction: the current Qt key
builder copies implicitly shared render-target/SRB descriptions and hashes
both lists for every lookup. The pipeline cache must continue to compare full
graphics state, shader identity and both descriptor values; raw resource-pointer
identity is insufficient, especially when items are recycled during scrolling.

The next candidate should remove redundant descriptor copies/hashing while
preserving exact key equality, copy-on-write ownership, bounded storage and
fallbacks. Verify resource replacement, layout changes, nested renderers and
native output before measuring the same cold/warm workload. Do not broaden
cache lifetime by assuming an SRB pointer or renderpass pointer remains valid.

This is a prospective target, **not an implemented fix**. It does not replace
the outstanding warm motion issue or the separate compositor-side failures.

## Storage, preservation and final state

An old diagnostic build directory was hash-verified onto C: with its original
path retained through a junction. Three preserved lean-trace files were then
transparently NTFS-compressed; their paths, logical contents and SHA-256 hashes
remain unchanged. The compression receipt records allocation and free-space
changes. No raw trace or unrelated data was deleted.

The [manifest](../experiments/masonry-wheel-cold-cpu-2026-10-31/manifest.json)
preserves sources, all strict failures, phase/CPU results, symbol proofs and raw
artifact receipts. D: packet duplicates were retired only after checking their
C: copies. All owned diagnostic apps/loggers are stopped. Normal F4 Go PID
23488 and Qt PID 84780 remain alive. No normal binary, installed Qt package,
external application/VM state, commit or push was changed.

> AI assistance has been used to create this output.
