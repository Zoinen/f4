# Bounded requested-row construction prototype — 2026-11-01

This turn builds and tests the lifecycle machinery needed to spread actual row construction across GUI frames. It is an isolated prototype, not enabled in Masonry or the normal application. It establishes neither a scrolling speedup nor constant 160 Hz.

The preceding parent-at-construction experiment still leaves approximately 39–51 ms cold-entry commits and refresh misses. Its findings motivate replacing the synchronous viewport-wide creation burst; simply changing a delegate's initial parent did not remove the work. See [the preceding experiment](masonry-parent-at-construction-2026-11-01.md).

## Implemented prototype

`RequestedRowIncubation` uses asynchronous `QQmlIncubator` instances for requested row identities and their initial property maps. A caller supplies the current generation, delegate component and ordered row plan. Requests deduplicate by row and identity. A changed generation/component, replacement identity, removal from the plan, destroyed component/viewport or change of window cancels obsolete work. The GUI `afterAnimating` connection follows a viewport to its replacement window.

The default bound is two constructing **or ready-but-unclaimed** roots per owner. Starting a frame admits at most two roots, rather than draining a large row loop. Queued requests do not create anonymous spare delegates. Errors are recorded for that identity until replacement and do not repeatedly retry in the frame loop. Cancellation clears the incubator and destroys any surviving rejected root.

Initial properties are installed before creation. The viewport parent and window exist at component completion. A `GalleryDelegateItem` is masked with `setIncubationReady(false)` before it joins the visual tree. Taking a finished object does not lift that mask. The future layout caller must apply its **current** row identity, visual snapshot and geometry, and only then explicitly publish it. Ready notifications are queued and coalesced; a changed generation invalidates old notification delivery. Deleting the owner from a ready callback is covered.

This is a standalone QObject child rather than a new MasonryLayout data member. It does not change the layout's ABI while linking against the existing diagnostic static dependency graph.

## Verification

The final static Qt 6.11.1 test executable has SHA-256 `a631745f9debedec384aa6209aef072b29029221849e777f3e318d199972fb94`. It was linked through `ci/msvc_link_response.py` using the existing foreground-entry host dependency graph with a new test entry object. No production payload was regenerated.

Nine focused cases pass in each of three final runs (11 QtTest results including setup/cleanup):

| Run | Renderer / imports | Failures / skips |
| --- | --- | --- |
| test-run-7-software | Offscreen software, DPR 1.75 | 0 / 0 |
| test-run-8-native | Native Windows renderer, actual DPR 1.75 | 0 / 0 |
| test-run-9-isolated-native | Native Windows, engine imports restricted to qrc resources, actual DPR 1.75 | 0 / 0 |

The cases cover bounded priority and publication, replacement while suspended, stale notification rejection, cancellation outside a reprioritized plan, a non-visual root and missing required property, owner destruction from notification, component/viewport destruction, destruction of an unclaimed ready root on revision change, automatic frame pumping after window replacement, and static/rendered publication. Some cases exercise multiple conditions.

The publication case sets the unclaimed root visible **before** lifting its incubation mask. The captured window remains black. After explicit publication, it contains the requested small native-text caption, blue rectangle and red raster image. Both named visual leaves are mapped through their full ancestor chain into contentItem at DPR 1.75: caption origin (8,12) physical pixels, raster origin (12,48). Both local unit vectors remain scene unit vectors. The native rendered output was inspected, including the 13-logical-pixel caption. This covers the prototype fixture, not all application panel leaves.

Initial harness failures are retained in test-run-1: components using an unprepared `inline:` base URL were still Loading when the synchronous readiness assertion ran. The harness now uses resource-local base URLs and waits for actual component readiness. Those six initial failures are not application regressions. Earlier exploratory runs precede the final window-replacement test and should not be treated as the final version. The final build receipt revalidates all seven hashed source/object/executable/response inputs against the three final runs.

The restricted-import run covers the modules used by this fixture. It does not replace the complete host's `F4QuickViewSurfaceTests qmlImportsWithoutInstalledQt`, system-DLL audit, packaged embedded-payload launch or other release gates.

## Still required before a performance claim

1. Connect the prototype to Masonry's actual `createComponent` / pool-acquisition / finalization path. An incomplete root must remain absent from `brick.item` and active/coverage indexes. Cancel a pending request when an already-free pool slot satisfies its row.
2. Supply the actual visible/target-leading rows before metadata-only look-ahead, within the existing viewport limits. Reconcile model reset/removal, mode changes, source revisions and shutdown before old models are released. The identity key must include the immutable source revision where needed. The prototype itself is not yet connected to those model signals.
3. Gate integration on an existing incubation controller. Test the actual GalleryViewport delegate and its nested hosts. The fixture supplies a manual controller; it does not prove a latency bound for arbitrary QML construction or unsupported controller-less operation.
4. Test normal finalization, free-slot reuse, target coverage, rapid direction changes, folder reset and mixed SVG/photo content. Verify the complete affected static leaves at 175%, with rendered output, rather than extending the fixture's result to real panels.
5. Re-run foreground-validated cold entry and rapid input with navigation-to-first-input work retained. Also exercise input before all initial rows finish: delaying benchmark readiness must not conceal construction work or missing coverage. Then repeat the full-folder warm tour and quarter-height middle scrolling.

No new PresentMon capture was made this turn because the prototype is not yet integrated. No cadence, acquisition, coverage or physical-motion error measurement is inferred from the small lifecycle test. The existing wheel analyzer still excludes wheel-curve pairs and admits no physical-error samples, so those older captures cannot prove the full motion requirement either.

The normal worktree application pair (Go PID 9252, Qt PID 106980) was verified alive after these tests. Product source, runtime configuration, embedded host and pixel-grid behavior were not replaced.

## Evidence

The source, test fixture and build scripts are frozen beside [the experiment manifest](../experiments/masonry-requested-row-incubation-2026-11-01/manifest.json). Raw test XML, rendered windows, leaf coordinates and build inputs remain under `D:/Code/f4-zoin/artifacts/requested-row-incubation-20261101`. The manifest binds the final source snapshots and evidence to their SHA-256 hashes. Executables remain outside the versioned experiment folder.

> AI assistance has been used to create this output.
