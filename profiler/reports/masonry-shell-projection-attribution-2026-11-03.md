# Masonry shell projection attribution, 2026-11-03

Three GUI samples in the o174 43.75 ms display gap contained the
same generated ShellSceneStore chain. The map's lambda symbols were
resolved through the AOT function pointer table in the exact captured
PE, then matched to the function indices and source comments in the
generated C++.

| MSVC lambda | QML function index | Source |
|---:|---:|---|
| 37 | 74 | ShellSceneStore.qml line 541, `onShellChanged` |
| 25 | 63 | ShellSceneStore.qml line 405, `resetShellProjection` |
| 17 | 37 | ShellSceneStore.qml line 181, `captureShellSurface` |

The typed shell stream calls `SurfaceRegistry::applyShell`, which
publishes `shellChanged` when the bounded shell map differs. The QML
handler immediately captures the shell, clears both presentation and
layout overrides, and emits `sceneReset`. The compact root path can also
publish a shell replacement for the same root operation. These are
separate fan-out points to time. The three sampled stacks are evidence
of the typed projection path, and do not establish duplicate execution
of the compact path or show that this chain accounts for the full gap.

The two later 37.5 ms gaps also contained 9 and 7 samples respectively
inside `F4GalleryBridge::synchronizePanelCatalog`. In the later gap,
4 samples reached `MasonryLayout::prepareDelegateRow`. These remain
distinct from the shell projection path.
