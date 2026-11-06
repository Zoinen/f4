# Bounded video resource reuse: correct, but scrolling spikes remain

The prototype removes repeated conversion-resource creation, preserves tested
pixels, and releases cached resources with their graphics device. The first
live pair still spikes, so it is not promoted to production. Allocation records
now point to the main scene renderer as the larger remaining source of churn.
Constant 160 Hz remains unachieved.

## Scope and baseline

This experiment began October 27 and completed after midnight October 28;
artifact identifiers retain `20261027`. The unmodified Qt 6.11.1 converter was
obtained from the [tagged Qt source](https://raw.githubusercontent.com/qt/qtmultimedia/v6.11.1/src/multimedia/video/qvideoframeconverter.cpp).
Rebuilding it produced byte-identical executable sections to its packaged
SDK archive member: 84 sections, 21,772 bytes. This checks executable section
bytes, not whole-object identity or normalized relocations. SDK libraries were
not replaced.

The diagnostic retains one buffer/sampler set and one current-size output
target per conversion RHI. Size changes replace the target; a device cleanup
callback releases retained resources. Frame shaders, color calculations,
orientation, readback, input textures and rendering behavior remain unchanged.
Both arms use the same executable, default four decode workers, hardware
decoding and all media types. The preceding single-thread lane is used only by
the correctness fixture, not by either live arm.

## Correctness and lifecycle checks

The fixture passes **301 checks, zero failures/skips**. Candidate frames have
independent backing buffers, so the reference's cached image cannot satisfy the
candidate. It checks exact pixel identity across NV12, planar YUV and alpha
BGRA; color matrix/range combinations; varying chroma, crop, rotation,
mirroring and seven additional metadata cases. Target-size changes preserve
pixels. RHI records prove GPU readback on the conversion thread. Cancellation,
shutdown and invalid-frame behavior also pass.

The fixture recorded 299 cached conversions, 298 cache hits, one buffer/sampler
set, seven target creations and one device-cache cleanup at thread exit.
Linking the same fixture against the unmodified converter gives the expected
299 passes / two failures: cache-hit and cache-destruction assertions fail.
Its pixel checks pass. This negative control proves the new allocation checks
distinguish the old path. Synthetic frames do not establish correctness for
every hardware mapping or platform.

The diagnostic host also passes four import-isolation/compiled-host checks and
the Windows system-DLL audit. No static UI geometry changed. These tests do
not establish complete portable rollout; no embedded payload was replaced.

## Live pair

Actual Downloads folder, Masonry density 152, actual 160 Hz and DPR 1.75. Each
arm has its own fresh, initially empty application cache and two ten-second
rapid-wheel blocks. Both pass unchanged active-window, input-count,
record-capacity and target-PID presentation checks. OS file cache and Go metadata
were not flushed. No builds, tests, offline analysis or UI Automation observer
ran during gestures. The existing normal portable worktree app remained alive.

| Arm / Qt PID | Block | Display gaps >9.375 ms | Maximum interval | Coverage holds |
| --- | ---: | ---: | ---: | ---: |
| Reuse / 108724 | 0 | 13 | 18.6921 ms | 0 |
| Reuse / 108724 | 1 | 4 | 12.5830 ms | 0 |
| Default / 116288 | 0 | 7 | 18.7099 ms | 0 |
| Default / 116288 | 1 | 0 | 8.0845 ms | 0 |

Reuse recorded 90 conversion spans with 87 hits, three buffer/sampler sets and
11 target creations. The default recorded 64 conversion spans. Last preparation
samples report 209 stored entries versus 192. These are cache-entry snapshots,
not unique-file counts or per-file latency measurements. Different progress
changes foreground upload work; this single ordered pair does not establish
a statistical regression. It does establish that the intervention does not
eliminate the observed spikes.

Largest measured render-thread API calls with reuse include UpdateSubresource
11.9035 ms and Map 9.6288 ms. Default includes CreateTexture2D 11.8247 ms and
UpdateSubresource 10.5501 ms. These are API wall times; this pair has no scheduler
trace to identify blocked time or lock owners. Motion checks still have large
individual errors, so neither arm satisfies the full goal.

## What changes the next action

| Arm | Cold renderer creations | Cold worker creations | Next-block renderer creations | Next-block worker creations |
| --- | ---: | ---: | ---: | ---: |
| Reuse | 1,606 | 176 | 165 | 105 |
| Default | 1,557 | 330 | 99 | 54 |

Counts include observed D3D buffer/texture creations, not every driver-internal
allocation, shader, sampler or pipeline. During cold scrolling, the render
thread accounts for about 90% of recorded creations with reuse, and 83% in the
default control. Both create hundreds of 256-byte dynamic constant buffers
(364 and 377 respectively in block zero). Geometry-buffer sizes also recur.
This is stronger evidence to prioritize scene-renderer allocation churn over
further isolated video-worker optimizations. Counts alone do not attribute all
refresh misses to those creations.

The next experiment should tag renderer buffer creation/recreation and batch
lifetimes during native scene updates, correlate them with the existing RHI
records, and identify avoidable replacement or resizing. Preserve bounded
memory, thumbnail throughput, painter order and pixels; do not hide work by
dropping media or reducing decode concurrency. Validate improvements with
actual presentation and motion gates, then warm wheel/middle-button captures.

Sources, tests, receipts, capture packets and analysis are frozen in the
matching experiment directory, with hashes for large external artifacts.
Diagnostic processes were stopped. The normal launcher hash is unchanged.
No production change, commit or push.

AI assistance has been used to create this output.
