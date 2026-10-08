# Editor repeated-search measurement — 2026-10-08

Repeated find-next on the supplied file finishes well within 30 ms in the controlled replay.

Source: `P:/[Year 2026]/[2026.05.29] Graduation Ceremony at Music School (at Philharmonic)/files.txt` — 2,280 bytes, 38 `mp4` matches.

The opt-in Go diagnostic opens the file read-only into a resident editor piece table, uses the real `EditorView.Search` asynchronous path for ten complete passes, and exports the first pass as semantic scenes. The source file is not edited. Flags: case insensitive, forward, plain text, whole words off. Repeated searches continue from the end of the current selection. A final miss verifies the end-of-file result dialog.

Warm measurements exclude the first pass and the end-of-file dialog: 342 Go samples and 114 Qt replay samples.

| Stage | Median | p95 | Maximum |
|---|---:|---:|---:|
| Pattern scan | <0.5 ms | <0.5 ms | <0.5 ms |
| Go UI callbacks (setup + selection/result) | <0.5 ms | <0.5 ms | 0.504 ms |
| Actual asynchronous find-next action | 0.501 ms | 1.005 ms | 1.509 ms |
| Document semantic projection after selection | <0.5 ms | 0.501 ms | 1.004 ms |
| Qt result application | 1.558 ms | 3.311 ms | 3.881 ms |
| Qt application + forced render/readback | 13.478 ms | 17.401 ms | 19.207 ms |

The timer resolves roughly 0.5 ms on this system: zero samples mean below that resolution, not zero work. Callback time is already inside asynchronous action time; Qt application is already inside render/readback time. Do not add nested measurements together.

Qt reuses the document scene replay harness with actual editor scenes (`surface.kind == editor`), incremental document/dialog store updates, no progress phase, an offscreen window of 2194×1186 logical pixels, and DPR 1.75. The harness function is named `viewerSearchSceneReplayProfile`, but it applies the supplied scene kind; this run uses editor content. No QML instrumentation connection is active, avoiding profiler event overhead.

All 38 matches reached the expected byte offsets in all ten passes. No quick-search progress frames appeared in the exported pending states or completed match states. The final miss produced the result popup. Go and Qt diagnostics passed.

Limits: this is not a live keyboard-to-screen trace. Input dispatch, production IPC/patch selection and physical display presentation are not timed. The Go file buffer is resident and the editor line index is initialized, matching repeated search after opening this small file rather than cold remote loading. Qt `grabWindow()` includes synchronous rendering/readback cost; it is not a measured production frame deadline. The CPU profile includes scene export, JSON serialization, and diagnostic runtime allocation; its percentages should not be interpreted as search-only latency.

Only opt-in diagnostic test code was added. The running application and its production behavior were unchanged.

## Reproduction artifacts

- [Diagnostic test](D:/Code/f4-zoin/internal/app/editor_search_profile_test.go)
- [Go measurements](D:/Code/f4-zoin/artifacts/editor-search-go-profile.txt)
- [Go CPU profile](D:/Code/f4-zoin/artifacts/editor-search-cpu.pprof)
- [CPU summary](D:/Code/f4-zoin/artifacts/editor-search-cpu-top.txt)
- [Qt measurements](D:/Code/f4-zoin/artifacts/editor-search-qt-profile.txt)

AI assistance has been used to create this output.
