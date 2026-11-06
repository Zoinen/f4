# Cached physical-pixel presentation

The Qt host and ZoinGallery share `ZoinGallery::PixelGridContext`, `GridMetrics`,
`PixelGridAlignment`, and `PixelGridScope` from ZoinGallery's public
`include/ZoinGallery/PixelGrid.h`. The implementation is in `src/pixelgrid/`.
The QML companion module is `ZoinGallery.PixelGrid 1.0`; registration does not
require a gallery session. Host `ScenePixelAlignment` is an adapter to the same
context, not a second alignment pass.

## Geometry and lifetime

A window owns one context, using its effective DPR. Layout plans commit shared
integer physical edges, distribute column remainders cumulatively, then convert
to Qt Quick logical geometry. Capacity uses ceiling; centering chooses one of
the neighboring physical pixels. Text shaping and glyph advances stay with Qt.

An attached leaf caches its correction. QML delegates retain the attached
alignment object and read its scalar `offsetX`/`offsetY` properties, avoiding
point-value wrappers and helper reevaluation when only the correction changes.
The `offset` point API remains available for existing consumers. Ancestor
observations are shared in a persistent hierarchy. Only dirty visible branches are visited, parent first;
there is no per-frame enumeration, allocation of leaf lists, depth calculation,
or sorting. Parent changes rebuild affected chains. Hidden branches defer work;
showing, geometry/font layout changes, window changes and DPR changes invalidate
the necessary nodes. Custom transform changes must explicitly invalidate the
scope. Diagnostic counters are collected only after `resetCounters()`.

Watched nodes cache whether they own a grid boundary. New descendant
registrations reuse that information and check a Qt-owned moving content root
once per parent relationship. Installing an attached scope calls
`invalidateScope()` to refresh the boundary and rebuild affected chains;
reparenting invalidates the owner check. The private scope marker must not be
changed independently of that invalidation. Dirty traversal reads cached
boundaries instead of repeating reflective property lookup on every ancestor.
The `boundaryChecks` diagnostic counter counts boundary discovery calls.

`PixelGridScope` owns a translation, not a layer or render texture. Children fit
in its local grid once. Scrolling changes the common translation fractionally;
the resting translation is fitted once. A scope's phase change does not dirty
its descendants. Qt-owned Flickable/ListView content items get an attached
scope when their first aligned descendant registers; the existing Qt content
item and its layout bindings stay intact. The GUI-thread flush runs before
scene synchronization through `QQuickWindow::afterAnimating`. A future custom
QQuickRenderControl driver must call `flushWindow()` after polish and before
sync explicitly.

## Motion

Wheel animation retargeting preserves one effective gesture until settlement.
Intermediate NumberAnimation restart notifications must not persist offsets,
change alignment state or finalize scrolling. Middle-button scrolling uses the
same viewport scope. Text retains its configured renderer throughout motion;
Qt native glyph pixel steps are accepted. No custom glyph shader is installed.

Actual image-viewer zoom, pan, rotation, vector geometry and media resampling
remain media operations. Their mapping is intentionally separate from UI-leaf
fitting. The grid service never rasterizes SVGs or changes their authored paths.

Dirty leaf alignment is deferred while its own cached scope is moving. The
context records only the affected alignment handles against that boundary;
it preserves their last corrections and performs no scene mapping for those
leaves until motion ends. Settlement requeues only the deferred leaves through
the ordinary parent-first flush. Stationary scopes continue fitting, and chain
removal clears deferred handles when an item is destroyed or reparented. Scope
owners publish the cached motion flag once per transition; callers must not
change the private `_zoinPixelGridMotionActive` marker independently.

The native camera shares GalleryViewportMotionState with the owning
Masonry layout. While active, viewport hit testing and rectangle selection use
its committed render position, correcting the GUI scope applied translation.
Prepared delegate roots use its render-thread culling region through
GallerySceneItem; intrinsic child gates keep their ordinary visibility.
Clearing a node camera binding checks the owner so stale cleanup cannot clear
a replacement camera. A bounded prepared margin may extend the culling region;
the viewport clip still defines the visible boundary. The ordinary GalleryPanelMotion path uses this camera for eligible Masonry
panels. Cached SVG upload gates use
the exact visible camera region for presentation and priority, independently
of a conservative prepared-root culling region. Binding captures source
generation and readiness; unready, incomplete or stale surfaces stay hidden.
Renderer readiness belongs to resident GPU batches, not a saved source hint.
Cached SVGs returning after culling use the same bounded upload queue as new
geometry. A render-thread progress observer survives initial source readiness;
it requests pending uploads and one completion frame without per-frame GUI
notifications. Completed surfaces stop requesting frames.
A fully admitted cached surface can join the current frame after preparation.
Incomplete or failed candidate draws are filtered after the complete-surface
check, preserving the original painter order without presenting a partial SVG.
Native motion queries expected cells in a bounded content-space window.
A prepared-cell bounding union is not a readiness proof: a neighboring column
can hide a missing cell. Numeric camera ranges exclude each missing expected
cell. While a camera is active, delegate preparation leads toward its input
target, capped by the existing metadata window; ordinary plans stay unchanged.
GalleryPanelMotion now uses the NumberAnimation-compatible GalleryScrollAnimation
owner. GalleryPanel enables its native Masonry path by default; eligibility
checks preserve GUI fallback for other modes and software rendering. It keeps one
render
job across retargets and commits the latest pose in proxy write-back before QML
persists a stop. Explicit proxy ownership fixes the job threading decision at
construction. Ordinary GUI animation remains the fallback.
Camera membership bounds use GalleryItemMappingScope during the GUI-locked
synchronization pass. Translation-only siblings share their parent mapping;
rotated, scaled or custom-transformed leaves keep the complete Qt mapping.
The scope retains at most 32 parent transforms and falls back when full. It
must not survive geometry, transform, parent or window changes; every pass
creates a new scope. It changes neither grid fitting nor animation timing
and adds no persistent scene-geometry cache.
Masonry rewrap boundaries preserve the committed native anchor and translate
the running trajectory after extent restoration. Clamped endpoints retain the
current eased pose and elapsed clock instead of bending motion backward.
Masonry Home and End publish the destination viewport thumbnail demand at
input time. The render camera clips motion to prepared cells while an
independent 150 ms deadline commits the destination viewport.

After the first viewport, the active Masonry panel fills a deferred
catalog in bounded pages and processes metadata with existing frame
pacing. Offscreen geometry settles independently of scrolling while
scene objects remain limited to the viewport and its overscan.

This integration has component and rendering coverage. The full workload
has not demonstrated steady 160 Hz frame delivery.

## Renderer upload work

Qt retains up to 16 MiB of retired vertex buffers and 16 MiB of retired index
buffers per renderer. The bounded working set avoids repeated allocation and
Immutable-to-Dynamic conversion when ordinary UI batches and large SVG batches
share the pool. QSG_RENDERER_BUFFER_POOL_LIMIT remains an explicit override,
including zero retention. Existing releaseCachedResources clears both pools.
This budget does not bound total live GPU memory or remove all frame spikes.

The portable Qt recipe applies `ci/qt_quick_upload_policy.py` after the stable
order-interval and buffer-pool fixes. It retains compatible unmerged batches
when their complete render-list span is unchanged, and bounds root order-space
reservations globally. Strict source anchors reject unsupported Qt changes.

ZoinGallery vector surfaces opt into the render-thread-only
`f4.vectorUploadGate.v2` capability. Initial uploads are prioritized and spread
across frames, with a soft limit of 1 MiB, 32 batches or 2 ms of upload work;
one oversized batch can advance to avoid starvation. These limits do not bound
all render preparation, driver calls or total frame time. A surface is drawn
only after its complete batch set is prepared, with source-generation checks
for cancellation and replacement. Existing ready surfaces and ordinary Qt
clients keep their normal rendering path.

Cold previews can complete a few refresh intervals later; the measured
four-SVG reveal added roughly 20–40 ms after CPU preparation. This policy
reduces upload bursts but does not by itself establish constant 160 Hz.
Application shader resources include desktop GLSL and GLES variants on
Windows as well as other platforms, so OpenGL validation exercises the actual
custom viewer and thumbnail shaders.

Native scroll chrome uses the same camera as panel contents. GalleryScrollAnimation
accepts the vertical scrollBar control and snapshots its minimum-size thumb mapping
during synchronization. Render frames update only the thumb matrix. Stopping
restores the ordinary item transform; pressing the bar first commits the latest
camera and stops motion before the control evaluates the drag. Native Masonry wheel/keyboard motion is enabled by GalleryPanel. Middle-button
auto-scroll uses the same camera in continuous-velocity mode on eligible panels;
other modes and software rendering retain the GUI frame driver. Continuous
motion follows Qt's render-animation timeline. Wall-clock integration at commit
was rejected because queued startup frames made displayed motion uneven; the
optional f4.animationClock.v1 capability exposes Qt's full-precision VSync time
only during render-animation callbacks. Unsupported or invalid clocks use Qt's
ordinary animation time. Finite wheel/keyboard curves accumulate the same
precise deltas in their existing animation domain, including retargets and
geometry rebases; their timestamps are not truncated back to milliseconds.
Threaded render jobs finish their queued Qt timer registration at the first
synchronization boundary, before their first commit. The GUI/basic render loop
keeps its event ordering. No extra registration work is done on steady frames.
Repeated identical destination/duration/easing requests preserve the running
segment and its deadline; changed commands still retarget.
Clock-domain changes never integrate the unavailable
interval. Dead-zone and
end transitions commit the current camera and return to the resting grid.

## Verification

Real Text/TextInput/TextEdit/image leaves must be checked after layout settles:
scene-space origins times actual DPR are whole pixels and local unit vectors
remain scene unit vectors. Captures must contain visible smallest/secondary
text, not just aligned wrappers. Tests cover 175% as well as the other supported
fractional scales. Retained motion tests assert zero fitting/maps/topology work;
mode tests cover wheel bursts, middle-button scrolling and resting captures.

Continuous motion stops and commits its current pose when the window is hidden
or minimized. Auto-scroll also disarms; restoring the window does not resume
an old gesture or accumulate hidden time. A fresh gesture starts at that pose.
