# Upload-policy rollout — 2026-10-18

## Shipped in the worktree build

The normal embedded Qt host now includes span-based unmerged batch retention, globally bounded order reservations, and v2 initial vector upload admission. The source transformation is part of ci/patch-qt-dependencies.py with data in ci/qt_quick_upload_policy.py, applied after stable-interval and buffer-pool fixes. It validates every anchor in both source files before saving either; recipe generation embeds all patch data and needs no repository helper at Conan source-preparation time.

Local full composition from the official Qt 6.11.1 renderer/interface sources equals the two previously compiled prototype sources after LF normalization. Proof: artifacts/current-upload-recipe-source-proof.json. The normal host response file was checked: both Qt6Quick references point to the verified combined archive. No Qt version, Go protocol, settings, or vector formats changed.

The original objective is not complete: prior current-source captures still show 9–11 ms GUI execution involving delegate creation/GC. The upload policy reduces the independently measured 17–21 ms render/upload bursts; it is a soft work budget, not a hard frame guarantee. Cold four-SVG reveal measurements show roughly 20–40 ms additional wait after CPU preparation. Full measurements and limitations remain in vector-complete-paint-latency-2026-10-18.md and current-render-stalls-2026-10-18.md.

## Correctness and packaging gates

- Native ordering/root/depth/overlap/mutation/lifetime/admission regressions: 30 passed on D3D11 and 30 passed on OpenGL, DPR 1.75.
- Current gallery SVG acquisition, failure/retry, navigation, successful-only prefetch, swaps, recycled previews, scene caching/cancellation, resize, native visibility, five panel modes, gradients/alpha/embedded images and authored animation: 27 passed on D3D11 and 27 passed on OpenGL after the shader repair below. Tests explicitly expect the v2 upload capability.
- All five wheel modes, Grid/Masonry alignment suspension/rest, Columns leaf origins/unit vectors: 10 passed on each backend at DPR 1.75. Actual rendered SVG and Columns text/icon captures were inspected, including thin SVG edges and secondary extension labels.
- Recipe regression suite: 31 passed. Coverage includes idempotent self-contained export, composition order, both implementation/capability sources, missing/ambiguous anchors, and no partial source saves on drift.
- Static import isolation plus shader resource variants: 4 passed.
- Windows system-DLL audit passed in promoted-upload-host-build.log.
- Fresh embedded payload, CGO_ENABLED=0 launcher build, and materialization/reuse/concurrency/corrupt-payload/source-precedence regressions passed. The running extracted host hash matches the freshly built normal host; startup stderr is empty.

## Two defects caught by the gates

The Windows gallery CMake previously used NOGLSL, excluding GLSL from custom shader packages. Both normal and candidate renderers failed OpenGL custom-shader rendering. The candidate's native failure handling exposed a stale raw-item dereference in the retained-scene test after failed graphics preparation removed its surface. The crash stack was in QQuickItem::parentItem called by the test, not proof of a codec fault despite the stripped executable's misleading nearby export name. Isolated symbolized comparisons identified the missing-shader path.

The build now packages GLSL 150 and GLES 300 variants on Windows too. A resource regression first failed on the old checkerboard package, then passed for all six core custom shaders. Rebuilt OpenGL gallery checks pass all 27 cases without the crash. Qt's qt_add_shaders reference documents that NOGLSL disables GLSL generation: https://doc.qt.io/qt-6/qt-add-shaders.html . Existing HLSL/MSL/SPIR-V generation remains available.

A Columns geometry test sampled late leaf movement when GUI suites ran concurrently: for example galleryExtension-0 physical x=300.996 instead of 301. Its earlier settlement predicate checked animation flags but not final leaf movement. The fixture now requires unchanged actual leaf origins across three rendered frames after flags settle, then applies the original strict physical-grid and unit-vector assertions. It does not round coordinates or loosen the threshold. The concurrent workload was rerun: Modes 10 passed alongside the relevant gallery cases (8 passed). OpenGL Modes also passes. No production alignment pass or rounding was added.

## Runtime identity and remaining work

Launcher SHA256: B510EBF344CBCD7FA1BDECB0A78C5C1BABC049E89BF2343E1D3448F44ACEB892.
Host SHA256: 7A9C2027297AD6B84085F7911532F0BBC4D9EBD6F3B0321C1FEB2AC8F9BAE5FA.
Extracted host: C:/Users/xs/AppData/Local/f4/qt-host/f2875c0fcc10e926aa4a2f01939890a3dce955e0de8db7078f0432c7eed5cbb4/f4-qt-host.exe.
The worktree launcher is D:/Code/f4-zoin/f4-zoin.exe and was launched directly with --gui=qt. The canonical D:/Code/f4/f4.exe was not replaced or terminated. No profiling observer is installed in the normal host.

Next investigate the GUI creation/GC path in the deployed source: synchronous nearby delegate construction, retirement/release before the nominal preparation timer starts, allocation-triggered GC and event-loop GC phases. Do not claim steady 160 Hz or repeat the earlier inconclusive budget/chunk constants without fresh evidence. No commit or push was made.

Compact logs and source proof are archived in profiler/experiments/upload-policy-rollout-2026-10-18. Windows checks do not substitute for Linux/macOS release gates or hardware coverage.

AI assistance has been used to create this output.
