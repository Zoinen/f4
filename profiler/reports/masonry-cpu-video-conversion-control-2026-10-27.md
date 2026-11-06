# CPU video conversion: pixel-tested prototype, no scrolling benefit established

An explicit, metadata-aware CPU prototype preserves tested pixels and retains
the original GPU path for unsupported combinations. Its live capture still has
spikes and does not improve on the valid subsequent control. It remains
experimental; production video conversion is unchanged and constant 160 Hz is
not achieved.

## Pixel correctness came first

Qt 6.11.1's private forceCpu conversion path was tested against the original
GPU path before any live intervention. Of 72 pixel rows, 52 exceed the unchanged
three-channel-value tolerance; differences depend on color matrix/range and
alpha format. Including setup, cleanup, invalid-frame and GPU-path checks, that
fixture reports 24 passes / 52 failures. This rules out blindly replacing
QVideoFrame::toImage with the private test-only flag. It is not evidence that
every Qt CPU conversion is wrong for every source.

The GPU reference explicitly proves a real RHI device and staging-texture
readback. It cannot silently fall back to CPU and produce a false match.
The first fixture startup hung in a Qt modal startup error because its small
link stripped static plugin/resource initializer objects. A non-invasive stack
inspection confirmed the modal wait. After terminating only that exact fixture,
its link retained SDK initializers and completed normally. The failed startup
is not treated as a converter result.

A diagnostic converter then used already-linked FFmpeg swscale with explicit
matrix/range, cached per-thread scaler context, correct mapped strides and
full chroma interpolation. It supports NV12/YUV420P with explicit SDR metadata,
except full-range BT.601. It preserves the existing crop/frame orientation.
Unsupported RGB/alpha combinations, unknown color metadata, HDR transfer and
surface-format transforms retain the original frame.toImage path. This is a
supported-fast-path experiment, not a universal CPU converter or final API design.
There is no new Qt private API in this prototype host path.

Final fixture: 299 passes, zero failures/skips:

- 160 pixel rows actually exercise the CPU path, with a counter assertion;
- 128 pixel rows retain the original path for unsupported combinations;
- seven explicit HDR/unknown-metadata/surface-transform cases assert exact
  fallback pixels and no CPU conversion;
- remaining checks cover invalid frames, actual GPU reference readback and
  setup/cleanup.

The matrix covers NV12, planar YUV and alpha BGRA; BT.601/709/2020 metadata and
full/video range; constant/varying chroma; crop; frame rotation and mirroring.
The tested CPU results remain within three channel values. This is not proof
for arbitrary chroma siting, interlaced content, HDR CPU tone mapping, every
codec/hardware-frame mapping or all platform implementations.

An initial swscale attempt had 20 pixel-row failures. Unsupported combinations
were retained on the original path rather than relaxing tolerance. An artifact
builder briefly targeted the preceding recorder fixture object path; that
fixture was rebuilt/restored and its 13 checks rerun successfully. Production
objects and hosts were unaffected. Final receipts pin the corrected inputs.

## Diagnostic host and capture validity

One diagnostic host contains old and candidate conversion paths, selected only
by a child-process profiling flag. It retains four workers, hardware decoding,
all media types, original atlas behavior and the existing owner/producer probe.
The new host passes four import-isolation/compiled-host checks and system-DLL
audit. No production source, CMake dependency contract, library archive, Go
protocol, embedded payload or saved setting changes in this step.

All attempts use the actual Downloads folder, fresh separate application caches,
Masonry density 152, 160 Hz and actual DPR 1.75, with two ten-second rapid-wheel
blocks. OS disk cache and Go metadata are not flushed. No tests/builds, analysis
or UI Automation observer run during gestures. Normal portable worktree app
remains alive.

The first control (Qt PID 112976)
is excluded: it recorded 1,459 wheel events, below the unchanged 1,500 minimum.
It did not lose focus. Its packet is preserved, but no valid comparison or
presentation success is claimed from it. The subsequent valid order is CPU
candidate followed by a fresh default control; these are not randomized repeated
arms and their event counts differ.

| Arm / Qt PID | Block | Display gaps >9.375 ms | p99 interval | Maximum interval | Coverage holds |
| --- | ---: | ---: | ---: | ---: | ---: |
| Candidate / 62564 | 0 | 16 | 7.9604 ms | 18.7632 ms | 0 |
| Candidate / 62564 | 1 | 4 | 6.3397 ms | 18.7418 ms | 0 |
| Valid control / 12944 | 0 | 7 | 6.4949 ms | 23.3102 ms | 0 |
| Valid control / 12944 | 1 | 0 | 6.3056 ms | 6.3987 ms | 0 |

The candidate has 1,737 wheel events versus 1,778 in the control, all active.
At finish it has stored 178 frames versus 203, pending zero in both. These are
store/preparation counters rather than unique visible-file completion counts.
No per-item latency or complete catalog coverage is inferred.

The candidate performs 42 CPU conversions and 23 GPU fallbacks among 65 tagged
conversion calls. Its three RHI device records include the render device and
two offscreen video devices: this intervention does not fully remove offscreen
GPU conversion. CPU mapping of hardware frames can also involve graphics work
outside the QRhi probe. Thus it is not a complete isolation of decoder GPU work.
No scheduler trace accompanies these arms; API wall times are not independently
measured blocked durations.

## Decision and next direction

Do not promote either private forceCpu or the conditional swscale prototype.
The live sample does not establish better scrolling, still misses refreshes,
and retains GPU fallbacks. The diagnostic analyzer outputs are not strict motion
passes. Nor does one temporal comparison prove CPU conversion inherently slower
on every workload.

Investigate reducing repeated GPU conversion-resource creation while preserving
the reference shader/color behavior, or bounding only GPU conversion concurrency
without serializing ordinary image decoding. The prior serialized-heavy-work
control reduced thumbnail preparation materially; do not repeat that broad
tradeoff as a production fix. Ownership remains a more precise target than
another unverified backend or global hardware-decoding switch.

Only the exact CPU-probe launcher tree was closed after analysis. Normal
worktree launcher 43936 / Qt 119460 remain running. No commit or push.

Evidence: `profiler/experiments/masonry-cpu-video-conversion-control-2026-10-27/`.

> AI assistance has been used to create this output.
