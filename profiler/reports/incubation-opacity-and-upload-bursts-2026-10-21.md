# Incubation opacity and upload bursts

Date: 2026-10-21. Worktree `D:/Code/f4-zoin`, branch
`zoin_branch/cached-pixel-grid`. Goal: constant 160 Hz without spikes.
**Not achieved. No new production change was made in this investigation.**

## Same-capture rebuild attribution

A reversible diagnostic records the ten assignments to FullRebuild in the
current Qt batch-renderer source. It records reason, prior rebuild flags,
renderer/node identity and bounded live ancestry. Removal sites record addresses
without dereferencing their QSG objects. A seven-case QtTest result (including
initialization/cleanup) passed: live ancestry, non-dereferenced invalid removal
address, metadata surviving node destruction, reused/null slot cleanup, and
bounded overflow. Four import-isolation tests and the static DLL audit passed.

The same executable also records the existing twelve preparation phases and
actual geometry uploads, with the upload capacity raised to 524288 to avoid the
previous rapid-scroll overflow. No invalidation-material observer is included.
The cold upload reservation was 164719 and the warm cumulative reservation
324223, both below capacity. Input validation accepted 160 Hz, actual DPR1.75,
Downloads, 187 entries, density152, 10 ms wheel repetition, and focus checks.
No builds, offline analysis or UI Automation polling ran during input.

Every FullRebuild first transition inside the rapid-scroll measurement windows
came from reason8: the existing-node DirtyOpacity path in Renderer::nodeChanged.
The cold blocks had 11 and13 first transitions; warm had8 and8. Other recorded
sites—root funding/removal, render-node depth behavior, opacity-class changes,
and material blending—did not cause an in-window first transition in this run.
This is a result for these input windows, not a universal claim about all scenes.

## Consequence for uploads

Upload records were matched to the upload/admission span of the same renderer
preparation pass. These values count actual uploaded batches, not file decodes.

| Stage / frame group | Frames | Median uploaded batches | Median upload span ms |
| --- | ---: | ---: | ---: |
| Cold full rebuild | 24 | 197.5 | 0.9218 |
| Cold other | 3177 | 2 | 0.0170 |
| Warm full rebuild | 16 | 199.5 | 0.91735 |
| Warm other | 3185 | 2 | 0.0164 |

The warm spike is a large number of small uploads, rather than repeated loading
of one large SVG buffer. For example, the slowest warm block0 frame contained
253 uploaded batches: its upload span was1.0871 ms and the recorded individual
uploads summed to1.0178 ms. Recorded calls included ordinary vertex-color, shader,
and text geometry. Full rebuilds coincide with the small-batch upload burst;
a targeted staging experiment must establish the counterfactual benefit.

Cold pending vector work is distinct. In the slowest cold block0 frame, total
preparation was5.6190 ms and uploads/admission used2.0266 ms, with1.9878 ms covered
by101 recorded upload calls. The two longest individual calls were pending-route
QSGCurveFillMaterial batches:675504 bytes /0.7058 ms and179712 bytes /0.5805 ms.
Byte totals are requested VBO/IBO bytes, not pooled buffer capacity. These are
wall times; no GPU-wait or scheduler classification is inferred from this trace.

## Exact item ownership

An ownership-only diagnostic maps newly observed opacity-node addresses under
QQuickWindow::afterSynchronizing while GUI items are locked. It scans items only
when qualifying events arrive, never evaluates arbitrary QML properties on the
render thread, and uses fixed-capacity output. Its timings are explicitly marked
`performanceEvidence:false` and must not be compared to minimal production runs.

A native test at actual DPR1.75 verified exact opacity-node to item matching,
public opacity, class/name capture, leaf origin/unit vectors and rendered content:
three QtTest cases passed including initialization/cleanup. Its host passed four
import tests and the dependency audit.

All81 sampled first-transition events in the accepted ownership run mapped:
43 cold,38 warm. Their owner was a process-local anonymous QQuickItem composite
(`QQuickItem_QML_412`), public opacity1.0, directly under GalleryPreviewVisualHost,
then GalleryEntryPreview and GalleryEntryDelegateBase in the Masonry raster or
vector branch. The generated numeric type suffix is not an exact source filename
and must not be hard-coded. No event was attributed by guessing the nearest item.

GalleryComponentHost currently stages an attached incubating item with
QQuickItemPrivate::setCulled(true), then unculls the candidate and culls the old
item during publication. Qt6.11.1 [setCulled](https://github.com/qt/qtdeclarative/blob/v6.11.1/src/quick/items/qquickitem.cpp#L6559)
changes hideRefCount. The [window synchronization code](https://github.com/qt/qtdeclarative/blob/v6.11.1/src/quick/items/qquickwindow.cpp#L2179)
applies that count through a QSGOpacityNode even when public item opacity stays1.
Thus this host staging path is the specific implementation target identified by
the observed private-opacity transitions. A native before/after regression is
still required to verify a replacement and its performance counterfactual.

## Next implementation and invariants

Test native scene-subtree staging for GallerySceneItem-derived renderer roots.
Keep direct visual-parent semantics, authored visible/opacity bindings, fit/resize,
ready-item retention, cancellation, error handling and reentrant destruction.
Keep the generic-root/software fallback. Handle a window/backend change while
incubation is pending. Do not globally suppress FullRebuild or alter material
ordering. No candidate implementation has been applied yet.

`artifacts/NEXT_NATIVE_STAGING.md` records the proposed native regression and
prototype boundaries. Changing a plain factory Item into a GallerySceneItem must
retain actual leaf placement/transform and rendered output at175% scale. A live
object override additionally requires proof of class-size/layout compatibility;
otherwise rebuild the whole core.

After correctness, capture without the ownership mapper to verify that total
full rebuilds and small upload bursts fall, then recheck minimal cold/warm rapid
and middle runs with all existing motion, display, coverage and focus gates.
Cold pending-vector admission/upload bursts remain separate unfinished work.
The goal cannot be completed from one rebuild counter or average frame rate.

## Evidence

- `real-scroll-rebuild-upload-rapid-20261021-{cold,warm}-wheel-masonry`.
- `real-scroll-rebuild-owners-rapid-20261021-{cold,warm}-wheel-masonry`.
- `rebuild-upload-{preparation-analysis,top-frames,frame-groups}.json`.
- `rebuild-upload-build-receipt.json` and `rebuild-owner-build-receipt.json`.
- `rebuild-upload-probe/RebuildTrace.h`, native test and its passing result.
- `rebuild-owners/RebuildOwners.h`, native test and its passing result.

Build/source/analysis snapshots and local raw-file hashes are under
`profiler/experiments/incubation-opacity-and-upload-bursts-2026-10-21`.
Raw traces, binaries and object files are not intended for commits. No commit or
push was performed. The normal worktree app remains on host SHA256
`c0133127a7060938dc42e0b7906eec2b6cb316855e865027539a7d088a79ffac`.

AI assistance has been used to create this output.
