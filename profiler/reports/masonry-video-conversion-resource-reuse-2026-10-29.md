# Masonry: bounded reuse of video-conversion resources

## Outcome

Constant 160 Hz without spikes remains **unachieved**. A diagnostic cache for
Qt's D3D11 video-to-image conversion preserves exact output and substantially
reduces repeated conversion-resource creation, but the four clean cold-scroll
captures do **not establish a presentation or motion improvement**. Do not
promote this implementation to the main Qt payload on this evidence.

The control captures have 13 display intervals above 9.375 ms in total; the
candidate captures have 14. Every block exceeds the original maximum motion
error of 2 physical pixels. Coverage holds are zero throughout. The candidate's
worst display interval is 18.7541 ms versus 12.5608 ms for the control. This small
sample does not establish a population-level regression either.

No production library, portable payload or running normal application is
replaced. No commit or push is made. This is a separate diagnostic executable.

## Why this experiment

The preceding scheduler trace follows a presentation wait through an NVIDIA
worker blocked in allocation, then to a Qt video-thumbnail thread creating its
conversion RHI context. Decoder-device reuse does not eliminate Qt's separate
conversion devices. Earlier conversion-lane and CPU-conversion experiments did
not establish a benefit.

This experiment keeps the conversion threads and GPU conversion intact. It
tests whether eliminating repeated creation of the converter's buffers,
render-target texture, sampler, render-pass and pipeline helps the live scene.
It does **not** eliminate each worker's initial conversion-RHI creation, input
texture imports, readback staging resources or output image allocations. The
preceding trace's first-use context-creation wait is therefore not claimed fixed.

## Implementation and bounds

The source is Qt Multimedia **v6.11.1**
[qvideoframeconverter.cpp](https://github.com/qt/qtmultimedia/blob/v6.11.1/src/multimedia/video/qvideoframeconverter.cpp),
SHA256 `eb4e27f87ab96f7d6bd4c0fc718d0242934a5c707296c96d6698aa177416c523`.
The downloaded original and reversible replacements are preserved. Reversing
the replacements restores the original source exactly. The installed SDK is
unchanged; a strongly linked diagnostic object replaces that archive member.

`F4_PROFILE_VIDEO_RESOURCE_REUSE=1` enables reuse only for D3D11 conversions
tagged `video-thumbnail-frame`. Other producers/backends keep original behavior.
The registry holds at most four RHI-owned slots, with 64 MiB per-slot and 128 MiB
aggregate estimated retained payload. The estimate counts target pixels plus a
small buffer allowance; it is not a measurement of all driver/native memory.
Oversize, busy and capacity cases bypass caching.

Each slot belongs to its QRhi's owning thread. Its compatibility key includes
source and rotated target dimensions, pixel format, plane count and shader
identities. Uniform bytes and actual input textures are updated for every
frame. A successful compatible conversion reuses the immutable quad, uniform
buffer, target, sampler, render-pass, binding object and pipeline. The binding
object is recreated against each frame's actual textures. Incomplete reads and
device-loss states discard uncertain resources. Cleanup callbacks release
dependent resources before their owning RHI is destroyed.

After successful readback, native shader binding references and input textures
are released **before** committing the retained resources. No completed input
frame is retained. This avoids pinning an entire decoder texture-array pool
through one cached frame. Readback textures remain transient. Resizing or
format/shader changes replace the compatible resource set rather than reusing
stale dimensions or content.

## Correctness and packaging verification

The final implementation passes:

- **304 synthetic GPU/reference and lifetime tests**, zero failures/skips.
  This retains the original pixel/format/transform matrix and adds same-key
  resource reuse, size/format replacement, payload overflow/budget checks,
  four-slot capacity, fallback and owner-thread cleanup checks. The resource
  reuse regression fails with the feature off: six buffer creations where the
  fixture expects four. Its failing output is preserved.
- **14 real-media tests per arm**, zero failures/skips, using all six actual
  Downloads videos. Each arm compares 60 GPU-converted frames exactly against
  the original conversion path: 30 delivered by Qt and 30 explicitly decoded
  into D3D11 frames and imported through QFFmpegVideoBuffer. The explicit
  decoder is destroyed before conversion. Hardware status is checked through
  the actual FFmpeg hardware frame, not inferred from QVideoFrame handle type.
  Pixel differences must be zero. Imported-frame reference counts do not grow
  across conversion, and explicit cleanup leaves zero cached bytes/slots.
- **48 native scene tests**, **27 shader tests**, **3 import-isolation tests**,
  and the Windows dependency audit. Native tests run with actual DPR 1.75.
  There is no new or modified static UI layout in this experiment.

The fixtures also passed with the host's existing initial-dynamic, uniform
capacity/reuse, native-query and first-geometry-map settings. Earlier fixture
failures are preserved: a missing static FFmpeg plugin import, and an incorrect
assumption that QVideoFrame handle type alone proves hardware decoding. The
final fixture fixes those setup/proof errors and adds explicit hardware imports.
It does not relax pixel tolerances. No actual GPU reset/hotplug is tested.

## Clean live comparison

All four cases use one executable:
`artifacts/f4-qt-host-video-converter-resource-reuse-20261029.exe`.
Both earlier diagnostic AV optimizations (private FFmpeg heap and thumbnail
decoder-device reuse) are enabled in **both** arms. The converter-reuse flag is
the sole candidate/control variable; conversion threading is unchanged.

Each case uses actual Downloads, all 191 entries, Masonry density 152,
3840×2160 at 160 Hz, actual DPR 1.75, two ten-second rapid-wheel blocks, and a
fresh isolated gallery cache. The OS file cache is not flushed. Cold scrollbar
setup uses the normal panel viewport path without a warm-tour bypass.
PresentMon measures display intervals; native probes record preparation and
motion. No ETW scheduler trace, build, tests, offline analysis or UI Automation
observer runs during these gestures. Existing display, content, focus/input,
memory/resource, renderer and error guards pass.

Cases use prefix `real-scroll-converter-resources-`, suffix
`-20261029-wheel-masonry`, in A1/B1/B2/A2 order. Cache suffixes are zzj/zzk/zzl/zzm.
The initial B1 attempt stops before launch on the disk guard. Only its successful
retry is counted. Completed older exports are losslessly NTFS-compressed with
identical before/after SHA256 hashes; unique captures are not deleted and the
disk guard is not lowered.

| Arm | Block | Preparation max, ms | Display max, ms | Intervals >9.375 ms | Max motion error, physical px |
| --- | ---: | ---: | ---: | ---: | ---: |
| A1 control | 0 | 5.5579 | 12.4349 | 1 | 4.792 |
| A1 control | 1 | 7.8891 | 12.5558 | 4 | 80.198 |
| B1 reuse | 0 | 9.1126 | 12.4908 | 1 | 53.210 |
| B1 reuse | 1 | 7.9457 | 12.5672 | 4 | 103.154 |
| B2 reuse | 0 | 5.4505 | 18.7541 | 5 | 115.403 |
| B2 reuse | 1 | 9.1239 | 12.4872 | 4 | 80.591 |
| A2 control | 0 | 5.1339 | 12.5608 | 3 | 91.874 |
| A2 control | 1 | 9.0642 | 12.5231 | 5 | 83.291 |

All eight blocks have zero coverage holds. The full percentile distributions
are in `live-comparison.json`; a favorable median is not an acceptance gate.

Process-cumulative conversion-resource counters, including startup:

| Arm | Buffer creates | Target/pipeline creates | Reuse/pipeline hits | Completed cached conversions | Final slots | Peak estimated retained bytes |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| A1 | 192 | 96 | 0 | 0 | 0 | 0 |
| B1 | 32 | 16 | 89 | 105 | 3 | 41,947,584 |
| B2 | 22 | 11 | 63 | 74 | 3 | 41,947,584 |
| A2 | 134 | 67 | 0 | 0 | 0 | 0 |

These counts confirm that the intended reuse executes. They are **not** a
matched conversion count or a pure per-measurement-window rate. B1 retains
10,760,448 bytes at the final snapshot; B2 retains 11,071,488 bytes. Live cleanup
counters remain zero because the snapshot precedes process termination;
deterministic fixtures separately verify cleanup. Render-thread slow geometry
unmaps remain below 0.8 ms in these clean captures, with no evidence that this
candidate removes all driver waits. No scheduler attribution of their missed
frames is claimed.

## Decision and next target

Keep the converter cache as a tested diagnostic experiment. Resource creation
is lower, but the full-fidelity scrolling gate fails and no stable display
benefit is established. Do not add this cache, its retained GPU memory or the
earlier AV diagnostic changes to the portable payload solely on these results.

The CPU trace already points to repeated preprocessing eligibility checks and
vector-gate membership work. A next implementation should target those callers
with regressions for deletion/reparenting during preprocessing, dynamic subtree
blocking, root exclusion, source generations and residency. Repeating the
rejected ancestor hash cache is not justified. Initial conversion-context
creation remains a separate proven first-use contention path, and warm-wheel
motion error remains unresolved. Any next candidate must be measured under
the unchanged strict cold, verified-warm wheel and quarter-height middle gates.

Reproducible sources, receipts, tests, analysis, packets and screenshots are
frozen under `profiler/experiments/masonry-video-conversion-resource-reuse-2026-10-29`.
Large data is losslessly compressed; executable/object/library and raw capture
hashes are recorded externally. The normal worktree application stays alive
with its original binary hash; all owned diagnostic instances are stopped.

AI assistance has been used to create this output.
