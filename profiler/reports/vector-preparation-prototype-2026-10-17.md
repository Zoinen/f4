# Moving SVG scene preparation off the GUI thread

## Purpose and scope

The native scrolling trace documented in
[scroll-spikes-attribution-2026-10-17.md](scroll-spikes-attribution-2026-10-17.md)
identified synchronous `QQuickVectorImagePrivate::loadFile` in long frame gaps.
This experiment tests separating Qt's own SVG-to-QML generation from QML
compilation and item creation. It does not change the running application's
vector presentation yet and does not establish spike-free scrolling.

The standalone probe links the worktree's current static Qt 6.11.1 host
dependencies and embedded imports. It uses the same
`QQuickQmlGenerator` used internally by `VectorImage`, with `CurveRenderer` and
`AsyncShapes`. No SVG is rendered into a bitmap. Generation and file writing run
on a worker; `QQmlComponent::Asynchronous` compiles the result; an asynchronous
incubator receives 1 ms GUI slices. Only one source is generated at a time.

Reproduction sources and results are retained locally:

- `artifacts/vector-preparation-probe.cpp`
- `artifacts/build-vector-preparation-probe.py`
- `artifacts/build-vector-preparation-probe.cmd`
- `artifacts/vector-preparation-probe.json`
- `artifacts/vector-preparation-probe-errors.log` (empty)

The probe has exited. The normal portable application remains running from
`D:/Code/f4-zoin/f4-zoin.exe`, without diagnostic overrides.

## Measurements

Inputs are three actual SVGs from Downloads. All three generated scenes loaded
successfully and retained their 1024 × 1024 intrinsic dimensions.

| Input | Original synchronous creation | Worker generation | Async compile wall time | Largest GUI incubation slice |
|---|---:|---:|---:|---:|
| `FileIcons_Dark+_nb_pro_1.svg` | 61.8812 ms | 10.0279 ms | 13.7360 ms | 1.0060 ms |
| `FileIcons_Dark+_nb_pro_1 (1).svg` | 14.6303 ms | 6.6073 ms | 11.2664 ms | 0.9301 ms |
| `PlayerControl+FileIcons_Dark++_nb_pro_5+.svg` | 12.1823 ms | 5.1493 ms | 10.6722 ms | 0.8387 ms |

The first synchronous case includes cold QML/import initialization and must not
be presented as the ordinary cost of every SVG. Worker generation and async
compilation durations are wall time, not GUI-thread occupancy. The direct
`loadUrl` calls took 0.0234–0.0283 ms, and starting incubation took
0.0797–0.0837 ms. The largest scene used two incubation slices; the others used
one. These are three observations, not percentile statistics or a hard bound for
arbitrary SVGs.

The result demonstrates that the measured source-generation cost need not
occupy a scrolling frame. It does **not** measure rendered frames, GPU uploads,
texture-backed SVG image elements, or QML completion notification costs in the
full application. Visual equivalence and animation behavior are unverified by
this probe.

## Constraints discovered

1. Qt 6.11.1's generator has a shared mutable static `QString indentString`.
   Concurrent generation is unsafe without serialization or a dependency patch.
   A worker mutex alone would not protect any remaining GUI-thread
   `VectorImage` generation. The production path must account for every caller.
   This was checked in the matching
   [Qt 6.11.1 source](https://github.com/qt/qtdeclarative/blob/v6.11.1/src/quickvectorimage/generator/qquickqmlgenerator.cpp),
   rather than an older source tree also present in the local Conan cache.
2. Creating successive generated QML files inside a directory already examined
   by the engine triggered `File name case mismatch` for later files. Fresh
   per-revision directories fixed the probe. Publication must be atomic and
   immutable, and directories must remain pinned for the lifetime of the scene.
3. SVG generation may create companion assets. A prepared scene needs ownership
   of its temporary directory as well as its original VFS lease. Releasing the
   source early would leave asynchronous compilation reading deleted files.
4. Replacing `VectorImage`'s synchronous loading wrapper must preserve its
   generated scene semantics: initial animation pause for panels, authored
   loops in the viewer, intrinsic size, fit/crop, curve text, transparency,
   revisions, stale completion rejection and failure skipping.
5. Compilation and incubation must remain asynchronous after a scene is cached;
   forcing completion would reintroduce the GUI stall. Integrate with the
   existing bounded viewport/prefetch scheduler and active-viewer priority.

## Next integration and acceptance checks

Prepare the immutable scene alongside vector source acquisition on existing
workers. Publish it only after generation succeeds; keep failure/retry semantics
and cancellation authority with the catalog. Load and instantiate through an
asynchronous surface, with a generation token and a pinned prepared-source
handle. Avoid regenerating scenes on zoom or resize.

Before enabling that path, verify gradients, non-square viewports, SVGZ,
transparency, text, embedded images, animations and loop behavior, malformed
files, source replacement and cancellation. Capture actual rendered output at
175%, compare it to the existing vector surface, and rerun mixed-image
navigation, panel modes and import isolation. Finally capture cold and warm
scrolling on the 160 Hz display with native stage timings and PresentMon.
Render-thread batch uploads remain an independent measured problem.

AI assistance has been used to create this output.
