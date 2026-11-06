# Editor Page Down: D:\test.py

Initial read-only controlled replay (historical baseline, before the live investigation). Source size: 263,496 bytes;
7,831 document lines. Go: 360 Page Down operations (three passes, 120 pages each),
300 columns and 48 viewport rows. Qt: 156 incremental document applications,
2194×1186 logical window, DPR 1.75. This initial replay itself changed no production behavior.

| Stage | Median ms | p95 ms | p99 ms | Max ms |
| --- | ---: | ---: | ---: | ---: |
| Editor ProcessKey | <0.5 | 0.498 | 0.500 | 0.695 |
| Full semantic scene export | 2.501 | 4.526 | 6.009 | 8.522 |
| Full scene JSON encoding | 4.498 | 8.002 | 10.288 | 15.084 |
| Go sum of above stages | 7.004 | 12.514 | 15.497 | 21.093 |
| Qt incremental application, Windows D3D11 | 2.369 | 3.826 | 4.522 | 6.053 |
| Qt application + synchronous render/readback, Windows D3D11 | 52.881 | 88.408 | 93.191 | 101.098 |
| Qt incremental application, offscreen | 3.181 | 5.413 | 6.443 | 7.488 |
| Qt application + synchronous render/readback, offscreen | 37.724 | 47.962 | 51.179 | 56.096 |

## Findings

Navigation and cursor/layout lookup are comfortably below 16 ms. The expensive
measured work follows navigation: scene construction, allocation, serialization,
and presentation. A representative exported window contains 144 buffered rows
for 48 visible rows; full scenes are 500–663 KB. The CPU sampling run attributes
37.95% of sampled CPU time to JSON Marshal, 21.05% to full semantic export,
and 31.30% to GC drain (including concurrent workers). These are diagnostic
workload percentages, not production event-loop percentages.

The relevant paths are `internal/editor/view.go` (VK_NEXT),
`internal/editor/qt_semantic.go` (SemanticNode),
`internal/editor/qt_semantic_window_render.go`, and
`qt/host/qml/DocumentWindowPresenter.qml` (window application and row pool).

## Limits and next measurement

This is not a live held-key queue trace. The Go full-scene encoder is deliberately
measured separately; production incremental patching can send less data.
Qt grabWindow forces GPU synchronization/readback and is NOT a physical-display
frame latency measurement. Do not add its duration to Go time to claim a live
keyboard-to-screen latency, or attribute the entire readback time to rendering.
The Windows and offscreen captures must not be treated as equivalent renderers.

The next useful measurement is production input → scene/patch export → transport
→ Qt row commit → frame presentation, using existing navigation trace hooks,
without grabWindow or UI Automation polling during the timed interval. That is
required to distinguish actual repeat backlog from the costs exposed here.
Optimization should target excess scene/row work only after that attribution;
dropping keyboard autorepeats would mask the symptom.

## Reproduction

Opt-in diagnostic: `internal/app/editor_page_profile_test.go`.
Set F4_EDITOR_PAGE_PROFILE_FILE to D:\test.py; optionally set
F4_EDITOR_PAGE_PROFILE_DIR to a temporary directory for Qt replay scenes.
Run TestEditorPageDownProfile with GOWORK=off and CGO_ENABLED=0.
Qt uses F4OperationsQueueTests::viewerSearchSceneReplayProfile with
F4_SEARCH_REPLAY_INCREMENTAL=1 and F4_SEARCH_REPLAY_SKIP_PROGRESS=1.
The harness name is historical: its scenes in this run are editor pages.
Raw logs and CPU profile are in the Windows temporary directory with prefix
f4-editor-page-. Source contents are not included in this report.

## Live editor investigation

The replay above did not reproduce the full problem. Two early captures were
incorrectly made in panels and were discarded. Subsequent captures explicitly
verified the visible `documentHeaderLeft` equals `D:\test.py`. The production
editor reports 302 columns and 53 visible rows, with 159 buffered data rows.

The live diagnostic opens that file without modifying it, resets to Ctrl+Home,
and sends 100 Page Downs at 16 ms intervals. The latest driver posts real Qt
key events, including auto-repeat, through the window's normal key route. The
expected final cursor is line 5301, viewport start 5248. UI Automation is not
polled during the timed repeat interval. A two-second settling interval separates
initial file/viewport negotiation from the repeat measurement.

The first verified editor/controller-input capture (`editor-live3`) showed Go
render/send p95 129.26 ms and input queue wait up to 3850.78 ms. This baseline
forwarded keys to the controller; later native-key captures include the additional
Qt key route, so they are not identical input harnesses. Do not describe these
numbers as physical-display latency or exact before/after frame rates.

### Confirmed costs and fixes

- Chroma already ignores previous-line parser state and returns no state, but
  the editor treated it as stateful. An explicit optional state-independent
  contract avoids the unnecessary whole-document state indexing pass.
- CPU samples in the real editor attributed 1.18 s of 1.39 s of styled row work
  to Chroma highlighting. A bounded token LRU (2048 lines, estimated 4 MiB per
  highlighter) reuses lexical tokens while resolving current theme attributes
  afresh. Regression coverage checks eviction, returned-attribute ownership,
  and theme changes without re-lexing.
- Socket backpressure caused large Go send stalls. One-MiB socket buffers and
  Qt TCP low-delay removed those large stalls; a single combined frame write
  did not improve results and was reverted.
- Native row payloads need not cross into QML and back to populate the C++ row
  store. The native store now installs the published payload directly, while
  negotiated editor layout consumes a compact array of absolute row extents.
- Ordered extent lookup was linear, with nested scans during overlap matching.
  Binary search and ordered overlap lookup bound this work.
- A 128-token row performed 8256 indexed reads while positioning tokens. Qt's
  native Row positioner reduces the regression to 128 reads with the original
  split-renderer styling and physical-pixel checks preserved.
- Production rows contain a window-relative `index`. Original fixtures omitted
  it, hiding false invalidations of unchanged overlapping rows. Adding realistic
  indexes reproduced 91 slot writes where only 2 were required. Native row
  comparison now excludes nonpainted index/hash hints while still comparing
  authoritative nested styles. Stale content keys cannot suppress real changes.
- Inactive editor slots retain their payload for reuse while remaining hidden.
  Model notifications identify the roles that actually changed.
- A live QML profile attributed 918 ms / 25296 evaluations to the token repeater
  model binding. Stable scalar document-kind and background properties avoid
  invalidations from unrelated cursor/status metadata.
- Negotiated ASCII editor rows with ordinary backgrounds can use one escaped
  StyledText surface per row. Invisible trailing padding is omitted only from
  that rendering; source, cursor and selection data remain complete. Explicit
  backgrounds and complex/control glyph cases retain the existing renderer.
  The fast path preserves the configured font (unlike the rejected pre tag).
- Reapplying the same fractional anchor emitted 40 contentY changes for 20
  placements. Placement of an already prepared editor row is now idempotent.
- Pool wraparound replaced 120 slots in a 20-row page-step fixture instead of
  updating 40 edge slots. Moving the native model block preserves delegates and
  persistent indexes while keeping the pool size bounded.

### Rejected experiments and measurement limits

Increasing ListView cacheBuffer to an entire adjacent page increased synchronous
work after jumps and was reverted. Eager JS snapshots of every native row field
and JS offset dependency chains did not provide a useful live improvement and
were removed. No keyboard repeats are discarded or compressed by these fixes.

The QML profiler trace (`editor-qml-live.qtd`, temporary directory) is for hotspot
attribution; profiling overhead makes its durations unsuitable for the final
unprofiled throughput claim. Raw traces and exported source-bearing scenes stay
outside the repository. The diagnostic driver and opt-in replay harness document
reproduction; production builds must not include the temporary driver hook or
QML debugging services.

## Follow-up: pointer source anchors after paging

The compact native editor window exposes numeric visual-row coordinates to
QML. The pointer controller still treated these numbers as full row maps and
defaulted their missing byte offsets to zero. This caused clicks, including
clicks reactivating the window, to move the cursor to the first source line.

The production-mode mouse regression failed before the fix with actual offset
0 versus expected 537. Mouse actions now obtain the row from the committed
native pool slot; unavailable anchors are rejected instead of becoming offset
zero. Legacy-map, compact-native and compact-native paged-window cases pass.
The fix does not restore per-page row-map conversion or change paging work.

The temporary auto-replay application subclass and first-open Ctrl+Home driver
were removed from the production entry point, along with the diagnostic row
comparison logger. The driver could separately reset D:\test.py on first open.

Live verification used the rebuilt portable worktree executable with its normal
profile and extracted embedded host (host SHA-256
811c19d8b39e3d2c6bb8be1fef88653c3637683b0de60852e28a3c5c579af79b).
Opening D:\test.py and pressing Page Down twice reached line 107. After activating
another application, clicking the exposed editor placed the cursor at line 93,
column 20 under the click without changing the viewport or jumping to line 1.
Native Windows 175% leaf alignment tests passed, as did QML import isolation,
GUI subsystem, DLL dependency audit and embedded-host extraction tests.

The full document suite still reports the previously observed short-editor
initial-row-position and legacy non-window scrolling failures. The delegate
detachment selection fixture now explicitly requests a selection, since an
unselected editor intentionally no longer allocates selection surfaces.

AI assistance has been used to create this output.
