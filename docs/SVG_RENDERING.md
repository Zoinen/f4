# SVG rendering

SVG and SVGZ file previews use Qt 6.11 VectorImage with the curve renderer in
Details, Columns, Grid, Icons, Masonry, and the flat image viewer. Interface
icons retain their existing rendering path. SVG rendering is automatic and adds
no semantic wire field. The shared transparency checkerboard preference is
configurable in GUI Gallery settings; F12 in the viewer toggles it immediately
for the viewer and both panels, and saves it across restarts.

## Source ownership and scheduling

VectorSource validates the document and intrinsic dimensions on an existing
image worker. Virtual sources use ImageSourceProvider materialization and retain
its lease. Sources are keyed by the existing immutable source revision. The
decode manager reuses weak handles; VectorSourcePin gives each QML surface an
independent strong handle, including during viewer transitions. Catalog
retention is bounded by recent revisions, visible/overscan rows and viewer
neighbors. Stale revision/authority completions are discarded.

Vectors use existing acquisition priorities and cancellation. The active viewer
comes first, followed by alternating next/previous candidates. Failed documents
do not consume the successful-image prefetch budget. Vectors bypass raster
thumbnail restoration and Fit/Native bitmap frames. Zoom and resize reuse the
surface and acquired source.

The viewer prepares a replacement scene in a second vector surface while the
current scene remains visible. GalleryVectorSwap commits only a ready surface
matching the latest requested URL, then releases the previous scene. Source
dimensions, fit/zoom and remembered viewport state commit with that scene; an
outgoing SVG never adopts its replacement's aspect ratio during preparation. Pending
scene preparation has zero opacity but retains effective visibility so Qt
Shapes can finish; panel previews keep their existing single surface.

## Presentation

GalleryVectorImage owns loading, ready and failure states without relying on
Qt 6.12 APIs. Shapes prepare asynchronously. Readiness polling stops when ready
or hidden. A procedural shader draws the transparency checkerboard. SVG text
uses curve rendering. Panels pause authored animation at its initial state;
visible viewer surfaces play it, retain authored repeat counts and pause when
hidden. Spherical viewing and bitmap sampling controls do not apply.

Definitive parse or scene failures restore ordinary panel file presentation and
enter viewer failure/skip handling. Source-access failures remain retryable;
changing the revision clears recorded failures. There is no raster fallback.

Qt VectorImage's supported SVG feature set is the rendering boundary. In
particular, blur/filter effects can be omitted by the Qt 6.11 curve renderer;
this is visible on SVG files with authored shadows/glow. Embedded raster content
inside an SVG remains authored raster content.

## Verification

GallerySessionTest covers local and virtual SVG/SVGZ acquisition, malformed
files, source replacement, cancellation, lease pinning, stale failure reports,
mixed SVG/photo navigation, failure skipping, successful-only prefetch,
resizing without rereading, animation visibility/repeat behavior, and all five
panel modes. At DPR 1.75, tests check scene-space origins and unit transforms of
vector surfaces and actual visible caption/secondary-text leaves, and capture
rendered output including small SVG text and thin strokes.

The synthetic acquisition/scroll/navigation trace measured 16 SVG metadata
operations: median 0.428 ms, maximum 0.571 ms, including worker source acquisition;
SVG validation alone had median 0.201 ms, maximum 0.368 ms. The small-caption
surface fixture took 24 ms to become ready including asynchronous scene
preparation. These are local/test-provider measurements, not Android transport
latency guarantees. Timing uses the existing F4_MEDIA_TIMING_TRACE path and the
surface sceneGenerationMs property.

Portable Windows verification includes qmlImportsWithoutInstalledQt, the system
DLL import audit, regenerated embedded host payload, and launch from the single
Go executable with F4_EXT_UI_PATH cleared. The extracted child's hash is checked
against the newly built host.
