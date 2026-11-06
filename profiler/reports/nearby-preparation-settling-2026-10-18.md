# Nearby delegate preparation settles with retained SVG scenes — 2026-10-18

## Change and reproduced failure

The preparation completion check now excludes precisely the retained-vector indexes already computed by retirement. The set is passed through the commit context, avoiding an additional retained-scene scan. Ordinary offscreen retention limits, vector retention limits, preparation budgets, GC and renderer settings are unchanged.

A new native regression creates real SVG and ordinary-file delegates, scrolls through enough rows to accumulate the bounded ordinary working set alongside retained vector scenes, then checks that preparation stops scheduling. Before the fix the fixture passed its retention/debt preconditions but failed `!layout->_nearbyPreparationScheduled`. Afterward it passes and retained vector indexes remain unchanged after settling. Test access uses a friend declaration, with no new runtime property or observer. Software rendering skips the hardware-only retention case explicitly.

## Verification

At DPR 1.75, Direct3D and OpenGL each passed the new settling regression and existing nearby preparation cases for Columns/Masonry (5 passes per invocation including setup/cleanup). Software passed the two existing preparation cases and skipped the native retention case. Existing vector-scene retention and vector panel alignment cases passed across Details, Columns, Grid, Icons and Masonry (8 passes including setup/cleanup). Embedded QML import isolation passed (3 including setup/cleanup). Windows system-DLL audit passed. The embedded payload was regenerated, CGO_ENABLED=0 launcher built, and payload/extraction/reuse/concurrent first launch/corrupt payload/lookup tests passed.

These tests do not constitute a full test-suite or cross-platform release claim. No UI layout geometry was changed.

## Clean actual-wheel comparison

A/B/B/A, normal default 5000 us GC budget, current buffer-fit QtQuick, D3D11, Downloads 184 entries, DPR 1.75, active 160 Hz window. Every capture injected 268 wheel events at 30 ms. No allocation observer, ETW sampler or build ran during these captures. Strict display-window analysis requires both display endpoints within the same direction window.

| Case | Down display p99 / maximum (ms) | Up display p99 / maximum (ms) |
| --- | --- | --- |
| Before A1 | 8.7686 / 18.7546 | 8.3343 / 12.4975 |
| After B1 | 8.8248 / 18.7011 | 8.3452 / 12.4733 |
| After B2 | 8.8777 / 18.7494 | 6.8086 / 12.4975 |
| Before A2 | 6.3216 / 18.7419 | 9.0050 / 12.5027 |

No repeatable display-cadence improvement is established. The fix is retained because the recurring timer bug is directly reproduced and corrected. Constant 160 Hz without spikes remains unachieved. Remaining investigation should target actual visual-binding allocations and render upload/copy work; do not attribute every remaining spike to the corrected timer or change GC constants without evidence.

Evidence: `profiler/experiments/nearby-preparation-settling-2026-10-18/` contains compact captures summaries and focused test logs. Raw capture JSON/PresentMon CSV remains under `artifacts/modes-profile/nearby-{a1,b1,b2,a2}-wheel-masonry*`.

AI assistance has been used to create this output.
