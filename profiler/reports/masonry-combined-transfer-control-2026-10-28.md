# Combined transfer policies leave explicit resource creation as the largest stall

The combined diagnostic passes all three interaction suites and substantially
reduces the observed mapping/upload tails. Whole scrolling still misses
refreshes. Its largest recorded call is a 13.6489 ms allocation of a roughly
2 MiB dynamic vertex buffer. No candidate is promoted, the ordinary portable
application remains unchanged, and constant 160 Hz is not achieved.

## Current-source check

The panel currently uses GalleryThumbnailImage (a QQuickImage subclass) as a
hidden source for GalleryThumbnailShaderLayer's ShaderEffect. The image provider
uses Qt's standard texture factory; the current fragment shader has no
qt_SubRect_source uniform. This retains the known atlas-extraction path.
ViewerResampleEffect separately contains an explicit removedFromAtlas path;
that viewer code must not be mistaken for the panel's shader implementation.

The prior atlas-aware panel shader already had repeated restored-display
preparation/recording regressions. It is not silently added to this experiment.
The current-source inspection establishes that the path remains, not that each
individual slow texture allocation in this capture necessarily comes from it.
Descriptors alone do not establish factory provenance.

## Combination and tests

One executable combines only these previously tested, individually selectable
policies:

- geometry NO_OVERWRITE after completed earlier context work, with conservative
  rejection of queued CPU work, mixed usage and unproven completion;
- up to 64 staging slots/device, a shared 32 MiB texel reservation budget,
  non-waiting slot/map checks and original-path fallback;
- D3D11 uniform capacity retention matching its existing native rounding.

The same executable runs with all three flags disabled for the control. Source
transformations and component receipts are checked; there is no video-decoder,
worker-count, atlas-size, shader, UI-layout or Go-protocol change.

With all three flags enabled, the combined executable family passes:

| Suite | Passes | Scope |
| --- | ---: | --- |
| Geometry safety | 5 | Real vertex/index data, queued reads, unknown completion, constant-buffer exclusion and teardown |
| Staging pixels/lifetime | 56 | Format/region/stride/mip/array bytes, source lifetime, fallback/burst ordering, budget and teardown |
| Native rendering | 46 | Existing 44 native checks plus merged/unmerged uniform-capacity transition and control |

All have zero failures/skips. Native rendering verifies expected actual DPR 1.75
and the requested D3D11 path, rendered pixels and scene origins/unit vectors.
Four import/compiled-host checks and the Windows system-DLL audit also pass.
Counts include each suite's setup/cleanup; they are not 107 independent user
scenarios. This validates the tested interactions, not every backend, native
client or a finished portable rollout. No embedded payload was replaced.

## Capture method

Each capture buffers helper stdout until child completion and uses one bounded
tool wait to return the result after approximately 45 seconds. No intermediate
assistant status or result output is emitted during that interval. The method
sidecars retain start/end times, arguments and successful exit status. This
reduces avoidable tool/chat output during input; it does not prove that the
desktop, compositor or agent UI has zero observer effect.

Actual Downloads, 190 entries, Masonry density 152, actual 160 Hz/DPR 1.75.
Two ten-second rapid-wheel blocks per fresh process, separate initially empty
application caches, default four workers, hardware decoding and all media.
Focus, input-count, record-capacity, target-PID and feature-use checks pass.
No builds/tests, offline analysis or UI Automation observer run during gestures.
OS file cache and Go metadata are not flushed. Normal portable app stays alive.

## One enabled/control pair

| Arm / Qt PID | Cold / next gaps >9.375 ms | Cold / next max display gap | Last stored entries |
| --- | --- | --- | ---: |
| Combined / 102396 | 6 / 1 | 25.0458 / 12.5297 ms | 189 |
| Disabled / 20572 | 9 / 0 | 18.7671 / 6.5410 ms | 201 |

Both have zero coverage holds and still fail the no-spike/motion requirement.
This single ordered pair, with different progress, is not a statistical
improvement/regression result. Stored entries are cache snapshots, not unique
file counts or per-file latency. No comparison to earlier interleaved-output
captures is used to claim that the quiet method improved performance.

The enabled path makes 262,174 NO_OVERWRITE maps and 1,137 unproven DISCARD
maps, with zero failed reuse maps. It stages 3,361 uploads, falls back for
4,688 unsupported and 471 exhausted-slot attempts, records zero map-busy/error
results, and peaks at 18,071,552 tracked staging texel bytes under its budget.

Thresholded maxima wholly inside measurement windows:

| Operation | Combined maximum |
| --- | ---: |
| Admitted geometry NO_OVERWRITE map | 0.1970 ms |
| Remaining direct geometry DISCARD map | 0.1411 ms |
| Staging MapWrite / queued copy | No records at or above 50 us |
| Staging-texture creation | 0.2657 ms |
| Original-path fallback update | 0.6790 ms |

These observations cover those inner operations, not all upload work or a
guarantee that their future calls always meet those bounds. Outer legacy RHI
wrapper names include interception/eligibility work and must not be mistaken
for the original raw API after substitution. The control's ordinary texture
upload still reaches 9.4682 ms. Cold uniform creations fall from 466 to 175;
total observed renderer creations fall from 1,597 to 1,421, including the new
staging resources. Counts are turnover, not peak memory or every driver object.

## Remaining explicit allocations

Exact containing creation/resource records identify:

- CreateBuffer 13.6489 ms: **2,054,448-byte dynamic vertex buffer**, bind flag 1.
- CreateTexture2D 8.3025 ms: **129x281 BGRA default shader-resource texture**.
- CreateTexture2D 7.8004 ms: **154x275 BGRA default shader-resource texture**.

These are not all large-atlas allocations. A further buffer creation reaches
4.7579 ms, and an unchanged constant-buffer host-write Map reaches 3.0683 ms.
API wall times do not separate CPU execution, descheduling, GPU work or lock
ownership; no scheduler trace was collected for this pair.

The next target is allocation lifetime/provenance: distinguish initial buffer
creation, capacity growth and immutable-to-dynamic promotion, and identify
the small standalone texture factories. Do not blindly widen every buffer or
reintroduce the rejected atlas shader. Reducing or preparing those allocations
must preserve bounded memory, painter order, complete media throughput and
the validated map/upload safety rules. Whole-scene presentation still requires
independent cold/warm wheel and middle-button validation before rollout.

Sources, component receipts, combined tests, capture-method sidecars, both
packets and analyses are frozen in the matching experiment directory. Large
binaries have external hash references. Diagnostic processes were stopped;
the normal portable launcher is unchanged and alive. No commit or push.

AI assistance has been used to create this output.
