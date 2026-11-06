# Text viewer/editor return to panels, 2026-11-06

Measured the portable `zoin` worktree app using the normal profile and the real
F3 → Escape → F4 → Escape flow. The selected file was
`P:\Events\[2026.10] UAE\[2026.10.26]\test\testfile.txt`; its panel has two entries,
and the other retained panel has 348 entries. Native Qt/Go monotonic navigation
probes were enabled. Computer Use delivered keys; accessibility snapshots were
used to confirm the current surface. This is not a QML-debugger trace or a CPU
sampling profile. Timings include the normal transport and rendering pipeline.

## Identified work

Escape handling itself took about 2–5 ms. The expensive operation was the
`panels_return_requires_catalog` rejection in `BuildAppScenePatch`. Entering a
text document removed the shell from the transport's authoritative snapshot.
On return, the row-free panel header could not safely reconstruct its catalog,
so the renderer exported a complete scene and sent both panel catalog snapshots.
The two measured full exports took 226.69 ms and 218.65 ms. Their resulting
panel scene updates reached Qt 281.08 ms and 270.80 ms after keydown.

`qt_semantic_panel_status.go` also queued redraws for identical disk-capacity
results (and stale filesystem results). This added render transactions around
surface switches without changing visible state.

## Implementation and invariants

The incremental transport snapshot retains the immutable panel shell headers
while a viewer/editor surface covers the panels. The full local snapshot keeps
its existing rows, and Qt keeps its existing panel objects. Returning reconciles
panel identity, catalog revision, state and selection through the existing patch
logic. It does not transmit catalog rows when they are unchanged. A new panel
identity, changed catalog revision or absent retained shell still requires the
conservative full export. This applies to text viewers/editors only; it does not
retain panels across arbitrary unsupported surfaces.

Capacity-query completion now uses `PostTaskWithRedrawDecision`. Changed free
space, capacity, availability and symlink state still redraw. Identical results
update query bookkeeping only; results from a stale source/path are ignored.

No QML layout, input-repeat behavior, wire schema or native host binary changed.

## Live final samples

All values are milliseconds from Qt keydown. Scene apply is the first correlated
`qt.input.patch.applied`. First frame is the first render-completion event whose
sync began after that apply. Missing frame observations are left blank; later
unrelated frames are not substituted. Document presentation was additionally
confirmed with `qt.document.window.frame.swapped`.

| Operation | Scene apply | First frame |
| --- | ---: | ---: |
| First viewer open | 25.33 | 66.30 |
| Escape from viewer | 10.72 | 25.85 |
| First editor open | 41.33 | 51.89 |
| Escape from editor | 12.62 | 29.12 |
| Repeated viewer open | 18.00 | 40.29 |
| Escape from viewer | 8.06 | 11.67 |
| Repeated editor open | 20.14 | 37.37 |
| Escape from editor | 10.70 | unobserved |

No full scene exports occurred during these final cycles. The redundant
catalog-export stall is removed. These small-sample observations do **not**
establish p95/p99 latency, nor do they demonstrate a consistently sub-16-ms
complete open/close cycle. File preparation, document row placement, asynchronous
scene commits and frame scheduling remain in the open path. First document
presentation remains more expensive than a warm open. UI Automation and tracing
can perturb timings; scene-apply latency must not be described as screen latency.

## Verification

- Regression-first `TestDocumentRoundTripRetainsUnchangedPanelCatalog` failed
  before the fix for both viewer/editor because opening discarded the catalog.
  It now checks repeated document updates, unchanged return, changed catalog
  revisions, different panel identity, preservation of real rows and bounded
  patch size.
- The existing 10,000-row editor-return regression now requires catalog reuse.
- `TestNativePanelStatusQueryRedrawDecision` covers initial, identical, changed
  capacity, symlink and stale-source query completions.
- Full `internal/nativeui`, `internal/plughost` and `internal/semantic` suites
  passed. Focused panel status tests passed.
- The full panel suite has an existing failure in
  `TestWindowsLocationAccessErrorDialogShowsReasonAndResolvesThemeAtRenderTime`:
  `normal button attr = 0xa0a1a29091920300`, expected `0x6061625051520300`.
  Replacing both modified panel status source/test files with their HEAD versions
  reproduces the same isolated failure. Both task files were restored afterward.
- The portable launcher was rebuilt with the existing embedded audited Qt host.
  The normal-profile application is restored without diagnostic flags.

Local raw evidence remains in `%TEMP%/f4-document-cycle-{go,qt}-{before,after,final}.jsonl`
and `%TEMP%/f4-document-cycle-summary.json`; raw traces and executables are excluded
from source control.
