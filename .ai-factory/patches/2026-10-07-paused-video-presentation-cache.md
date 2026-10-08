# Reuse unchanged video presentation without reducing filter quality

**Date:** 2026-10-07 Europe/Moscow
**Severity:** high
**Scope:** ZoinGallery final video presentation; cached Qt/FFmpeg unchanged.

## Evidence and cause

A paused frame did not prove an idle render loop: live process sampling was
mostly idle. However, unrelated window redraws executed the full final video
filter again. Before the fix, the hardware regression submitted four final
filters for four forced redraws with an unchanged frame revision, on both the
direct and pyramid paths at DPR 1 and 1.75.

The supplied 3840x2160 H.264/YUV clip uses direct filtering at 2560x1440. Each
output pixel can perform 64 YUV-to-linear-light sample evaluations. Reusing only
decoded/uploaded video textures does not cache this final presentation work.

## Implementation

- Retain the completed video filter in a live RGBA8 layer, refreshing on content
  or material changes rather than on unrelated window redraws.
- Preserve the existing linear-light filter, kernel, tap count, transfer
  conversion, and dithering. Do not introduce bilinear video filtering.
- Resolve aligned filter coordinates at exact physical-pixel centers and use
  their exact source/output footprint. Motion/intermediate passes remain
  continuous. This eliminates offscreen-projection rounding differences.
- Copy retained texels through the existing framebuffer-corrected vertex path.
  Qt's ordinary layer quad clipped an edge in the initial fractional-DPR test;
  it was rejected before restarting the application.
- Fall back to direct presentation during movement, inherited fades, non-unit
  transforms, fractional scene edges, screen/backing-DPR mismatch, unsupported
  projection grids, and oversized caches. Do not lower cache resolution.
- Limit each cache dimension to 4096 and the RGBA8 pixel budget to 16 Mi pixels.
  Hidden pyramid levels retain their existing live/dirty behavior.
- Gate per-submission diagnostics behind `F4_VIEWER_RESAMPLE_DRAW_TRACE`.
  Keep QtQuick private-header use conditional on video playback being enabled.

## Verification

Strict cached-versus-direct comparisons use zero tolerance. Synthetic BGRA and
NV12 cases cover native scale, magnification, direct reduction, pyramid
reduction, odd extents, frame replacement, fresh rotated/mirrored frames,
nearest-neighbor mode, opacity/movement fallbacks, clear-to-image, and allocation
limits. A shallow QVideoFrame copy must not be mutated after sink delivery;
orientation fixtures create fresh immutable frames and verify a new revision.

Check scene-space origins/extents and unit vectors at 175% for both the actual
viewer image shader and the named cache compositor. Rendered captures include
the real clip at 2560x1440 and the full viewer hierarchy. Idle render-control
signals are checked separately from forced redraw reuse.

The real 4K frame is pixel-identical with caching enabled/disabled. Repeated
cached render plus synchronous readback measured approximately 3–5 ms, versus
56–71 ms direct, on Apple M2 hardware OpenGL. These include readback/copy costs;
they are not isolated GPU timings or proof of live compositor responsiveness.
New-frame filtering still has its original cost; playback needs a separate
live performance check before considering another quality-preserving change.

The broad GPU suite reports 65 passes and the same four pre-existing image
failures. Existing checks were not weakened. CPU resampling reports 27 passes;
video geometry reports five passes. Portable glibc 2.27/hardening, embedded-only
QML import, payload extraction, and static Go checks pass.

The final named-compositor geometry and idle-signal checks report 23 passes,
including the full viewer at 175%. This checkout was rebuilt, repackaged and
restarted directly with `--gui=qt`; the live log confirms Wayland/hardware
OpenGL on Apple M2. User confirmation of actual paused/playing responsiveness
is still required. The launch path selects the supplied clip; reopening its
viewer is a separate live interaction.

The independent missing-audio-backend issue remains in FIX_PLAN.md. No Qt or
FFmpeg recompilation, alternate audio engine, commit, or push was performed.

## Prevention / follow-up

Count expensive filter submissions, not merely window frames or CPU usage.
Require exact pixel comparisons and physical scene transforms for caching.
Continue manual live pause/resume, mouse/hover, resize, seek, close/reopen, and
multi-platform quality checks; do not claim playback performance is solved by
an unchanged-frame cache.
