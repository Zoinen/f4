# Masonry geometry-bounds attribution — 2026-10-21

Status: diagnostic only. No bounds cache or application behavior change was introduced. Constant 160 Hz remains unachieved.

## Question and method

Phase-specific CPU samples pointed at `Element::computeBounds` in a few preparation tails. This probe tests whether large geometry is repeatedly scanned within the same observed geometry revision, which could justify caching local-space bounds.

The diagnostic uses the current production libraries, with reversible instrumentation in the batch renderer and the existing bounded scrolling recorder. It observes geometry dirtiness, replacement, removal, root changes, and transforms. Detailed timing covers geometry with at least 256 vertices. A repeat requires the same node, geometry and vertex-data pointers, count, stride, position offset, and observed DirtyGeometry epoch. Smaller geometry contributes only cumulative counts. CPU cycle readings are raw cycles, not CPU milliseconds.

The timed scope excludes probe hash bookkeeping. The probe still adds bookkeeping and cycle-query overhead, so these are attribution measurements, not an instrumented-versus-production speed comparison. A DirtyGeometry notification can occur even when vertex values have not changed; this probe does not compare vertex contents. Node removal intentionally invalidates its record.

## Accepted captures

`real-scroll-bounds-scan-20261021-{cold,warm}-middle-masonry`: Downloads, 187 entries, density 152, DPR 1.75, 160 Hz. Middle scrolling distance is one quarter of the window height. Both captures pass focus and bounded-recorder checks; no state-table overflow. Each contains two approximately ten-second input windows. Analysis selects only records wholly inside those windows, not cumulative startup counters.

| Pass/block | Large scans | Vertices scanned | Total scan time | Same-revision repeats |
| --- | ---: | ---: | ---: | ---: |
| Cold / 0 | 1,109 | 1,127,108 | 3.937 ms | 0 |
| Cold / 1 | 1,107 | 1,126,596 | 3.882 ms | 0 |
| Warm / 0 | 1,115 | 1,128,644 | 4.207 ms | 0 |
| Warm / 1 | 1,107 | 1,126,596 | 4.821 ms | 0 |

The slowest preparation frame is 6.3053 ms, of which 0.4300 ms is measured large-geometry bounds work (118 scans, 106,884 vertices). Other preparation tails have no large scans: for example, 4.0474 ms cold and 4.4645 ms warm. These measurements do not explain all preparation work, small geometry, scheduling interruptions, or recording/submission stalls.

## Decision

Do not implement the proposed per-node local-bounds cache from this evidence. The probe found no reusable large bounds within its invalidation contract. This does not prove vertex contents always changed: redundant DirtyGeometry notifications, scene reconstruction, and removal/reinsertion remain possible follow-up subjects. Bounds work occurs in bursts, but it does not account for most measured preparation tails. Further attribution should focus on preparation outside these scans and the separately observed recording/submission costs.

## Verification

- Probe unit tests: 6 passed, 0 failed, 0 skipped.
- Actual Windows/D3D11 drawing-order tests at DPR 1.75: 44 passed, 0 failed, 0 skipped.
- Actual Windows/D3D11 curve-preprocessing tests at DPR 1.75: 6 passed, 0 failed, 0 skipped.
- Embedded QML import and compiled-host checks: 4 passed, 0 failed, 0 skipped.
- Windows DLL import audit: system DLLs only.

An earlier offscreen drawing-order run skipped 42 native cases. It is not the evidence used above; the subsequent hardware run executed them.

Source, receipts, test logs, capture packets, presentation CSVs and analysis are preserved under `profiler/experiments/geometry-bounds-probe-2026-10-21`, with SHA-256 manifest. Executables and object files are excluded. The normal worktree application was kept alive throughout; the separate diagnostic launcher is stopped after capture.
