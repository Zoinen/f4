# Controlled SVG upload chunking measurements, 2026-10-20

The 160 Hz goal remains incomplete. Same-binary controls exposed a substantial
preview-latency regression in one-step-per-frame chunking. A budget-filling revision
largely removed that tail regression while retaining lower preparation p99. Neither
prototype is deployed; the normal worktree app remains the visible-node-admission
production build, launcher SHA256
552e2f7322e04f34caf9ebc131dfdc7a648c51f056339c26010b9a9f338e20d8.

## Instrumentation and validity

A diagnostic-only F4_PROFILE_DISABLE_VECTOR_CHUNKS switch selects the original
whole-upload route in the same executable. Every cold/warm capture in each pair
has identical successful-link receipts; its build identity records the switch.
The final sources/probes and the preceding one-step version are preserved separately.

Bounded probes record chunk byte count, time, batch identity and gate sequence.
Separate events record each gate sequence's first renderer observation, first ready
state, first high-priority demand (priority <=100), and first ready+present eligibility.
All counters stayed below capacity and gateOverflow was zero. Records publish once
with release/acquire atomics; root flag bookkeeping has a fixed diagnostic bound.

Presentation itself is a poor demand timestamp: during native scrolling it is enabled
only after readiness. The demand event therefore uses visible-request priority, which
the existing motion controller derives from viewport intersection. Eligibility is a
renderer proxy for a surface's ability to draw in that frame, not the first physical
screen pixel or end-to-end file-open latency. Measurements cover first demand per
gate sequence; later invalidations within that same sequence are not new episodes.

Only demands and completions observed in the same input window enter wait statistics.
Uncompleted counts are reported; these include disappearance/cancellation and boundary
cases and are not assumed to be merely slow completions. Populations are aggregate
gate episodes, not files paired by pathname across processes.

Both pairs use Downloads, 186 entries, Masonry density 152, DPR 1.75, 160 Hz screen,
30 ms bouncing wheel, two cold and two warm ten-second windows per run. Cold means
first scrolling in that process, not an emptied filesystem cache. Source extents match
between controls: cold 4101.142857, warm 3649.142857 logical pixels. No builds, tests,
UIA observer or offline analysis ran during any input window. Bounded preparation and
independent PresentMon/motion instrumentation are the same within each pair.

## One-step-per-frame control

Prefixes real-scroll-vector-chunks-control-disabled/enabled, same binary:

| Metric | Disabled | Enabled |
| --- | ---: | ---: |
| Cold completed / uncompleted demands | 162 / 4 | 160 / 2 |
| Warm completed / uncompleted demands | 164 / 0 | 157 / 1 |
| Cold eligibility median / p95 ms | 23.3930 / 53.7005 | 32.2208 / 99.0708 |
| Warm eligibility median / p95 ms | 18.1710 / 37.4435 | 28.7029 / 93.7790 |
| Preparation maximum ms | 7.5753 | 3.3118 |

Chunking reduced the observed preparation spike but more than doubled warm p95 wait.
This version was rejected for promotion. A representative 558,432-byte source used only
two transfer steps 5.844 ms apart, yet waited 99.503 ms from demand to eligibility;
most of its delay was queueing behind other sources, not its own two copies.

The pending loop processed each batch once per frame. New chunked batches were also
budgeted at their entire estimated payload, causing early exits despite unused byte
capacity. Both effects unnecessarily postponed work to subsequent frames.

## Budget-filling revision

The revision continues a batch in <=512 KiB steps while the existing 1 MiB byte and
2 ms soft time limits allow. Prospective work is capped only when it is eligible for
actual chunking; ordinary small or unsupported uploads retain their original estimate.
Changed/dirty batches do not reuse an old partial-transfer estimate. Chunk format
eligibility agrees with initialization, including unmerged geometry, index compatibility
and visualization state. Neither byte nor time budgets were raised.

Validation for this revision:

- D3D11 and OpenGL: 47 enabled renderer checks and 44 disabled control checks each.
- Both backends: 30 camera and 14 SVG presentation/animation checks each.
- Completion, replacement during transfer, multi-node drawing, painter order, buffer
  lifecycle and existing oversized completion limits retain their regression coverage.

One intermediate test harness had an incorrect function-count assertion, corrected to
its actual 14 existing functions before any test run. One intermediate link failed
because the owned diagnostic host was still running; only its exact launcher tree
was stopped and the final build completed. Those logs are retained. The normal app
remained running and was not replaced.

## Budget-filling controlled results

Prefixes real-scroll-vector-chunks-fill-disabled/enabled, same binary:

| Metric | Disabled | Enabled |
| --- | ---: | ---: |
| Cold completed / uncompleted demands | 160 / 3 | 164 / 4 |
| Warm completed / uncompleted demands | 160 / 0 | 160 / 0 |
| Cold eligibility median / p95 ms | 23.9794 / 56.3690 | 25.3325 / 59.9614 |
| Warm eligibility median / p95 ms | 19.0665 / 37.5895 | 25.2784 / 37.3113 |
| Warm eligibility p99 / max ms | 53.7762 / 66.3198 | 59.5366 / 66.2145 |
| Cold preparation p99, blocks 0/1 ms | 2.5562 / 2.1009 | 2.1397 / 1.8426 |
| Warm preparation p99, blocks 0/1 ms | 2.0976 / 2.1634 | 1.8672 / 1.7545 |
| Preparation maximum ms | 3.9327 | 4.0398 |
| Render-to-frame-end maximum ms | 4.9218 | 5.5088 |

Warm p95 is now comparable to control; warm median/p99 still cost about one frame.
Cold p95 is 3.59 ms higher. This is an explicit tradeoff, not a claim that all loading
latency improved. The earlier 7.58 ms control spike did not recur in this second pair;
it remains valid evidence and must not be erased by the newer smaller maximum.

Enabled chunk CPU p99/max: cold 0.1800/0.2328 ms; warm 0.1151/0.1720 ms.
Every transfer step stayed <=524,288 bytes. These times cover consolidation/queueing,
not subsequent driver/GPU execution. The full pipeline maximum matters: recording
spikes up to 2.5946 ms remain in the enabled run and require attribution.

All sixteen ten-second blocks across the two controlled pairs had zero missed display
intervals (>9.375 ms), no out-of-presentation camera commits, and passed motion-error
p95 <=0.5 / maximum <=2 physical pixels. Largest display intervals were 6.4272 ms in
the first pair and 6.3843 ms in the budget-filling pair. Producer-present interval
outliers still occur and remain distinct from actual display misses.

## Remaining work

Before promotion, measure rapid wheel and middle-button scrolling and check viewer
latency/priority, geometry mutation during partial transfer, generation changes, and
hidden/visible transitions. Inspect large remaining recording/driver spikes; do not
move upload work outside the measured scope and call it eliminated. If the tradeoff
is acceptable under those checks, integrate strict portable recipe replacements and
regressions, rebuild the production library and embedded host, run import/dependency/
extraction gates, restart only the worktree app, and repeat measurements on that source.
No commit/push or production deployment occurred in this investigation.
