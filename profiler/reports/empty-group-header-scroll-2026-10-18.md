# Empty group-header scrolling notifications — 2026-10-18

## Finding and production change

`MasonryLayout::setContentYInternal()` emitted `groupHeaderGeometriesChanged` on every content offset change, including ungrouped folders. The QML Repeater then read and wrapped an empty `QVariantList`. The notification is now conditional on legacy group ranges or active group descriptors. Layout rebuild/removal notifications remain intact, including the final notification needed to remove existing header delegates.

This changes no visual layout and adds no pixel rounding, scene traversal, or per-frame geometry comparison.

## Regression evidence

`scrollingNotifiesOnlyExistingGroupHeaders` covers Details, Columns, Grid, Icons and Masonry, each with no groups, legacy ranges, and group descriptors (15 rows). Eight synchronous scroll steps must publish eight content notifications, zero header geometry notifications without groups, and eight with groups. Removing groups must notify and empty the corresponding header model; subsequent scrolling must stop publishing empty header notifications.

- Before: `artifacts/group-header-baseline-175.txt`: 15 failing rows. Ungrouped rows observed 8 notifications instead of 0; grouped rows passed their scrolling check and failed the post-removal empty-notification check.
- After D3D11 at DPR 1.75: `artifacts/group-header-after-d3d11-175.txt`: 19 passed, 0 failed, including existing grouped-collapse and scrollbar regressions plus init/cleanup.
- After OpenGL at DPR 1.75: `artifacts/group-header-after-opengl-175.txt`: 18 passed, 0 failed, including existing grouped-collapse coverage plus init/cleanup.
- Import isolation: `artifacts/group-header-import-isolation.txt`: 3 passed, 0 failed.
- Embedded payload/materialization checks: `artifacts/group-header-embedded-tests.txt`: passed.
- Windows system-DLL audit passed for the rebuilt normal host.

## Live allocation verification

Current capture: `profiler/traces/group-header-qml-memory-2026-10-18.qtd` (20,430,855 bytes, complete XML, profiler client exit 0). Actual wheel input every 30 ms, 268 wheel events, Downloads with 184 entries, Masonry density 152, active window, DPR 1.75, 160 Hz screen, default 5,000 us GC budget.

Filtered sustained scroll clusters: 10,858,869,100–13,909,746,400 ns and 15,352,961,400–19,491,103,600 ns. These are a content-offset activity proxy, not exact QPC frame windows; startup and stationary tails are excluded. See `artifacts/group-header-qml-scroll-windows.json`.

The prior comparable scrolling trace evaluated `GalleryGroupHeaders.qml:12` 1,093 times. Current capture evaluates it 27 times (97.5% fewer); layout rebuild notifications still remain. Temporal small-allocation attribution for the same source expression falls from 2,186 events / 279,808 bytes to 54 / 6,912 bytes. This attribution is a proxy based on active QML ranges, not an allocator stack. See `artifacts/group-header-qml-verification.json` and `artifacts/group-header-qml-allocation-sites.json`.

The earlier trace used the experimental combined renderer, while this capture uses the normal renderer and includes the prior hidden-scrollbar fix. Therefore these captures do not establish an isolated frame-time or global allocation improvement. The dedicated notification regression establishes the causal behavior change. No claim of steady 160 Hz follows from an instrumented trace.

## Remaining work

The current allocation trace still identifies `iconPixelOffset` (3,969 events / 468,320 bytes) and viewport cursor intersection checks (2,232 / 214,272) as prominent temporal allocation sites. Investigate value-wrapper creation and unnecessary reevaluation with regressions; preserve cached final scene-space alignment at rest and disable correction during motion. GC/delegate-creation stalls and renderer validation remain open. Constant 160 Hz without spikes is not achieved.
