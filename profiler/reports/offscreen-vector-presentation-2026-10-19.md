# Retained SVG presentation and Masonry scrolling — 2026-10-19

The constant-160-Hz goal remains **unachieved**. This change removes verified
offscreen SVG draw work while preserving prepared scenes. Clean display timing
is effectively tied; it does not establish smoother scrolling.

**Later build audit:** both immutable A/B hosts below contain stale generated
thumbnail QML with `F4_BITMAP_SOURCE` / `F4_BITMAP_DESTROY` handlers, although
those handlers are absent from the recorded source files. Thus "clean" below
means that detailed native probes were disabled; it does not establish that
the intended QML source was fully deployed. The observed operation counts and
display intervals remain evidence for those binaries. Use the fresh-cache
baseline documented in `qml-cache-and-sweep-experiment-2026-10-19.md` for further
optimization; do not treat this report as a verified logging-free baseline.

## Reproduced defect and production change

`GalleryVectorSceneCache.qml` retained prepared offscreen SVG surfaces with
`displayEnabled` left at its default `true`. Masonry deliberately retains a
bounded set of fully clipped scenes to preserve their GPU geometry. Their Qt
visibility therefore does not imply that they are inside the viewport.

The cache now binds `displayEnabled` to a live, visible host whose `inViewport`
is true. Existing hidden preparation, upload priority, source leases and ready
state remain intact. The viewer's direct vector surfaces are unchanged.

The extended `nearbyPreparationSettlesWithRetainedVectorScenes` regression first
failed on the original implementation: a prepared retained surface had
`inViewport == false` but `present == true`. After the fix, each observed ready
retained offscreen surface has presentation disabled. Scrolling one back into
view preserves its source URL, load timestamp, content object identity and native
ready state. The test samples the rendered orange SVG and saves a window capture.

This changes presentation state, not layout arithmetic or pixel-grid policy.
Existing leaf-alignment and rendered-output checks ran at DPR 1.75. The resumed
SVG captures were also inspected on both rendering backends.

## Verification

At 175% on Direct3D 11 and OpenGL, the selected suite passed **87 checks per
backend**: 27 layout-engine, 43 mode, 7 session, 7 interaction and 3 embedded-import
checks. This includes all panel modes, vector resizing, ready bitmap retention,
row commits, pooling, video badges and transitions. This is a selected suite,
not a claim that every existing test passes. The previously documented unrelated
legacy Details logical-coordinate assertion has not been repaired here.

An initial post-fix focused run passed the behavioral and rendered-pixel checks
but failed to save its capture because the requested directory did not exist.
After creating the capture directories, both focused and broader runs passed.
The original failure log is preserved separately from successful verification.

The final host passed the Windows system-DLL dependency audit and embedded-only
QML import checks. The compressed host payload was regenerated; embedded-host
extraction/reuse tests passed; Go was rebuilt with `CGO_ENABLED=0`, `GOWORK=off`
and the system Go cache. Only this worktree's launcher was restarted, directly
with `--gui=qt`. Its extracted child hash matches the freshly built host; normal
stderr was empty. Runtime identities are archived.

## Renderer attribution

Diagnostic builds use bounded native QPC scopes and per-frame aggregates for
renderer preparation, drawing, material updates and uploads. Event arrays were
checked for exhaustion. JSON is serialized after measurement. These probes add
overhead; timings across separate captures are not clean A/B performance proof.
Inclusive nested method times must not be added together.

In two eight-second sampled captures of Downloads (186 entries, density 152,
160-Hz display, DPR 1.75), the median per-frame work changed as follows:

| Operation | Before cold / warm | After cold / warm |
| --- | ---: | ---: |
| Material dynamic updates | 673 / 676 | 370 / 371 |
| Unmerged preparations | 53 / 53 | 8 / 10 |
| Unmerged draws | 53 / 53 | 3 / 10 |
| Merged draws | 323 / 321 | 323 / 321 |

Measured renderer p95 was 2.319 / 2.233 ms before and 3.586 / 2.789 ms after.
Thus reduced operation counts do **not** justify a renderer-time improvement
claim. Prior stage probes also showed that a sampled `prepareAlphaBatches`
symbol alone was misleading: its measured inclusive p95 was about 0.1 ms,
whereas complete preparation and recording took substantially longer.

The remaining gate implementation scans batch ancestry and constructs owner
maps when ready hidden gates exist. A follow-up diagnostic capture isolated its
constructor: cold/warm p50 **0.1715 / 0.1780 ms**, p95 **0.3068 / 0.3184 ms**,
maximum **0.6509 / 0.5582 ms**. Gate finishing had p95 0.0011 ms in both windows.
This is measurable overhead, but does not account for the largest stalls.
No classification-cache or native renderer behavior change was made.

The completed exact-QPC scheduling/GC attribution confirms several remaining
causes rather than one gate hotspot. In the warm diagnostic capture:

- A 22.1419-ms producer interval overlaps 12.5197 ms of GC, with 18.669 ms
  of GUI-thread running time.
- A 13.9236-ms interval has zero GC, 2.231 ms GUI running and 10.3436 ms
  render-thread running.
- A 13.3811-ms interval has zero GC, 9.3416 ms GUI running and 6.6671 ms of
  instrumented native GUI work, including 3.7855 ms nearby work and 2.8273 ms
  property work. These nested durations must not be added indiscriminately.

Scheduler alignment spread was 900 ns; ETW reported no lost events/buffers.
These are diagnostic intervals with probe overhead, not clean display cadence
measurements. The next changes need to address GUI preparation/binding bursts,
GC pauses and residual render CPU work; another blind gate-cache rewrite is not
justified by the measured ownership-scan duration.

## Clean display comparison

Six fresh-launch runs used A1/B1/B2/A2/A3/B3 order, two four-second wheel windows
each. A differs from B only by the cache's `displayEnabled` binding. Immutable
host hashes and embedded raw-QML presence/absence were checked during pair
construction. No detailed native/GC scopes, tests, builds, offline trace
analysis or UI Automation observer ran during clean captures. The minimal
measurement controller and PresentMon were common to both versions.

Every run verified the actual display/DPR, folder, density, active-window input,
one swapchain, Composed Flip, zero model/snapshot changes, zero caption
measurements, 5-ms GC budget and reversing wheel motion. No frame recorder was
enabled. A long interval below means a displayed interval over 9.375 ms.

| Run | Version | Long intervals | Maximum display interval (ms) | Window p99 (ms) |
| --- | --- | ---: | ---: | --- |
| A1 | Before | 4 | 12.5058 | 6.3424 / 6.2769 |
| B1 | After | 5 | 18.7334 | 6.2856 / 6.3268 |
| B2 | After | 2 | 12.5008 | 6.2874 / 6.2770 |
| A2 | Before | 4 | 18.7562 | 6.3464 / 6.2953 |
| A3 | Before | 4 | 12.5724 | 6.3539 / 6.3083 |
| B3 | After | 4 | 12.5148 | 6.3231 / 6.3076 |

Totals: A **12**, B **11**, over 24 seconds per version. These sparse events
do not establish a reliable cadence improvement. Both versions still miss the
6.25-ms display budget. Garbage-collection stalls and other GUI/render-thread
work remain unresolved; the objective must stay active.

## Evidence

The companion directory
`profiler/experiments/offscreen-vector-presentation-2026-10-19/` contains the
failing and passing tests, resumed-vector captures, clean PresentMon rows and
motion summaries, derived native timing/call-count findings, bounded-probe and
build/analysis helper sources, portable audit, host hashes, runtime identity and
a SHA-256 manifest. Large sampled JSON records are archived without bulky
per-event arrays; derived attribution and aggregate findings are preserved.
Executables, ETL traces and native build products remain outside this archive.

No commit or push was made.
