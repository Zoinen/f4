# Single atlas upload: pixels verified, scrolling benefit not repeatable

The candidate combines the atlas image and its four padding edges into one
upload. It demonstrably reduces five atlas updates to one per image, but its
scrolling benefit does not reproduce in reversed temporal order. It remains
diagnostic-only; no production source, library archive or payload is changed.
Constant 160 Hz without spikes is not achieved.

## Baseline and implementation

The cached Qt 6.11.1 atlas source was rebuilt using the current Quick compile
options/PCH and generated private-header MOC. All 107 executable sections,
19,894 machine-code bytes, match the atlas object in the production Quick
archive. This is executable-section parity, not whole-object or normalized
relocation equivalence.

The source patch reverses exactly. One diagnostic executable contains both
paths, selected once through F4_PROFILE_SINGLE_ATLAS_UPLOAD. Both arms retain
the production 4096 x 4096 atlas and eligibility threshold. The candidate
copies formatted 32-bit pixels into a two-pixel-larger image, duplicates edge
pixels and corners, then submits that owned image as one QRhi upload. Existing
conversion, coordinates and post-upload image release remain shared. Allocation
failure falls back to the original path. No per-frame rounding is added.

This trades one CPU image copy for fewer driver update commands. It does not
change texture allocation, geometry/uniform-buffer mapping or submission policy.

## Correctness checks

- Focused tests: 12 passed on D3D11 and 12 on OpenGL, zero failures/skips.
  Padding bytes match clamped source pixels for RGB32, ARGB32-premultiplied and
  RGBA8888-premultiplied, including nonstandard source stride, one-pixel axes,
  transparency and non-square images.
- Filtered atlas rendering matches standalone textures within two channel
  values; actual content replacement and atlas recreation are checked.
- Raster leaves, neighbor image and an eight-logical-pixel native caption
  have named scene-space origin and unit-transform checks at actual DPR 1.75.
  The saved control/candidate render is pixel-identical. The small caption was
  visually inspected in the rendered capture.
- With packing disabled, the focused suite passes 11 cases and fails precisely
  the explicit assertion that the single-entry path was used. With packing
  enabled all 12 pass. This distinguishes the intervention from an unchanged
  control rather than treating ordinary rendering as proof of one upload.
- Native rendering suite: 44 passed on D3D11 and 44 on OpenGL, no skips/failures.
- Diagnostic host: four import-isolation/compiled-host cases pass; system-DLL
  audit passes.

The test fixture initially attempted QML Text without a static QML plugin in
the small test executable. It was changed to instantiate the actual native
QQuickText class directly, then rebuilt and rerun. The host's independent
import-isolation check remains required and passed.

## Cold-scroll comparison

Order is control A, packed B, packed B2, control A2. Each arm is a fresh process
with its own empty application cache, the actual Downloads folder (190 entries),
density 152, 2194 x 1186 logical window, 160 Hz and DPR 1.75. Each performs two
ten-second rapid-wheel blocks. The OS disk cache and Go metadata are not flushed.
No other applications are disabled. No builds/tests, UI Automation observation
or offline analysis run during gestures. Focus and recorder-capacity checks pass.

| Arm / Qt PID | Block | Display gaps >9.375 ms | p99 interval | Maximum interval | Coverage holds |
| --- | ---: | ---: | ---: | ---: | ---: |
| A control / 118508 | 0 | 12 | 7.8878 ms | 37.5538 ms | 0 |
| A control / 118508 | 1 | 3 | 6.3393 ms | 12.4936 ms | 0 |
| B packed / 46076 | 0 | 8 | 6.4819 ms | 18.6952 ms | 3 |
| B packed / 46076 | 1 | 5 | 6.3796 ms | 12.5217 ms | 0 |
| B2 packed / 1192 | 0 | 12 | 7.2064 ms | 18.7581 ms | 0 |
| B2 packed / 1192 | 1 | 4 | 6.3629 ms | 18.8247 ms | 0 |
| A2 control / 57156 | 0 | 9 | 6.6566 ms | 25.1023 ms | 0 |
| A2 control / 57156 | 1 | 5 | 6.3597 ms | 12.5396 ms | 0 |

Every block still exceeds the unchanged two-physical-pixel motion ceiling.
These diagnostic analyzer invocations publish all blocks; they are not claimed
as strict motion passes. Different preparation completion timings produce
different upload/image counts, so totals must not be compared as identical
fixed workloads.

| Arm | Atlas images | Upload entries | Packed images | Total packing wall time | Mean per image |
| --- | ---: | ---: | ---: | ---: | ---: |
| A | 545 | 2725 | 0 | 0 | n/a |
| B | 551 | 551 | 551 | 27.1869 ms | 49.3 us |
| B2 | 548 | 548 | 548 | 23.3142 ms | 42.5 us |
| A2 | 512 | 2560 | 0 | 0 | n/a |

The reduction in driver update count is verified, but reliable frame-time
improvement is not. B has a 6.1861-ms cold atlas upload; its next block includes
a 6.8652-ms atlas upload and a 7.1870-ms standalone texture allocation. Dynamic
buffer maps still take several milliseconds. These runs lack scheduler traces:
their API wall times must not be labeled blocked time using a different run.

## Decision and next discrimination

Do not promote the candidate based on its first pair or fewer commands alone.
Keep the tested implementation as an experiment. Smaller atlas pages and this
combined-upload path both leave allocation/map/update stalls, so the next probe
should distinguish broader device/resource contention from atlas-specific work.
Record device/context identity and/or vary existing decode concurrency while
retaining the same rendered contents; do not silently disable supported media.
The current GalleryRuntimeOptions.maxDecodeThreads default is four, an existing
bounded control suitable for an isolated diagnostic experiment. No such change
was made in this step.

Only the exact atlas diagnostic launcher tree was closed after the captures.
Normal portable worktree launcher 43936 and Qt 119460 remain running with the
previously verified embedded build. No commit or push was made.

Evidence: `profiler/experiments/masonry-single-atlas-upload-2026-10-27/`, with
source/compile/link receipts, baseline compatibility, test logs and rendered
images, capture packets, PresentMon CSVs, analyses and SHA256 manifest. Binary
build products remain in artifacts and are represented by external hashes.

> AI assistance has been used to create this output.
