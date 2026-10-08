# Skip a redundant resampling pass for moderate windowed video

**Date:** 2026-10-08 MSK
**Files:** `third_party/ZoinGallery/qml/ViewerResample.qml`,
`third_party/ZoinGallery/tests/ViewerResampleGpuTest.cpp`,
`ci/patch-qt-dependencies.py`, `ci/test_patch_qt_dependencies.py`
**Tags:** Qt Quick, video, GPU, resampling, Linux audio, CI, Conan

## Windowed playback

The earlier small-window optimization fixed repeated video-plane conversion,
but the benchmark still showed a cost gap at a 1538x866 output size. Testing
with and without the presentation layer showed no measurable difference. The
remaining cost was one full intermediate pyramid render for a moderate resize.

For decoded video only, when exactly one pyramid level would be selected and
both scale ratios are at most 2.6x, use the existing direct high-quality filter.
Deeper reductions still use the pyramid. The filter kernel and precision are
unchanged; still images and hardware/nearest-neighbor paths are unaffected.

The 4K GPU benchmark measured roughly 27.7-28.1 ms median for the previous
windowed pyramid path and 19.4-19.9 ms with direct sampling (about 29-30%
lower). Direct sampling matches the direct reference at zero pixel tolerance.
Rendered comparisons against the former pyramid remain visually very close;
the paths need not be bit-identical because the pyramid quantizes intermediate
levels.

Regression rows cover both presentation-cache states at direct, windowed and
quarter-size targets. They assert the selected level count and verify that the
deep-reduction pyramid remains selected. The GPU benchmark suite passed: 8
passed, 0 failed. It measures synchronized render/readback cost, not a claim
that this isolated benchmark is a live presentation-FPS measurement.

## Linux audio CI follow-up

The Linux build already requests Qt Multimedia, FFmpeg and PulseAudio and keeps
ALSA disabled. The first CI attempt failed before compilation because the Qt
recipe patcher used a broad `with_libalsa` marker: ConanCenter's separate
`package_info()` dependency branch matched it, so the patcher falsely treated
the original validation guard as a conflicting prior patch. The patcher now
recognizes the complete validation replacement, and a regression test includes
the unrelated package-info branch. The focused patch suite passes (5 tests).

## Verification limits

The CI run that exposed the patcher false positive was not a successful release;
its Linux single-file Qt jobs stopped while patching the Conan recipe. Full
suite CTest also reports unrelated baseline architecture and document-surface
failures in the portable Ubuntu test container. The focused video GPU test and
Linux audio patch tests are the change-specific gates.
