# Qt Quick physical-pixel alignment proof

This standalone Qt 6.11.1 experiment compares the same QML scene in two
windows. The first uses the ordinary Qt Quick rendering path. The second opts
into an engine-level pixel-grid policy. QML `x`, `y`, `width`, `height`,
implicit sizes, anchors and bindings remain logical and unchanged; rendering,
item mapping and hit testing use corrected physical-pixel presentation
geometry.

The proof is deliberately limited to ordinary, unrotated and unscaled UI. It
covers nested items, layouts, Rectangle borders, Image destinations, native
Text and editable text, stock Basic-style controls, clipping, rectangular
ShaderEffect items with Qt's default vertex stage and built-in grid, and window
captures. Layered content, custom shader vertices/meshes, rotated/scaled items
and custom renderers are reported as unsupported. They are not counted as passing
coverage. Animation frames are snapped too; the test verifies the endpoint
returns to aligned geometry.

## Run the comparison

The current proof executable is `build/rectangle-compatible/qt-pixelgrid-zoin.exe`.
The earlier shader alignment build remains at `build/shader-aligned/qt-pixelgrid-zoin.exe`.
The shader baseline remains at `build/shader-diagnostics/qt-pixelgrid-zoin.exe`.
The earlier raster fixture build remains at `build/raster-icons/qt-pixelgrid-zoin.exe`.
The performance snapshot remains at `build/perf-optimized/qt-pixelgrid-zoin.exe`,
and the initial proof at `build/qt-pixelgrid-zoin.exe`. These are static
`/MT` executables with QML and the required static plugins embedded. Run the current build on
the primary display to open **Original** and **Pixel aligned** side by side.
The tested display reported DPR 1.75. Do not set `QT_SCALE_FACTOR=1.75` for
this native run: Qt multiplies that override by the monitor's native DPR.

The controls move the fractional nested scene, resize a bound rectangle,
change bound text, enable animated movement and save a window capture. The
sample also contains a small editable TextInput, a generated 16×16 raster icon,
odd-sized centering, a button, checkbox and clipped edge. The status line
reports the window DPR and the policy's unsupported-item count.

Four additional raster fixtures have white interiors and cyan perimeter borders
exactly one physical pixel thick. Their image provider produces 48×48, 56×56,
64×64 and 72×72 bitmaps, displayed at 1:1 texel-to-screen-pixel resolution.
The URL fixes the physical bitmap size; logical Image dimensions divide that
size by the current screen DPR. No position snapping is present in the QML.
Linear texture filtering stays enabled so fractional placement is visible in
Original. The fixtures have 0, 1, 2 and 4 extra Item ancestors inside their
diagnostic group, all below the existing fractional moving ancestor.
Measured coordinates, pixel comparisons and the software-renderer regression
are recorded in [the raster fixture report](profiler/reports/raster-icons-2026-09-26.md).

The bottom row adds two procedural GPU shader samples, each 192×96 physical
pixels, with a one-pixel cyan frame and white horizontal/vertical stripes:

- **Gallery:** Qt Quick ShaderEffect, the same rendering primitive used by
  `GalleryThumbnailShaderLayer.qml` and the viewer filmstrip. Its fragment
  shader draws the diagnostic pattern instead of the production image effect.
- **Viewer:** an exact snapshot of F4's `ViewerResampleEffect` C++ renderer and
  `viewer_resample.vert`. Only the fragment stage is replaced with the pattern.
  The **F4 viewer's own pixel snapping** checkbox controls its existing vertex
  alignment independently of the window policy. It resumes automatically when
  ancestor movement stops, just as the production viewer's resting path does.

Strokes integrate coverage over a physical pixel rather than using a hard
threshold that would hide fractional translation. The normal ShaderEffect now
inherits the window policy automatically: its scene origin, rectangle, mapping
and containment align without component-specific snapping. The viewer sample
still becomes crisp through its own vertex shader. Its QQuickItem mapping remains fractional and
is not claimed as a coordinate-mapping or interaction fix. Software rendering
shows an unavailable label and does not instantiate or claim GPU shader tests.
The comparison windows restrict QML imports to embedded resource paths, so
loading these samples cannot silently depend on the installed Conan QML tree.
See [shader alignment and regression results](profiler/reports/shader-alignment-2026-09-26.md)
and the retained [shader baseline](profiler/reports/shader-diagnostics-2026-09-26.md).

The engine aligns the shader's presentation rectangle; it does not rewrite
arbitrary fragment code or application uniforms. A fragment shader can still
intentionally draw fractional strokes, blur, or resample textures. The 1:1
physical-pixel diagnostic matches all 18,432 reference pixels in the aligned
window. Custom vertex shaders, mesh objects (including explicit GridMesh
objects), ShaderEffectSource/layers and the custom F4 viewer remain excluded.
Built-in grids specified as a size, including subdivisions, are supported.

Run the geometry, image-capture and interaction proof from PowerShell:

```powershell
Remove-Item Env:QT_SCALE_FACTOR -ErrorAction SilentlyContinue
Remove-Item Env:QSG_RHI_BACKEND -ErrorAction SilentlyContinue
Remove-Item Env:QT_QUICK_BACKEND -ErrorAction SilentlyContinue
$env:QT_QUICK_CONTROLS_STYLE = 'Basic'
.\build\rectangle-compatible\qt-pixelgrid-zoin.exe --check
```

The check requires the unaligned view to reproduce at least one fractional
physical-pixel origin, prints every named item's baseline and aligned scene
origin, confirms logical geometry is unchanged, verifies whole-pixel bounds
and identity unit vectors, then exercises resize/content changes, policy
toggles, a fractional animation endpoint, button activation, text editing and
the rendered cursor rectangle. It changes the caption font size and raster
source through QML bindings, then checks the text implicit size and image
loading update. A dynamic rectangle fixture checks mapping before and after
synchronization and exact captured pixels after ancestor-only movement,
reparenting, destruction/recreation, resizing and repeated policy toggles.
The raster regression checks the provider's perimeter and every captured texel,
plus a one-pixel exterior margin, initially, after ancestor movement/resize and
at the animation endpoint. RGB tolerance is one code value; frame thickness,
dimensions and alignment are exact. Every new image and caption is also covered
by scene-origin and identity-transform assertions.
Shader tests require visible GPU output, compare original and corrected item
origins, assert presentation edges and identity transforms, and compare all
18,432 pixels against an independent integer reference. A separate solid shader
with fractional dimensions verifies exact quad and clipping pixels during
ancestor movement, reparenting, recreation, resizing and policy toggles. It
checks containment at the visible boundary and actual mouse delivery, and
switches custom vertices/meshes on and off to verify coverage invalidation.
Tests also exercise the viewer's own alignment on/off and animation endpoints.
Rectangle regressions compare the Move buttons' painted extents with the actual
Original window, measure all four border widths, and repeat after five immediate
window resizes. Nested positive/negative half-pixel cases check mapping, shared
edges and identity unit vectors at every tested DPR.
It saves native-resolution captures beside the executable in `captures/`.
The initial characterization images are retained in `build/captures/`.
`baseline-original-native.png` and `baseline-aligned-unpatched-native.png`
record the initial run against the stock Qt library;
`comparison-original-native.png` and `patched-aligned-native.png` compare
policy off/on in the patched build. The current test scene's baseline
included `smallCaptionLeaf` at physical `(71.96, 393.557)` and
`editableTextLeaf` at `(71.96, 430.657)`; the corrected origins are whole
physical pixels. The latest hardware aligned capture reports DPR 1.750 and
one unsupported item: the custom F4 viewer renderer.

For a separate software scene-graph renderer run, keep the monitor's native
DPR and set only the renderer:

```powershell
Remove-Item Env:QT_SCALE_FACTOR -ErrorAction SilentlyContinue
Remove-Item Env:QSG_RHI_BACKEND -ErrorAction SilentlyContinue
$env:QT_QUICK_BACKEND = 'software'
$env:QT_QUICK_CONTROLS_STYLE = 'Basic'
.\build\rectangle-compatible\qt-pixelgrid-zoin.exe --check
Remove-Item Env:QT_QUICK_BACKEND
```

The check prints the actual scene graph API and fails if software was requested
but is not active. `QSG_RHI_BACKEND=software` does not select Qt Quick's software
adaptation; the older proof's software label was therefore not sufficient.
The software renderer already rounds these original raster draws, so its
unaligned window need not exhibit texture filtering. Its aligned window must
still match every reference pixel. The new fixture exposed a Qt software
renderer bug: rounding image bounds in logical coordinates could remove an
edge row before DPR conversion. The supplemental source change retains QRectF
bounds until scene transformation. It applies to both software comparison
windows and does not affect the hardware renderer.

The deterministic CTest matrix checks effective DPR values 1.0, 1.25, 1.5 and
2.0. On the 1.75 primary monitor, its scale factors are the ratios of each
target to 1.75 and the test asserts the observed window DPR. It uses the native
Windows window system; the offscreen platform hung in this Qt build even
without exposed windows.

```powershell
& 'C:\Dev\Qt6.8.1\Tools\CMake_64\bin\ctest.exe' --test-dir build/rectangle-compatible --output-on-failure
```

## Rebuild the experiment

The checked-in source patch is
[`patches/qt-6.11.1-f4-pixel-grid.patch`](patches/qt-6.11.1-f4-pixel-grid.patch).
It was generated from the official Qt 6.11.1 source archive whose SHA256 is
`252acef8c5ae68074d91cadba2ee4a83465051bbb970dd26e8f0daa0f3904e03`, matching
the Conan recipe. The installed Conan package did not retain its source tree;
the archive and extracted source live under ignored `.work/`. The reusable
export check is `scripts/export-qt-patch.ps1`.

On this host, `scripts/build-qtquick-experiment.ps1` compiles the ten changed
Qt Quick translation units using the retained Qt 6.11.1 Release `/MT` build
flags and headers, then replaces those exact members in a copy of the
installed `Qt6Quick.lib`. It writes only the experiment archive under ignored
`.work/` and refreshes `build-provenance.txt`. It never changes the Conan
package. Configure the demo against the local Qt toolchain and that archive,
then build from an x64 Visual Studio developer environment. This build
uses a new archive to retain the earlier shader, raster and performance snapshots:

```powershell
.\scripts\build-qtquick-experiment.ps1 -OutputLibrary Qt6Quick-rectangle-compatible.lib
cmake -S . -B build/rectangle-compatible -G Ninja `
  -DCMAKE_MAKE_PROGRAM=C:/Dev/Python/Scripts/ninja.exe `
  -DCMAKE_TOOLCHAIN_FILE=D:/Code/f4-zoin/qt/host/build-static-final5/conan_toolchain.cmake `
  -DQt6_DIR=D:/Code/f4-zoin/qt/host/build-static-final5 `
  -DCMAKE_BUILD_TYPE=Release `
  -DQT_PIXELGRID_PATCHED_QUICK_LIBRARY=D:/Code/f4-zoin/tools/qt-pixelgrid/.work/Qt6Quick-rectangle-compatible.lib
cmake --build build/rectangle-compatible --config Release
```

For a stock-library baseline, configure with the same toolchain but omit
`QT_PIXELGRID_PATCHED_QUICK_LIBRARY` in a fresh build directory, or explicitly
clear the cached value with
`-DQT_PIXELGRID_PATCHED_QUICK_LIBRARY:FILEPATH=`. Build and run `--check`
before applying the Qt patch. The experiment-only archive and executable
remain separate from the installed Conan package and all F4 binaries;
production integration would need a separately versioned Qt/Conan package.

## Patch behavior

The per-window C++ interface in `src/QtQuickPixelGrid.h` enables the policy and
reports presentation bounds or unsupported items. State is kept outside Qt
class layouts. The patch computes each item's absolute scene origin through
its ancestor and `QQuickTransform` chain, rounds to the actual window DPR, and
applies only the correction between that item and its corrected parent. It
uses the same correction in scene-graph node transforms and item mapping, so
visible bounds and input hit testing follow the same presentation geometry.
Primitive rendering snaps absolute destination edges to keep shared borders
together; text placement, clipping and the TextInput cursor rectangle use the
same physical grid. A separate static state property opts each window in and
reacts to screen/DPR changes without changing logical QML values.

At exact half-pixel edges, the hardware path follows the stock aliased
rasterizer's pixel-center coverage: `N + 0.5` selects `N`. This preserves the
already crisp Move button footprint instead of shifting its top and shrinking
its height by one pixel. The software backend retains Qt's `qRound` convention.
Both stabilize floating-point ties within 0.000001 physical pixels. Border
width continues to use Qt's existing `border.pixelAligned` calculation;
`border.width: 1` is two physical pixels at DPR 1.75.

For supported software rectangles, a node side table preserves the fractional
logical presentation bounds through QPainter. The stock node's early conversion
to an integer logical QRect can enlarge an edge past its physical clip and hide
the right border after resizing. The side table is active only with the policy,
is cleared on disable/destruction, and adds no fields to Qt classes. See the
[rectangle compatibility report](profiler/reports/rectangle-compatibility-2026-09-26.md)
for the before/after pixel measurements.

During dirty-node synchronization, a thread-local cache shares geometry among
node transforms, clipping and primitive rendering. It computes each required
ancestor first, uses direct arithmetic for ordinary translations, and reads
the window policy/DPR once. Entries expire every synchronization; mapping and
input outside synchronization always calculate fresh geometry. Item mutation
during synchronization invalidates the active generation. Qt class layouts
are unchanged.

Ancestor movement enqueues descendant presentation transforms in one walk.
Qt's normal logical-transform notification still runs for the actual move;
the patch avoids sending it again for every descendant. Input-containment
hints are invalidated and recalculated when needed. Policy changes enqueue
transforms, content and clipping in one walk.

ShaderEffect also revisits its quad when only an ancestor moved. Its existing
first/last grid vertices act as the stored presentation result; a geometry
upload occurs only when those bounds changed or Qt already marked it dirty.
Clipping rectangles are similarly compared and refreshed when their rounded
extent changes with scene position. Vertex-shader and mesh policy transitions
invalidate both the effect and its descendants. No Qt class layout changes or
per-frame full-tree shader pass are introduced.

Ordinary translations are constructed directly from the difference between
snapped scene origins. The scene node receives a new matrix only when this
final matrix changes. This avoids redundant `DirtyMatrix` work and numerical
jitter from separately converting the logical position and correction to float.

## Performance stress benchmark

`qml/PerformanceScene.qml` creates 445 visual items: 400 small rectangles split
between a flat repeater and twenty nested repeaters, plus their containers.
Fractional coordinates and animated ancestor positions exercise moving content
in both hierarchies. The benchmark warms up for 120 frames and records 360 more
by default. It reports frame intervals and render-thread time from
`beforeSynchronizing` through `afterRendering`.

Run three stock and three patched samples on the primary monitor:

```powershell
Remove-Item Env:QT_SCALE_FACTOR -ErrorAction SilentlyContinue
Remove-Item Env:QSG_RHI_BACKEND -ErrorAction SilentlyContinue
& .\scripts\profile-performance.ps1
```

The script builds into separate `build/perf-stock` and `build/perf-patched`
folders, leaves the existing demo executable alone, and writes raw samples to
`build/performance/benchmark-results.json`. Use `-Runs`, `-WarmupFrames`, and
`-SampleFrames` to change the sample size. It uses the monitor's actual DPR;
the current host reports 1.75. The captured result is summarized in
[`profiler/reports/performance-comparison-2026-09-26-152409.md`](profiler/reports/performance-comparison-2026-09-26-152409.md).

The optimized comparison, its limits and verification results are in
[`profiler/reports/performance-optimization-2026-09-26.md`](profiler/reports/performance-optimization-2026-09-26.md).
To compare stock, the retained previous patch and the optimized patch:

```powershell
& .\scripts\profile-performance.ps1 -Runs 5 -WarmupFrames 180 -SampleFrames 900 `
  -PreviousQuickLibrary .\.work\before-sync-cache\Qt6Quick-pixelgrid.lib `
  -ResultDirectory .\build\performance-final-animation
```

The three builds use the same application sources. Their run order rotates.
Add `-SkipBuild` to use existing benchmark binaries. Add `-MeasureUpdates` to
drive the same phase binding once per rendered frame with a deterministic
sequence instead of NumberAnimation. This mode also times the synchronous GUI
property write, binding evaluation and descendant invalidation. It excludes
later GUI polish/event processing; it is not total GUI-thread CPU time.

The render-thread interval measures synchronization and CPU-side render
submission; it does not measure GPU completion or compositor presentation. The
QML Profiler could not establish a debug-protocol connection to this static
executable, so the report does not attribute the measured time to individual
Qt functions.
