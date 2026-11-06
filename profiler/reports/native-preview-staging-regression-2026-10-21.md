# Native preview staging regression — 2026-10-21

## Status

The 160 Hz goal is unfinished. This investigation establishes a correctness-tested candidate, not a measured performance improvement. Production source and the packaged normal app were not changed in this experiment. No commit or push was made.

## Reproduction and mechanism

The existing `GalleryComponentHost` masks an incubating root using `QQuickItemPrivate::setCulled`. Qt converts this hide reference into a scene graph opacity change. The preceding live renderer probe identified existing-node DirtyOpacity as the first full-rebuild cause in rapid scrolling. A native fixture now reproduces the actual host lifecycle: keep the old red surface visible, pause asynchronous incubation after the new root attaches, render while pending, finish incubation, and publish the green surface.

Against unchanged production sources, the fixture records one opacity-triggered rebuild when publishing a native scene root. Four test cases pass; the native publication regression fails as expected. The generic-root and cancellation/reentrant-close cases pass.

## Candidate under artifacts only

`artifacts/native-staging/GallerySceneItem-after.{h,cpp}` adds an internal publication mask to the existing native subtree gate. Pending content remains blocked even when viewport motion or prepared-scene retention would otherwise expose it. `GalleryComponentHost-after.cpp` selects that path for `GallerySceneItem` roots; generic roots and software rendering retain Qt's existing hide-reference path. Authored opacity, visibility, enabled state, direct parent bindings, and host publication guards remain intact.

No per-frame ancestor lookup, opacity suppression in Qt, cache increase, raw-pointer owner cache, or pixel-grid migration was introduced.

## Verified correctness

- Expanded fixture: five cases pass, zero fail/skip on Direct3D 11, OpenGL, and software rendering, all at actual window DPR 1.75.
- Native publication: zero opacity-triggered rebuilds on the candidate, versus one on the unchanged baseline in Direct3D.
- Old surface remains visible during partial incubation; new surface replaces it on completion.
- Cancellation clears outstanding incubation; replacement, unsupported roots, authored properties, and deletion from a loaded handler are exercised.
- The actual colored visual leaf maps to physical origin (14,14), with unit scene vectors and no inherited scale/rotation.
- Six initial/pending/ready native/generic Direct3D screenshots compare byte-for-byte after RGBA conversion between baseline and candidate. See `artifacts/native-staging/pixel-comparison.json`.
- Separate source-ownership diagnostic passes four embedded import tests and the Windows system-DLL audit.

The fixture has no text or icon leaves. Actual factory/icon and scene-motion interaction regressions are still required before production integration.

## Real-panel source attribution (structural evidence only)

A fresh ownership probe records QML context base URL and declared property names without evaluating QML properties. Both runs locate owners in `qrc:/ZoinGallery/qml/GalleryEntryVisualFactories.qml` with declared properties `preview`, `fallbackIcon`, `sourceStatus`. This uniquely matches the `fallbackSourceColor` component's plain `Item` root in that file. This root would need to use the existing native scene gate for the candidate to help it.

The first run maps 16 such owners and one Details extension text owner. The second maps 16 fallback-source-color owners. All recorded wheel inputs report the diagnostic window active. However, only 1263/1265 events were delivered, below the unchanged rapid-workload requirement of 1500. Both runs are rejected as timing evidence; no speed claim is derived from them. A prior progress message inferred focus loss from foreground transitions; the actual input records instead show insufficient input count. No timing gate was relaxed.

See `artifacts/rebuild-owner-source/ownership-findings.json`, plus original raw JSON and build receipts. Ownership traversal is explicitly marked `performanceEvidence:false` and must not be used to judge frame times.

## Remaining work

1. Extend motion/retention and actual preview-leaf regressions before changing production.
2. Integrate the native publication path and the specifically identified factory root, then rebuild the complete core and host; do not override objects across an unverified class-layout change.
3. Compare first full-rebuild causes and upload bursts against an equivalent reference host. Include all causes, not just opacity, to detect displaced work.
4. Run minimal production-source rapid wheel and quarter-height middle scrolling, cold and warm, with unchanged 160 Hz/DPR/focus/motion gates.
5. Investigate remaining cold SVG geometry upload work and other tails independently. This mask alone does not establish constant 160 Hz.
6. Only after validation, run portable/import/extraction gates and replace the worktree's normal app.
