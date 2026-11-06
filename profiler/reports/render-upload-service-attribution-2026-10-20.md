# Render upload service attribution, 2026-10-20

Constant 160 Hz without spikes remains incomplete. This investigation adds diagnostic
CPU-service counters and scheduler attribution. It makes no production behavior change.
The normal worktree application stays on the verified startup-registration build.

## Scope and integrity

Downloads, 186 entries, Masonry density 152, DPR 1.75, 160 Hz display, bouncing
wheel input every 30 ms. Three fresh-process captures each contain four 10-second
input windows: 120 seconds total. Native scrolling uses the production default and
unchanged finite OutSine path; there is no Hermite experiment. The renderer retains
its bounded 16 MiB/category pools and existing upload admission limits.

The first two captures add QueryThreadCycleTime at prepareRenderPass and alpha-upload
boundaries only (four counter reads/frame). Counters and stages use bounded arrays,
pre-touched before startup; all measured samples have valid cycle readings. Thread-time
calibration showed GetThreadTimes returning zero for 57/64 busy 2 ms intervals, so it
was not used for these short stages. Raw cycles are retained and are never converted
to milliseconds: https://learn.microsoft.com/en-us/windows/win32/api/realtimeapiset/nf-realtimeapiset-querythreadcycletime.

The second capture additionally records context switches and ready-thread events.
Both scheduler traces cover the input windows, match the process/thread identities,
and report zero events/buffers lost, zero schema errors, and successful ProcessTrace.
They are decompressed-hash-verified before source ETLs are removed. Scheduler tracing
has overhead; it is diagnostic evidence, not an uninstrumented performance claim.

The third diagnostic expands only the D3D11 full dynamic-update call into its existing
map, memcpy(requested bytes), unmap sequence, adding separate timing scopes. Qt 6.11.1
QD3D11Buffer does not override the full helper; the helper implementation was verified:
https://github.com/qt/qtbase/blob/v6.11.1/src/gui/rhi/qrhi.cpp#L3766.
Other backends retain the original call. No production Qt library was replaced.
All part nesting/count checks pass. Slow-upload data is thresholded at 100 us and
must not be treated as total upload traffic. Nested durations must not be summed.

No build, test, offline analysis, or UI Automation observer ran during input windows.
Only the scheduler-designated capture ran kernel tracing. Captures preserve actual
successful-link receipts and source hashes. Stage-1 source/receipt snapshots precede
the later diagnostic rewrite; warm snapshots are cumulative and are filtered by the
explicit input windows, not blindly compared as disjoint runs.

## Evidence

The earlier failed production capture remains authoritative: its 7.0954 ms preparation
preceded an 18.734 ms display gap. Of its recorded slow uploads, ordinary UI materials
spent up to 1.738 ms in full dynamic updates while reusing oversized buffers. That
particular outlier did not recur here; its off-CPU attribution is still unproven.

In the new scheduler capture, the worst public preparation frame was 5.2354 ms:
5.2338 ms scheduled, 0.0016 ms runnable, zero blocked/unknown. The worst warm frame
was 4.5164 ms: 4.5139 ms scheduled, 0.0025 ms runnable, zero blocked/unknown. This
rules out scheduler descheduling/blocking as the principal cost in those two frames.
Scheduled time can include driver spinning and is not proof of useful application work.

Their largest upload is the same unmerged CurveStroke batch: 2,054,448 vertex bytes
plus 186,768 index bytes. Cold upload takes 3.1227 ms, of which 2.9117 ms is the full
dynamic update. Warm upload takes 2.6473 ms, including 2.4563 ms dynamic update.
Both reuse existing buffers without growth, conversion, or creation. Therefore
eliminating resource creation alone cannot remove this cost.

The separate split-call capture identifies two kinds of expense:

- Ordinary SmoothColor requests 800 vertex bytes from a 50,952-byte reused buffer:
  full update 0.5431 ms, Map 0.5356 ms, copy 0.0059 ms, Unmap 0.001 ms.
- A 2,241,216-byte CurveStroke update: full update 0.9004 ms, Map 0.1835 ms,
  copy 0.7144 ms, Unmap 0.002 ms. It also reuses existing buffers.
- Another small text batch requests 1,680 bytes from a 511,896-byte vertex buffer:
  Map 0.2576 ms, copy 0.0002 ms. Oversizing is observed; its causal contribution
  still needs an isolated comparison.

In thresholded split samples, mapping totals 73.3754 ms versus 24.5517 ms copying
across the four windows. These totals cover only recorded slow uploads. Unmap is
usually small. The split capture did not reproduce the earlier 2.9 ms full update;
its lower numbers do not establish that inlining or added instrumentation improved
performance. Do not promote the diagnostic rewrite as an optimization.

All twelve measured windows have complete presentation brackets and zero missed
refreshes under the current analyzer. This is encouraging repeated evidence but
cannot erase the earlier failed capture or establish the full goal.

## Next optimization targets and limits

Separate the two observed cases. For tiny ordinary batches, test selecting a smaller
adequate pooled buffer when an owned/reused buffer is grossly oversized, retaining
the existing bounded pool and measuring creation/growth churn. Do not add blind
shrinking or reverse the verified 16 MiB working-set fix.

For large static SVG batches, investigate avoiding repeated GPU uploads of unchanged
geometry across gate re-entry. Any cache must have explicit revision/invalidation,
bounded resource ownership, recycling/device-loss cleanup, and unchanged full-vector
presentation. The earlier visited-root-retention experiment was rejected after a
negligible measured benefit; do not silently reintroduce it. A single large upload
can exceed the time budget because admission checks occur between whole batches.
Any staged-upload design must retain complete-image presentation and draw ordering.

The rare earlier ordinary-buffer stall still needs a reproduced scheduler trace.
Velocity continuity, reversal/bounds behavior, other input patterns, and broader
workloads also remain within the active goal. No commit or push was made.

Evidence is indexed by ../experiments/render-upload-service-attribution-2026-10-20/manifest.json.
AI assistance has been used to create this output.
