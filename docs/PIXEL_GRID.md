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

An attached leaf caches its correction. Ancestor observations are shared in a
persistent hierarchy. Only dirty visible branches are visited, parent first;
there is no per-frame enumeration, allocation of leaf lists, depth calculation,
or sorting. Parent changes rebuild affected chains. Hidden branches defer work;
showing, geometry/font layout changes, window changes and DPR changes invalidate
the necessary nodes. Custom transform changes must explicitly invalidate the
scope. Diagnostic counters are collected only after `resetCounters()`.

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

## Verification

Real Text/TextInput/TextEdit/image leaves must be checked after layout settles:
scene-space origins times actual DPR are whole pixels and local unit vectors
remain scene unit vectors. Captures must contain visible smallest/secondary
text, not just aligned wrappers. Tests cover 175% as well as the other supported
fractional scales. Retained motion tests assert zero fitting/maps/topology work;
mode tests cover wheel bursts, middle-button scrolling and resting captures.
