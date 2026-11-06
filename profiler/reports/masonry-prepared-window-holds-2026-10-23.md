# Masonry camera holds at the prepared-content boundary

## Result

The camera sometimes reached the edge of materialized rows while rendering
continued at 160 Hz. A symmetric look-ahead window left unused preparation
budget behind the direction of travel. The safety guard correctly prevented
showing unprepared cells, but this paused motion until another GUI snapshot.

The candidate shifts unused trailing metadata budget toward a distant motion
target. It preserves the normal trailing delegate margin for reversals and
does not increase the total metadata-window extent. Resting and short-target
plans remain unchanged. No rounding, camera-clock adjustment, or safety-guard
removal was introduced.

Live captures at DPR 1.75 found zero prepared-content holds in four candidate
rapid-wheel blocks and two quarter-height middle-scroll blocks. Rapid-wheel
captures also passed the existing maximum 2-physical-pixel curve timing gate.
This is a validated candidate, not completion of the full 160 Hz goal or a
portable release rollout.

## Reproduction and attribution

The earlier independent pixel capture delivered consecutive rendered IDs, yet
its camera records contain 11 coverage clamps, up to 90.86 physical pixels.
A PresentMon-only repeat without Desktop Duplication contains another nine,
up to 112.75 pixels. Thus steady frame delivery does not prove steady motion.
These holds must not disappear when curve-pair analysis excludes coverage
clamps.

The bounded boundary probe records the closest unprepared row above/below the
viewport during synchronization. Reason codes are 0: no delegate among the
surface children, 1: delegate has no scene node, 2: facade not ready, 3: wrong
presentation/index. All 18 held frames in the targeted reference capture
were blocked by reason 0. There were no asynchronous-facade or SVG readiness
failures at those recorded blocking rows.

One measured reference snapshot had:

- Camera anchor: 609.759532 logical pixels.
- Viewport extent: 1035.428571 logical pixels.
- First absent row: index 87, top 1916 logical pixels.
- Symmetric preparation endpoint: 1904.045246 logical pixels.
- Resulting safe camera maximum: 880.571457 logical pixels.
- Requested next camera pose: 906.945898 logical pixels.

The first clamp held the camera back 46.16 physical pixels. Subsequent frames
could remain held while the snapshot aged. Earlier clamps had snapshots
6–18 ms old; the boundary probe also captured a longer initial hold. The
algorithmic problem is insufficient *directional* preparation, not a reason
to remove the safety constraint or widen the total working window.

## Change

[GalleryViewportMaterializer.cpp](../../third_party/ZoinGallery/GalleryViewportMaterializer.cpp)
now shifts metadata bounds toward a target beyond the existing leading margin.
The shift is capped by the trailing metadata area outside the normal delegate
margin. Both metadata edges move by the same amount, preserving their span.
Leading synchronous delegate demand remains capped by those bounds.

This conserves geometric window size, not an identical number of files:
variable masonry bands can contain different entry counts. The captured
reference had median/max roots 137/152; candidate rapid captures had 138/154.
This was not implemented by retaining the whole directory or increasing an
unbounded cache.

[GalleryLayoutEngineTest.cpp](../../third_party/ZoinGallery/tests/GalleryLayoutEngineTest.cpp)
adds the measured missing-row regression and checks reverse direction,
unchanged viewport, conserved window extent, and retained trailing margin.
Before the fix it fails with `preparation ends at 1904.045246`; after the fix,
all 31 layout-engine cases pass. The existing short-target checks also pass.

The diagnostic motion analyzer additionally records `coverageHolds` for every
measurement window, independently of curve-pair exclusions. Its strict gate
now requires zero such holds. Four focused metric tests pass, including a
real captured hold, reverse direction, endpoint rest, and prior-window data.

## Live verification

Downloads: 190 entries, density 152, D3D11, 160 Hz, DPR 1.75. Captures had no
concurrent builds, tests, UIA polling or offline analysis. Every recorded wheel
input was active. Candidate cold/warm rapid captures used the same process
123996. Middle scrolling used process 121352 after a deliberate restart with
the quarter-height/reversal diagnostic enabled.

| Capture/block | Coverage holds | Largest hold | Display gap p99 | Display gap max | Curve timing error max |
| --- | ---: | ---: | ---: | ---: | ---: |
| Reference rapid 0 | 9 | 99.86 px | 6.2961 ms | 6.4262 ms | 2.2462 px |
| Reference rapid 1 | 9 | 92.35 px | 6.2942 ms | 6.4354 ms | 3.2262 px |
| Candidate rapid 0 | 0 | 0 | 6.2861 ms | 6.3712 ms | 1.9746 px |
| Candidate rapid 1 | 0 | 0 | 6.3011 ms | 6.3990 ms | 1.7382 px |
| Candidate warm rapid 0 | 0 | 0 | 6.2875 ms | 6.3665 ms | 1.7569 px |
| Candidate warm rapid 1 | 0 | 0 | 6.2866 ms | 6.3649 ms | 1.6862 px |
| Candidate middle 0 | 0 | 0 | 6.2963 ms | 6.5111 ms | not evaluated by curve model |
| Candidate middle 1 | 0 | 0 | 6.2837 ms | 6.3737 ms | not evaluated by curve model |

No capture block had a displayed gap over the unchanged 9.375 ms miss gate.
Wheel counts were reference 1814, candidate 1764, candidate warm 1805, all
above the unchanged 1500 requirement. Caption measurements remained zero.
Middle displacement was 296.5 for an 1186-high window, with eight reversals,
1014 active checks and zero inactive checks.

The reference and first candidate have the same 3905.571429 scroll extent.
The warm candidate's extent is 3609.571429 after cached geometry settles; do
not claim that this is a randomized identical-geometry A/B benchmark. The
middle pass starts at the former extent. Additional order-reversed and longer
runs remain useful before broad rollout.

Import-isolation: four cases passed for both diagnostic hosts. System-DLL
audits passed. The candidate host links the fresh materializer object ahead
of the unchanged baseline core archive; this changes only that translation
unit, with no class-layout change. Receipts identify the precise inputs.
The ordinary full application has not yet been rebuilt or repackaged.

## Measurement corrections and limits

- The marker/camera association additionally checked 2063 same-job input
  retarget pairs: next-frame position was continuous exactly against the prior
  curve. Stable-curve camera step error against consecutive nominal refresh
  ordinals was below 0.001 physical pixels. Those facts do not account for the
  independent coverage clamps above.
- PresentMon returned success without a CSV in combined desktop-observer
  captures. PID filtering did not fix that; an unfiltered combined attempt
  failed too. PresentMon-only unfiltered captures do record F4. The precise
  interaction is unknown. Do not attribute it conclusively to process-name
  filtering. The harness now rejects a missing/empty CSV and verifies the
  recorded target PID. No missing trace is used as smoothness evidence.
- The existing CMake test response line exceeds Windows linker's usable line
  length. Identical arguments were split across lines for a diagnostic relink.
  This test target also lacks the system `usp10.lib` needed by its static
  HarfBuzz archive; the diagnostic relink explicitly includes that host
  dependency. These harness/build limitations were not disguised as app fixes.
- Coverage metrics are independent of the raw presentation timestamp/2-pixel
  gate. Neither gate was relaxed. Endpoint pauses, source loading and cold SVG
  uploads still need separate verification.
- Current baseline includes unpromoted native preview staging work. This
  candidate's success does not by itself resolve its steady-work tradeoff.
- Middle scrolling has displayed-gap and camera-coverage evidence here;
  actual content-displacement/velocity verification is still pending.

## Remaining work

Repeat with reversed run order, verify middle-scroll displacement and startup
behavior, and check larger/heavier directories and cold preparation. Resolve
the existing staging tradeoff before selecting the complete production change
set. Then rebuild the coherent host, run relevant geometry/render/import and
portable embedding/extraction gates, and replace the worktree app. Keep the
full constant-160-Hz goal active until the end state is verified.

Evidence snapshot: `profiler/experiments/masonry-prepared-window-holds-2026-10-23/`,
including source, failing-before/passing-after tests, build receipts, original
capture packets, target/whole-session presentation CSVs, summaries and a SHA256
manifest. Executables and build objects are excluded; their hashes are pinned.

> AI assistance has been used to create this output.
