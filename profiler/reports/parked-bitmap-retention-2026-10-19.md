# Parked bitmap retention — 2026-10-19

## Result

Masonry and Grid shader thumbnails keep their ready bitmap source while their
presentation is parked or guarded during a coherent identity commit. Requested
visibility still hides those pixels. A valid replacement is published directly
after the commit, and disabling thumbnails, switching to vector content or a
non-media entry clears retention. Compact thumbnail rendering is unchanged.

The verified portable worktree build is running. The measured same-image
clear/reload pattern disappears, with a bounded bitmap memory tradeoff. A clean
trial has 14 baseline versus 11 candidate long display intervals; this does not
prove constant 160 Hz or reliable cadence improvement. Spikes remain. No commit
or push was made.

## Establishing the cause

The initial diagnostic adds sourceChanged logging to the actual QQuickImage in
both thumbnail renderer types. The existing native message handler timestamps
events with QPC nanoseconds; explicit measurement windows select scrolling.
A second immutable diagnostic host adds Component.onDestruction markers so
pointer reuse cannot be mistaken for a surviving image object. Keys normalize
the native pointer independently of the temporary QML class/name decoration.

The lifecycle-aware baseline has 93 clear-then-set pairs inside the two input
windows: 80 replace a different source/entry and 13 return to the identical URL
and entry on the same surviving object. The earlier source-only capture finds
7 such pairs but is not used as lifecycle proof. Sampled clears happen at
viewIndex=-1, rather than solely during the brief identity guard.

The census is instrumented semantic evidence, not a frame-cadence comparison.
Per-source logging performs IO and may perturb timing. It does not directly
count file reads, decodes or GPU allocations. Stable source/Ready-state tests
and removal of source reassignment establish avoided source churn; do not label
the notification count as an exact byte-read or GPU-upload count.

## Implementation and checks

GalleryThumbnailShaderLayer keeps retainedImageSource as explicit state.
Non-empty source changes publish the requested URL. Temporary empty requests
preserve it while bitmap retention is allowed. Preference, vector and non-media
changes clear it. Drawing remains controlled by the requested source and the
existing parent/transition culling gates; retaining pixels does not grant drawing
visibility. The existing pool/delegate ownership, prefetch, source revisions,
pixel-grid bindings, GC budget and native Qt libraries are unchanged.

An initial Binding-based prototype failed: its empty value could apply before
its when condition disabled the binding. Its source and failing test output are
archived; it is replaced by explicit source state. No Binding object is added
per thumbnail. The new parkedBitmapKeepsReadySource regression first fails on
the original code in Masonry and Grid: actual source becomes empty instead of
the loaded URL. It then verifies:

- Parking and returning the same slot preserves Image.Ready and emits no source
  notification.
- Disabling thumbnails clears the source; re-enabling can legitimately recommit
  a model-visible tile. A slot remaining parked stays empty after that disable.
- A guarded replacement does not alter the displayed source before commit, then
  publishes one non-empty change. Captured centre pixels are blue, not the old
  red image. Actual shader scene origins and unit transforms pass at DPR 1.75.
- An unreadable replacement reaches Error with zero implicit width, and a new
  valid identity/source recovers to Ready.

A test-only initial compile error (itemForIndex returns QQuickItem) is corrected
with the native delegate cast. The first preference assertion incorrectly assumed
the model-visible row remained unassigned after a preference refresh; its failed
output is retained and the assertion now follows actual assignment state.

Selected suites pass on each of Direct3D11/OpenGL at actual 175% scale: 27 layout
engine, 43 modes, 7 session, 3 embedded imports and 7 interaction/video QtTest
totals (including setup/cleanup). Existing checks cover all panel modes, SVG
retention, thumbnail replacement, failed/non-video badge readiness, transitions,
actual static leaf origins/unit transforms and rendered captures. The final
failure/recovery extension and embedded imports pass again after logging removal
on both backends. The full legacy suite is not claimed green.

## Semantic census and memory tradeoff

Both lifecycle captures use Downloads 186 entries, density 152, actual 160 Hz,
DPR 1.75, two four-second boundary-bounce wheel windows at 30 ms input and the
unchanged 5 ms GC budget. No tests/builds/offline analysis overlap capture.

| Count inside input windows | Baseline | Retention |
| --- | ---: | ---: |
| Source notifications | 374 | 251 |
| Non-empty source assignments | 200 | 182 |
| Empty assignments | 174 | 69 |
| Clear-then-set pairs on surviving objects | 93 | 0 |
| Same-URL clear/reload pairs | 13 | 0 |
| Image destructions | 72 | 71 |

The remaining empty assignments are not followed by source restoration on the
same surviving object within the selected windows. Different-source assignment
still occurs directly. These are separate captures with small lifecycle/motion
variation, not a deterministic per-entry paired benchmark.

At the ends of the two windows, non-empty source counts are 88/103 baseline and
107/113 candidate across the retained panels. Summing encoded requested thumbnail
width * height * 4 gives 19,760,056 / 17,693,832 bytes baseline and
22,866,004 / 23,654,780 bytes candidate: approximately 3.1 / 6.0 MB additional
requested RGBA payload. This is a source-based estimate, not measured RSS, decoded
QImage residency or GPU memory; it excludes format/allocator/driver overhead.

The free delegate pool already trims to min(64, liveCount), and mode commits
trim anonymous spares to zero. Retention adds no pool entries or unbounded cache.
Existing recent-row/vector limits remain. The extra ready pixels in those
bounded spares are the deliberate memory tradeoff for avoiding repeated loads.

## Clean display comparison

A is the immutable coherent-row host; B adds bitmap retention with all logging
absent. Fresh processes run A1/B1/B2/A2/A3/B3, each with two four-second input
windows and the workload above. QML profiling, frame recording, census, WPR and
UI Automation are disabled. All have active/validated input motion, one Composed:
Flip swapchain, zero caption measurements and zero top-level model/snapshot
notifications. Both display endpoints must lie inside an input window.

| Run | Display gaps >9.375 ms | Maximum gap, ms | Block p99, ms |
| --- | ---: | ---: | --- |
| A1 | 3 | 12.5070 | 6.3515, 6.2870 |
| B1 | 4 | 12.5704 | 6.3428, 6.3348 |
| B2 | 4 | 12.5095 | 6.3522, 6.3252 |
| A2 | 7 | 12.5126 | 6.3652, 6.3296 |
| A3 | 4 | 12.5138 | 6.3225, 6.2747 |
| B3 | 3 | 12.5012 | 6.3400, 6.3178 |

Baseline totals 14 long gaps over 24 seconds, candidate 11. The small difference
does not establish reliable cadence improvement; B1 worsens the first pair and
refresh misses persist. Retain this change for the verified eliminated source
churn and correctness coverage, without claiming the full 160 Hz goal achieved.

## Rollout and remaining work

Diagnostic source logging is removed byte-for-byte back to the tested clean
implementation. The normal static host is rebuilt, system-DLL imports audited,
and embedded-only QML imports rerun. The payload is regenerated and extraction
tests/Go launcher build pass with CGO_ENABLED=0, GOWORK=off and system GOCACHE.
The worktree launcher starts directly with --gui=qt; staged-launcher equality,
extracted-host equality and empty stderr are verified. Canonical F4 is untouched.

Atomic GC and render-thread resource/scene work remain independently measured
constraints from earlier traces. Next obtain current-build attribution for the
remaining clustered cold misses, including the whole native preparation commit
and renderer work; do not assume eliminated source clears explain every miss.
Keep real VFS source acquisition, live viewport priority and successful-only
navigation behavior intact. No GC constant or rounding pass is changed here.

Archive: profiler/experiments/parked-bitmap-retention-2026-10-19. Its SHA-256
manifest covers sources, failed/final checks, replacement captures, native-QPC
semantic logs/analysis, six clean summaries, scripts and runtime/binary hashes.
Raw ETW/PresentMon CSV, executables, libraries, build outputs and generated
payloads are excluded.

AI assistance has been used to create this output.
