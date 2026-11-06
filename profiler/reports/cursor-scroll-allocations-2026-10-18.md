# Cursor scrolling allocations — 2026-10-18

## Production changes

`GalleryCursorController.indexIntersectsViewport` previously fetched a QRectF from C++ into QML, read several geometry/layout properties, and returned a boolean on every scroll update. `MasonryLayout::indexIntersectsViewport(int)` now owns that predicate and returns a primitive boolean. It preserves invalid-index/empty-geometry handling, zero-height rejection, the Columns horizontal axis, and strict positive painted-area intersection (edge contact alone is invisible).

Ordinary wheel updates also checked visibility and assigned the visual cursor even when it already equaled the authoritative target. The ordinary-scroll branch now tests identity first. Pending keyboard reveals, anchored cursor handoff, and nearest-visible replacement retain their checks. No geometry, visual layout, pixel rounding, or renderer policy changed.

All current callers are imperative handlers/functions; no property binding depends on implicit reads inside the replaced QML predicate.

## Regressions and verification

`nativeViewportIntersectionMatchesCursorRules` covers all five modes, 200 cells at five offsets around an interior cell: exactly touching either edge, barely intersecting either edge, and fully intersecting. It compares against the independent strict painted-area helper and exercises the QML facade invocation. Invalid indices and zero-height rejection are checked.

The first local test draft accidentally used the older navigation helper with a 0.51-logical-pixel tolerance; it correctly disagreed at touching edges. The final test uses the existing strict painted-area helper matching the original QML contract. No production behavior was changed to satisfy that draft.

- D3D11, DPR 1.75: `artifacts/native-intersection-final-d3d11-175.txt`: 14 passed, 0 failed. Includes five native predicate rows, cursor reveal, resting cursor pixel grid, and wheel burst rows for all five modes, plus init/cleanup.
- OpenGL, DPR 1.75: `artifacts/native-intersection-final-opengl-175.txt`: 9 passed, 0 failed. Includes five native predicate rows, cursor reveal, resting cursor pixel grid, plus init/cleanup.
- Import isolation: `artifacts/native-intersection-import-isolation.txt`: 3 passed, 0 failed.
- Native static DLL audit passed: `artifacts/native-intersection-final-build.log`.
- Embedded payload/materialization tests passed: `artifacts/cursor-allocation-embedded-tests.txt`.

## Comparable QML captures

Before: `group-header-qml-memory-2026-10-18.qtd`. After: `cursor-allocation-qml-memory-2026-10-18.qtd` (19,708,243 bytes, complete XML, profiler client exit 0). Both use the normal renderer, Downloads (184 entries), active window, DPR 1.75, 160 Hz screen, default 5,000 us GC budget, and 268 actual wheel events at 30 ms intervals. Startup and stationary tails are excluded using sustained contentY-change clusters; this is an activity proxy, not exact QPC synchronization.

| Captured scrolling range | Before | After |
| --- | ---: | ---: |
| updateVisualCursorForViewport calls | 1,116 | 1,103 |
| Inclusive range duration | 12.4817 ms | 2.9480 ms |
| indexIntersectsViewport calls | 1,116 | 0 |
| Temporal small-allocation proxy for intersection helper | 2,232 events / 214,272 bytes | no attributed events |

Data: `artifacts/cursor-allocation-qml-comparison.json`. Allocation attribution assigns events to active QML ranges; it is not a C++ allocator stack. The native predicate is still available when cursor identities differ or keyboard reveal needs it. Zero calls above applies to this ordinary wheel workload, not all navigation.

Do not infer a frame-time guarantee from these instrumented captures or sum overlapping ranges. The observed captures still contain frame spikes. No clean uninstrumented cadence gain or constant 160 Hz is claimed.

## Next target

Icon offset point wrappers remain prominent (about 468 KB in the preceding scrolling capture). Investigate scalar/cached access without adding repeated work or disturbing final scene-space alignment. GC/delegate-creation stalls and experimental renderer validation remain open.
