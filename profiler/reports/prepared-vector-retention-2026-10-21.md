# Prepared vector scenes during native Masonry scrolling

Date: 2026-10-21. Workspace: `D:/Code/f4-zoin`, branch `zoin_branch/cached-pixel-grid`.
The objective remains constant 160 Hz without spikes. This change removes a verified
source of redundant work; the objective is **not achieved**.

## Mechanism and implementation

The GUI layout already selects a bounded set of recently visible SVG delegates
(up to 16 offscreen vectors). `GallerySceneItem::updatePaintNode` honors that
retention, but the render-thread camera previously overrode it whenever a root
left its viewport. Qt then removed and later recreated the vector's GPU batches.
There is also a handoff interval before GUI synchronization marks an outgoing
visible vector as retained.

`GallerySceneItem` now carries GUI retention and per-camera prepared retention
separately. `GalleryScrollAnimation::postSync` grants the latter only to a vector
with a published source that is visible or already selected for retention.
Each synchronization replaces that decision. Clearing the owning camera clears
its prepared retention and restores ordinary GUI visibility. Existing three-argument
callers retain their previous behavior. No cache bound, materialization margin,
source priority, animation clock, pixel snapping, or viewport dimensions changed.
The existing parent clip and vector upload gate still control drawing/input.
The implementation has no diagnostic flags or recording arrays.

## Residency experiments

All input runs use native 160 Hz, actual DPR 1.75, Downloads, 187 entries, density
152, and accepted focus/input/buffer validation. Middle input is one quarter of
window height below its origin. Rapid wheel input repeats at 10 ms.

Increasing the QObject vector cache idle limit from 16 to 64 made no difference:
nine loaded surfaces remained alive, with no destruction after initial loading,
while 206 curve uploads / 55,744,272 bytes recurred in each ten-second middle-scroll
block. The limit was not binding. No production cache-limit change was made.
A GUI-retention-only camera fix was insufficient; the prepared handoff is needed.

With both retention paths, middle scrolling uploaded 40 curve batches / 11,102,544
bytes in the first block and **zero curve geometry in all three subsequent
blocks**. This result reproduced with the enabled/control run order reversed.
Control remained at 206 batches / 55,744,272 bytes per block in both orders.
Counts include every upload route, not merely pending vector admission.

The corrected rapid-wheel observer measured:

| Arm / stage | Block | Curve uploads | Bytes |
| --- | ---: | ---: | ---: |
| Enabled cold | 0 | 56 | 14,437,440 |
| Enabled cold | 1 | 0 | 0 |
| Enabled warm | 0 / 1 | 0 / 0 | 0 / 0 |
| Control cold | 0 | 869 | 234,406,224 |
| Control cold | 1 | 860 | 232,396,992 |
| Control warm | 0 | 874 | 234,465,552 |
| Control warm | 1 | 867 | 233,675,568 |

In this rapid pair, warm render-start-to-frame-end p99 was 3.166 / 2.795 ms
enabled and 3.198 / 3.347 ms control. Cold enabled tails were worse than control
(3.999 / 3.405 versus 3.314 / 2.960 ms). These observer-heavy wall-clock results
are not evidence of a universal tail-time improvement. Hidden resident scenes
still have traversal cost. The causal upload elimination is reproducible.

No displayed interval exceeded 9.375 ms in that pair. Nevertheless, the unchanged
strict two-physical-pixel motion gate **failed in every block of both arms**.
Enabled maximum errors were 12.923 / 41.485 px cold and 2.274 / 44.413 px warm.
Control maxima were 2.660 / 31.171 and 42.881 / 35.167 px. The animation timeline
continued at 6.25 ms while some displayed changes were approximately 4.06 or
8.33 ms apart. Correlation does not establish the reason for that display-timing
variation. Do not change the clock or relax the threshold on this evidence.

## Observer crash and excluded captures

The first rapid attempt overflowed the 131072-entry upload recorder and was
rejected. A 524288-entry recorder completed control, but an enabled warm run
crashed during settling and was also rejected.

The debugger reproduced the crash. A byte-identical `.text` / `.pdata` symbol-map
build identified `QSGGeometryNode::activeMaterial()` called from the invalidation
observer in `Renderer::invalidateBatchAndOverlappingRenderOrders`; `rcx` was zero.
Qt legitimately retains a removed first batch element with a null node until
purging. The observer, unlike the actual invalidator, dereferenced that node.

A focused native regression reproduced access violation `0xc0000005` before the
observer fix. Afterwards eight tests passed, including removed/missing nodes,
removed/render-node pointers that must never be read, no material, and a live
material. The corrected observer lives only in diagnostic artifacts. The final
rapid enabled and control captures both completed cold and warm, with bounded
recorders and no crash. Earlier incomplete captures are not substituted for
those accepted captures.

## Regression and build verification

Before the implementation, the native retention fixture had three expected
failures (GUI-retained, prepared-only, and both) and three passing cases including
initialization/cleanup. The integrated regression directly checks native batch
VBO/IBO residency, clipping outside a 160-logical-pixel viewport, release of
prepared-only retention, and visible re-entry. It enumerates the actual visual
leaf's scene origin and unit vectors at DPR 1.75.

Final integrated results: six passed, zero failed/skipped with Direct3D 11;
six passed, zero failed/skipped with OpenGL. Four rendered fixture images matched
byte-for-byte in RGBA across the backends and were visually inspected. The fixture
uses a solid native rectangle to test the scene boundary; SVG-upload behavior is
covered by the live captures above, not inferred from that rectangle alone.

Four import-isolation / compiled-host tests passed. The static Windows DLL audit
passed. The embedded payload was regenerated and its decompressed hash checked
against the new host. CGO-disabled Go build and embedded extraction/reuse/concurrency/
corruption tests passed with the system Go cache.

An initial MSVC link failed on a generated response line longer than 32767
characters. The existing large view-surface test was relinked with the identical
arguments, one per response-file line. The new native fixture needed its QML
import scan disabled (it contains no QML) and Windows HarfBuzz system libraries;
its final normal CMake build passed. Its final scale environment disables native
DPI multiplication so the asserted actual window DPR is 1.75, rather than 3.5 on
a 200% monitor.

Previously recorded full UI suite failures remain documented in the QML signal
production report. This focused validation does not claim those suites are green.

## Evidence and remaining work

`profiler/experiments/prepared-vector-retention-2026-10-21/manifest.json` indexes
snapshotted sources and records hashes for local raw captures. Diagnostic crash
dumps, binaries, objects, and raw performance traces are not intended for commits.
The original production source files were saved before applying the fix.

Important artifact prefixes:
- `real-scroll-prepared-retention-{control,enabled}-20261021`
- `real-scroll-prepared-retention-reversed-{enabled,control}-20261021`
- `real-scroll-prepared-retention-safe-{enabled,control}-20261021`
- `prepared-retention-safe-{input-phases,wheel-motion,motion-attribution}.json`
- `prepared-retention-safe-and-reversed-uploads.json`
- `prepared-retention-final-{d3d,opengl,imports}.txt`
- `prepared-retention-portable-build-receipt.json`

The worktree app was replaced only after verification, using its own embedded
launcher and content-addressed host. A fresh production-library diagnostic with
minimal instrumentation was captured as described below. Display timing and motion-error
attribution remain open. No commit or push was performed.


## Fresh integrated-code capture

A new diagnostic was rebuilt from the integrated production sources and verified
production libraries. Only the bounded native-motion and phase probes remain;
the renderer upload, invalidation, preparation-stage, and cache census arrays are absent.
Its QML import-isolation tests and static DLL audit passed.

| Input / stage | Block | Preparation p99 ms | Render-to-end p99 / max ms | Display max ms | Motion error max px |
| --- | ---: | ---: | --- | ---: | ---: |
| rapid / cold | 0 | 2.464 | 3.493 / 7.873 | 6.449 | 2.529 |
| rapid / cold | 1 | 2.018 | 2.979 / 4.747 | 6.450 | 1.622 |
| rapid / warm | 0 | 2.036 | 2.789 / 4.601 | 6.390 | 2.056 |
| rapid / warm | 1 | 2.078 | 2.959 / 5.885 | 6.575 | 2.933 |
| middle / cold | 0 | 1.880 | 2.474 / 3.379 | 9.149 | 8.280 |
| middle / cold | 1 | 1.526 | 2.032 / 4.016 | 9.173 | 8.429 |
| middle / warm | 0 | 1.567 | 2.285 / 3.773 | 6.423 | 0.494 |
| middle / warm | 1 | 1.531 | 2.167 / 4.126 | 9.246 | 8.533 |

Every block had zero displayed intervals above 9.375 ms. All four middle blocks
had no coverage stalls. **The unchanged two-pixel motion limit still fails**;
these results do not prove the full 160 Hz objective. In middle scrolling, some
displayed intervals alternate around 3.35 and 9.15 ms while rendering stays
under 4.13 ms. This is a reason to investigate presentation timing and validate
its measurement, not evidence for changing the animation clock.

Two earlier minimal middle captures lost focus and were rejected. The accepted
repeat additionally recorded bounded foreground-change events using an out-of-process
WinEvent hook (no UI Automation polling). Its focus checks all passed.

Deployment identity: launcher SHA-256
`26eb402bce3628bbc34a80ad62932eec3ca1e7244c09c076dbb3e1b0e47720ef`,
embedded host SHA-256
`c0133127a7060938dc42e0b7906eec2b6cb316855e865027539a7d088a79ffac`.
At deployment verification the launcher PID was 92184 and Qt PID 103492.
`prepared-retention-production-live-identity.json` records the extracted cache path.

Minimal evidence prefixes: `real-scroll-retention-minimal-rapid-20261021` and
`real-scroll-retention-minimal-middle-focus-20261021`. Analysis snapshots are
`retention-minimal-{rapid,middle}-{phases,motion}.json`.

AI assistance has been used to create this output.
