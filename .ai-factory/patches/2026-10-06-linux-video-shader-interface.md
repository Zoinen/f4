# Match both video shader stages on OpenGL

**Date:** 2026-10-06 23:05 Europe/Moscow
**Files:** third_party/ZoinGallery/resources/viewer_resample.vert,
third_party/ZoinGallery/ViewerResampleEffect.cpp,
third_party/ZoinGallery/CMakeLists.txt,
third_party/ZoinGallery/tests/ViewerResampleGpuTest.cpp,
qt/host/CMakeLists.txt, qt/host/README.md
**Severity:** high

## Problem

Linux video playback advanced its timeline but rendered transparent frames.
The OpenGL log reported incompatible declarations of uniform block `ubuf`,
followed by a graphics-pipeline failure. Sound was independently unavailable.

## Root Cause

Video used an extended fragment uniform block with the image-only vertex
shader. OpenGL links stage interfaces together and rejects the mismatch;
successful behavior on other graphics backends did not prove this interface.
The prebuilt ARM64 Qt package also omitted every Linux audio backend. A valid
FFmpeg decoder and a working desktop PipeWire server cannot supply that missing
compiled Qt implementation.

## Solution

Compile a video vertex variant using the same define and uniform fields as its
fragment counterpart, then select both video stages together. Preserve the
image pair and C++ std140 layout. Add opt-in material creation and texture-upload
failure/recovery diagnostics through the existing media trace.

First reproduced four blank-frame failures on Apple M2 hardware OpenGL at DPR
1 and 1.75. The same pixel regressions now pass, including pyramid downsampling,
two different frames, and returning to the image material. Compiled QSB
reflection checks field names, order, types, offsets, sizes and matrix layout
in each stage. The real 4K user file renders correctly in a 175% GPU capture.

The Linux configure gate checks generated Qt headers, not live server
availability or Conan option metadata. Disabled features are `-1`, so compare
explicitly with `1`. Reject audio-less playback builds by default; expose only
an explicit, warned video-only diagnostic override. Artifactory still has no
matching audio-enabled package. No dependency recompilation or alternate audio
pipeline was attempted; the remaining audio work stays in FIX_PLAN.md.

Focused checks pass; four broader image GPU assertions still fail on this
device. The image vertex QSB is byte-identical to the original source's
compiled output, and existing checks were not weakened. Native portability,
embedded-only QML, payload extraction and static Go audits pass. The new app
was restarted directly on Wayland with hardware OpenGL.

## Prevention

- Test the actual material on each graphics API, not only metadata or decoding.
- Compare compiled stage interfaces as well as C++ uniform offsets.
- Keep exact RGB frame tests separate from CPU/GPU YUV chroma differences.
- Treat audio implementation, live device availability, and active playback
  as three independent gates.
- Require audited audio-enabled binaries; never conceal a missing package by
  rebuilding dependencies against a newer baseline or silently shipping soundless
  playback.

## Tags

`#qt` `#opengl` `#shader-interface` `#video` `#linux` `#conan` `#audio-backend`
