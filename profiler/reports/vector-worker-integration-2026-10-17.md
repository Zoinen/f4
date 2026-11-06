# Worker-prepared SVG scenes: integration and remaining frame spikes

## Implementation

[VectorSource.cpp](../../third_party/ZoinGallery/VectorSource.cpp) now uses Qt
6.11.1's own VectorImage QML generator on the existing source workers. It
publishes an immutable scene directory together with the original VFS lease.
Authored raster elements are copied into that directory; SVG geometry remains
curve-rendered and never enters the bitmap thumbnail or Fit/Native frame caches.

The generator is private Qt API and has shared mutable static storage. All
gallery generation is serialized. The waiting queue respects viewer priority
and neighbor ordinals and skips cancelled requests. A currently executing
generator pass is not interruptible; subsequent requests can overtake waiting
thumbnail work. Direct local QML sources use a single background worker.
Do not introduce simultaneous stock VectorImage generation outside this gate
without fixing the shared Qt storage.

VectorSourcePin asynchronously compiles the prepared QML component. The component
retains the source handle independently of the requesting surface until loading
finishes, so cancellation cannot delete files under the compiler. Generation
tokens reject stale completions. The QML surface incubates its component
asynchronously and retains its readiness check for asynchronous Shapes. Existing
viewer double buffering, failure skipping, fit/crop, zoom, rotation, checkerboard
and animation controls remain in the presentation path.

The new embedded-image regression initially caught an invalid `file:asset.png`
URL. Generated assets now use an absolute encoded directory URL, and their
ownership follows the prepared revision.

## Verification

- `artifacts/vector-prepared-complete-175.txt`: **26 passed, zero failed**, with
  actual window DPR explicitly checked as 1.75 in the surface regression.
  Coverage includes local/virtual SVG and SVGZ, malformed documents, changed
  revisions, cancellation, source and compiler lifetime, viewer swaps without
  blank frames, successful-only prefetch, all five panel modes, retained scenes,
  resizing without extra source reads, and animation visibility/authored loops.
- New reference samples compare rendered gradient/alpha and embedded PNG content
  with QSvgRenderer. `vector-prepared-assets-175.png` was visually inspected.
  `vector-surface-175.png` was inspected for the small caption and thin stroke.
- A compact raster preview was measured at physical x = **37.1** after a rendered
  frame. Its missing cached pixel-grid correction was added, matching other
  preview leaves. The regression now measures **37** and checks scene-space
  translation and unit vectors. No ancestor traversal or per-frame alignment
  loop was added. Image readiness alone is not a settled-frame fence; the test
  now explicitly observes a rendered frame before asserting alignment.
- `artifacts/vector-prepared-release-imports.txt`: embedded-only import isolation
  and production-host QML startup pass. Isolation now actually loads a prepared
  GalleryVectorImage using resource-only import paths, including its generated
  scene's transitive imports.
- Windows system-DLL import audit and tagged embedded-host extraction/reuse tests
  pass. The payload and Go launcher are rebuilt with the static portable host.
  The restarted `D:/Code/f4-zoin/f4-zoin.exe --gui=qt` uses the extracted host
  whose SHA256 matches the fresh build:
  `01c11cfa8064d5a272e01e8bf67d5b6a942e18bc52b8e1511180861e2a4fc1c0`.
  Its title shows `zoin_branch/cached-pixel-grid`; startup stderr is empty.

The old swap fixture had 1000 nearly coincident cubic contours. A standalone
stock Qt VectorImage reproducer also failed to display that scene within six
seconds at DPR 1.75 and kept geometry workers running during shutdown:
`artifacts/vector-legacy-blue-result.json`, `vector-legacy-blue-probe.cpp` and
`vector-legacy-blue.svg`. The swap regression now uses 1000 separated curves to
exercise asynchronous preparation and blank-frame prevention. The pathological
intersection-solver case is preserved as a separate unresolved reproducer;
passing the swap test does not establish a geometry-processing bound for it.

The animation fixture still logs Qt's `setPaused() cannot be used when animation
isn't running` warning when toggling visibility after authored playback has
finished. Its initial freeze, visible playback and final repeat-count checks pass.

## Live 160 Hz measurements

Downloads, Masonry, 184 entries, density 152; actual DPR 1.75, screen 160 Hz,
window 2194 × 1186 logical pixels. The diagnostic host uses the same code as the
production build with frame-stage observers. PresentMon records display cadence.
No UI Automation observer or CPU sampling runs during these captures.

Wheel delta 120 every 30 ms, four seconds per direction:
`artifacts/modes-profile/vector-prepared160-wheel-masonry.json` and matching
`-present-summary.json`. Initial scroll extent is 4161.142857 logical pixels.

| Wheel direction | Display median | Display p95 | Display p99 | Maximum display gap |
|---|---:|---:|---:|---:|
| Down | 6.2499 ms | 9.2562 ms | 12.5013 ms | 18.7983 ms |
| Up | 6.2500 ms | 7.1825 ms | 9.2783 ms | 12.4993 ms |

Native frame production: median 6.2458 ms, p95 7.8022 ms, p99 12.4318 ms,
maximum 20.2852 ms. The largest frame spent 10.5368 ms in render preparation
and 11.5414 ms in render CPU work, with full batch rebuild state 65535.
Another 19.1727 ms interval on the return pass spent 10.3937 ms in render
preparation despite incremental rebuild state 4. These are remaining measured
costs, not evidence of a remaining synchronous SVG generator call on the GUI.

Middle-button scrolling with the pointer 296 logical pixels from the origin
(one quarter of window height), four seconds per direction:
`vector-prepared160-middle-masonry.json` and matching `-present-summary.json`.
This followed the wheel run and had a warmed scene/extent of 3710.142857;
it is not a cold first-entry test.

| Middle direction | Display median | Display p95 | Display p99 | Maximum display gap |
|---|---:|---:|---:|---:|
| Down | 6.2498 ms | 7.2589 ms | 9.3490 ms | 9.3541 ms |
| Up | 6.2498 ms | 7.2676 ms | 9.3610 ms | 9.3906 ms |

Native frame production: median 6.2333 ms, p95 7.3564 ms, p99 8.0248 ms,
maximum 10.2796 ms. All recorded presents were displayed in both captures;
that does not prove a new frame reached every refresh. The standalone prototype's
roughly 1 ms incubation slices are not a bound on all integrated frame costs.

## Remaining target

Constant 160 Hz without spikes is **not achieved**. Investigate render-preparation
and upload costs next, then the remaining pre-frame/scheduling gaps. Cold and
warmed scenes, wheel bursts, middle scrolling, actual display cadence and
175% resting pixel alignment remain acceptance conditions. Do not replace them
with a warmed-only or rasterized-vector workload.

AI assistance has been used to create this output.
