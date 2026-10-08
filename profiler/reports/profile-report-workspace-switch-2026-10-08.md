# Live workspace switching profile — 2026-10-08

The reported delay is reproduced in the actual application: returning to tab 1 costs about 331 ms; switching to tab 2 costs about 238 ms.

## Run

24 alternating workspace.activate actions at one-second intervals, following eight seconds of startup settling. The first four switches are excluded from warm statistics, leaving ten samples per direction. All three restored workspaces remained present. The actual portable Qt frontend rendered a normal window at the configured fractional scale; this is not a mocked QML scene replay.

Tab 1: graduation folder on P: in masonry mode, C: on the other side. Tab 2: CozyPets dachshund download directory and generated orange-cat video directory in details mode. Tab 3 remained untouched. The session contained Count=3 and Active=0 after capture.

The automated runner sends the same typed workspace.activate action as the tab control through real Qt → Go IPC. It bypasses physical mouse dispatch. The Windows computer-use helper could not initialize even after retry/reset, so current tab identities were taken from the saved session and checked against the restored three-tab state. This run restarted the application and cannot preserve unsaved, unserialized UI state from the pre-restart instance.

## Warm measurements

| Destination | Complete frame median | p95 | Maximum | Go activation median | Go input queue median |
|---|---:|---:|---:|---:|---:|
| Tab 1 | 331.222 ms | 346.988 ms | 364.144 ms | 0.044 ms | 0.825 ms |
| Tab 2 | 238.250 ms | 243.025 ms | 244.164 ms | 0.046 ms | 0.808 ms |

Complete frame means the first qt.frame.end timestamp after menu, both panel-catalog, and shell snapshot application has completed. Earlier frames can show partial state and are excluded. This measures Qt frame completion, not photon/display latency or all subsequently loaded thumbnails. Snapshots lack the action trace marker, so response phases are associated temporally within each one-second action interval. Each interval contained its expected activation snapshots.

| Qt work per switch | Tab 1 median | Tab 2 median |
|---|---:|---:|
| menus_snapshot | 46.608 ms | 45.858 ms |
| panel_catalog_snapshot | 78.913 ms | 89.482 ms |
| shell_snapshot | 162.330 ms | 74.762 ms |

These are accumulated Qt message-application durations within each interval; later scene patches/metadata updates are additional work. Panel-catalog time combines the two panels. The dominant measured cost is synchronous Qt state application and the QML reactions it triggers, rather than the Go index switch or action transmission.

## Cause supported by the trace

Every activation crosses panel identities. The Go incremental presentation path reports panel_id_changed, followed by invalid_patch and full snapshots. Across 24 switches, 48 panel_id_changed rejections were recorded, two attempts per switch. The payloads then include menus_snapshot, two panel_catalog_snapshot messages, and shell_snapshot. Qt applies them synchronously and rebuilds the visible panel/shell state.

[Go incremental rejection](D:/Code/f4-zoin/internal/plughost/extui.go:2305) and [workspace activation](D:/Code/f4-zoin/third_party/vtui/framemanager.go:620) explain the boundary. [Qt snapshot application](D:/Code/f4-zoin/qt/host/src/QtShellControllerSceneFrames.cpp:180) applies the new typed stores. [Application menu Instantiator](D:/Code/f4-zoin/qt/host/qml/ApplicationMenuPopup.qml:123) builds category and entry delegates when menu models are replaced. The trace establishes whole message costs; it does not independently quantify individual bindings or delegate creation inside those costs.

The first measured switch to tab 2 took 269.744 ms. It included menu application 50.953 ms, two catalog applications totalling 120.873 ms, and shell application 74.636 ms. The same work repeats in warmed switches, demonstrating that startup alone does not account for the delay.

## Next implementation targets

1. Retain per-workspace native panel/shell presentation and activate retained state instead of republishing and rebuilding it on every tab change. Keep catalog identity validation when applying actual deltas.
2. Avoid replacing equivalent application menu models during panel-to-panel tab activation; retain menu delegates and update only changing state.
3. Profile QML bindings and native gallery layout inside shell/catalog snapshot application before attempting smaller optimizations. Current trace does not establish thumbnail decoding or disk reads as the principal cause.

## Validation and limitations

Native diagnostic build, Windows system-DLL import audit, embedded QML import regression, Go navigation trace tests, and portable Go build passed. CPU capture includes startup/shutdown and Windows blocking calls; its large runtime.cgocall percentage is dominated by PTY read/wait stacks and is not evidence that those waits cause the UI delay. Timeline phase measurements are the useful latency evidence.

The opt-in runner exits after capture. The app was relaunched directly with --gui=qt, without tracing/profile environment variables. No performance fix was applied. Opt-in diagnostic hooks remain available for further investigation.

## Artifacts

- [artifacts/workspace-profile-summary.json](D:/Code/f4-zoin/artifacts/workspace-profile-summary.json)
- [artifacts/workspace-profile-go.jsonl](D:/Code/f4-zoin/artifacts/workspace-profile-go.jsonl)
- [artifacts/workspace-profile-qt.jsonl](D:/Code/f4-zoin/artifacts/workspace-profile-qt.jsonl)
- [artifacts/workspace-profile-cpu.pprof](D:/Code/f4-zoin/artifacts/workspace-profile-cpu.pprof)
- [artifacts/workspace-profile-cpu-top.txt](D:/Code/f4-zoin/artifacts/workspace-profile-cpu-top.txt)

AI assistance has been used to create this output.
