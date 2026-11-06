# Shared entry visual factories — 2026-10-18

## Production change and ownership

The preceding turn measured and removed hidden diagnostic border objects.
This turn classifies retained QQmlComponents by owner and source URL, then
removes six duplicated factory objects per row. `GalleryEntryVisualFactories`
owns the five mode factories and folder-preview factory once per GalleryPanel.
Each `GalleryVisualHost` still owns its own visual instance and supplies its
typed row explicitly. Shared components do not capture a particular row's
creation context. Panel deletion owns factory cleanup; loader activity,
compatible visual reuse, folder-frame retention and visual geometry remain
unchanged. No rendering layer, scale transform or additional rounding is added.

An initial double QML cast triggers invalid generated C++ in Qt 6.11.1. A typed
row property on the host removes the second cast; the final build retains AOT
compilation. Qt is not upgraded and compilation is not disabled. The last working
application remains alive during the failed build; no failed executable is used.

New QML files are `GalleryVisualHost.qml` and `GalleryEntryVisualFactories.qml`,
registered in the existing static module. `GalleryPanel.entryVisualFactories`
is a read-only alias to its factory object. GalleryEntryDelegateBase's loaders
select shared components and pass their own row, rather than defining six local
components. Existing mode and folder renderer implementations are retained.

## Factory evidence

The native owner census runs only after frame capture ends. The before snapshot
contains 217 row bases: 1085 components parented to row bases and another 217
folder factories, all 1302 carrying GalleryEntryDelegateBase.qml as their URL.
GalleryEntryPreview separately owns 1519 components, seven per row.

The after snapshot has zero components with the row-base URL and 24 components
owned by four panel factory objects. A controlled regression also asserts no
direct QQmlComponent children per row, identity of the selected shared component,
separate visual instances with correct row pointers across all five modes,
row metadata replacement, unaffected sibling instances, and correct factory
lifetime after row and panel destruction. The old implementation fails because
the shared factory object is absent; the final regression passes.

The snapshots differ in hardware/workload geometry and materialized row counts
(217 versus 270). Consequently neither total QObject counts nor the total
component-count difference is used as a causal performance result. The strong
result is the six-per-row factories becoming six-per-panel, with explicit
instance ownership. Preview-owned factories remain a large target.

## Rendering and regression checks

- Gallery suite: 35 passed on D3D11 and 35 on OpenGL at 175% scale.
- Mode/folder suite: 24 passed on D3D11, covering first-frame appearance,
  folder-frame reuse, transitions, actual folder pixels, shared padding,
  bottom alignment, all-mode debug leaves, current-item reveal, deterministic
  navigation, wheel motion and resting pixel alignment.
- Import isolation/shader audit: 4 passed on each backend. The isolated host
  test now creates a row and its shared visual with SDK import paths removed.
- Static Windows system-DLL audit passes.
- Embedded generation/materialization/reuse/concurrency/corruption/lookup tests
  pass for the rebuilt portable payload.

Visual inspection catches a weakness in the existing SVG alignment test: a
100 ms wait can save a fallback icon even though a vector surface exists. The
test now waits for source readiness, settled motion and actual orange SVG pixels
before checking leaf coordinates and saving the capture. The stronger five-mode
test passes separately on both backends (7 passes including init/cleanup).
The corrected capture is inspected, including its small caption and thin blue
vector stroke. The former fallback capture is retained as evidence of why the
test changed; this is not presented as an established production SVG failure.

## Display setup and clean comparisons

The first clean A1/B1/B2/A2 set runs on the app's current display, which has
changed to 120 Hz/DPR 2, and Downloads now contains 186 rather than 184 files.
All four captures match each other, but they cannot validate the 160 Hz goal
or be compared with earlier 175% runs. Their displayed medians are about
8.333 ms. These captures are retained separately, not discarded or relabelled.

A native screen inventory confirms the 160 Hz MSI MAG322UPF at DPR 1.75 is
still available, alongside the 120 Hz MAG 272URDF. The existing diagnostic moves
only the worktree window to the requested screen. No global display mode is
changed. A second complete comparison explicitly requests the 160 Hz screen;
every capture verifies active=true, screenHz=160, DPR=1.75, 186 entries, density
152, extent 4101.142857, 30 ms wheel events and the normal 5000 us GC budget.
No profiler, WPR, census scan, tests or builds run during clean measurements.

Strict displayed intervals have both endpoints inside the same direction window.

| 160 Hz run | Down p99 ms | Down max ms | Down >9.375 ms | Up p99 ms | Up max ms | Up >9.375 ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| A1 | 18.7227 | 41.8206 | 43 | 6.3835 | 18.7497 | 4 |
| B1 | 18.7407 | 31.3155 | 33 | 6.3984 | 18.7598 | 3 |
| B2 | 6.3547 | 12.5467 | 4 | 6.3623 | 12.4991 | 3 |
| A2 | 12.4683 | 18.7607 | 7 | 6.3738 | 12.5128 | 5 |

Candidates total 43 long intervals versus 59 in controls. The two pairs differ
substantially; B1 p99 does not improve and both versions retain serious spikes.
No unexplained control/candidate outlier is removed. These short, variable
comparisons do not prove a reliable cadence improvement or constant 160 Hz.
Factory elimination is independently verified; the new large spikes require
current-state CPU/render attribution rather than an assumed explanation.

## Rollout and evidence

The Qt host is rebuilt, its embedded payload regenerated and a CGO_ENABLED=0
launcher built with the system Go cache. Panel state is restored by the live
diagnostic before replacing only this worktree's launcher tree. The normal app
runs directly with --gui=qt, experimental overrides cleared. Its extracted Qt
child matches the normal host; startup stderr is empty. Canonical F4 is untouched.
No commit or push is made. The full goal remains active and unachieved.

- Host SHA256: `63C219076AC13670925EC0F8340FC78708ABA9F53982588FF22F195F069D4D83`
- Launcher SHA256: `90FEBA68760E8AE888B3EA4B343DDBEB8BC135568E6D294C2408C28E02ABF8EC`

`profiler/experiments/shared-entry-factories-2026-10-18/` contains expected-failure
and passing logs, stronger SVG-paint evidence, native owner/screen censuses,
frame JSON and strict summaries for both full comparison sets, scripts, live
deployment identity and a manifest. Raw PresentMon CSVs, binaries and build
outputs remain untracked in artifacts. The existing PIXEL_GRID document has a
non-UTF-8 byte and is left unchanged; the factory ownership contract is recorded
here instead of changing unrelated encoding during this optimization.
