# Asynchronous delegate spare reserve — 2026-10-19

Status: isolated prototype, not deployed. Constant 160 Hz without spikes remains
unachieved. This experiment changes neither production source nor the normal
running worktree application.

## Why this experiment

Native samples previously placed createComponent/popBrickItem/prepareDelegateRow
and QML incubation in long GUI producer intervals. Moving the same nearby callback
after a frame did not improve cadence. Test whether a bounded reserve can remove
synchronous root construction from some row-admission operations instead.

## Implementation and bounds

A separate gallery library modifies copies of MasonryLayout.cpp and
MasonryLayoutProperties.cpp. Only non-contained Masonry requests a reserve. It
asynchronously constructs up to two complete, unassigned delegates using the
existing component, viewport context and Qt incubation controller. One construction
can be pending while completed spares are consumed. An empty reserve retains the
original synchronous visible-row creation path. No visible request waits for a
spare to finish.

Completed spares have no file identity. Taking one installs final row properties
within the existing visual-update boundary before normal row preparation continues.
Component/viewport replacement cancels pending work and discards unused spares;
owner destruction clears the sidecar. Definitive component errors stop automatic
retry until configuration changes. Existing recent-row pool, viewport plans,
thumbnail priorities, nearby scheduler and collector remain intact. Two extra
completed unassigned slots per participating layout are outside the normal pool;
this is an explicit small memory/lifetime addition, not zero-cost prefetch.

The helper has no Qt or MasonryLayout ABI change. Sources and binaries stay under
artifacts/async-delegate-spares and separate diagnostic executable names. No
installed SDK, production dependency patch or embedded payload is replaced.

## Regression work and results

The initial standalone test reproduced an initialization error: required viewIndex
was not recognized as initialized. Calling QQmlComponent::setInitialProperties
from setInitialState was insufficient for this incubator. Supplying the map through
QQmlIncubator::setInitialProperties before create fixes the failure. The original
required-property case changes from exit 1/zero ready spares to exit 0/five ready
spares across creation, consumption and component replacement.

The extended lifecycle test passes: request deferral, completion before return,
unassigned identity, mode initialization, two-spare bound, replenishment, component
replacement, taken-child lifetime, pending-owner cancellation, registry cleanup and
suppression of retries after a deliberately invalid component. Its final JSON
includes the expected errors from that intentional invalid-component case; they
are not normal-host failures.

The first full Direct3D mode run had 43 passing results and one failure in
masonryRetiresSlotsUsingTheRendererVisibilityPolicy. A newly prepared spare had
isVisible=false although nativeCullingAvailable=true. The prototype now applies
the existing renderer visibility policy to completed unassigned slots and tracks
capability changes. The existing test is unchanged. Final selected mode runs pass
44 results per backend including fixture setup/cleanup, on both Direct3D 11 and
OpenGL at actual test DPR 1.75. They cover reuse, mode geometry/navigation,
retained vectors, coherent row publication, pixel-grid leaves and rendered output.
This is not a full repository test-suite claim.

## Clean scrolling comparison

Six fresh processes run A1/B1/B2/A2/A3/B3. A is the bounded-Details clean baseline;
B adds the asynchronous reserve. Each run has two 10-second boundary-bounce wheel
windows, 30 ms wheel spacing, Downloads 186 entries, density 152, extent
4101.142857 logical pixels, actual DPR 1.75 and 160 Hz display. Both use the
original collector with a 2 ms budget. Frame probes, object census, caption
measurements and model/snapshot-only changes are absent. Input/motion assertions
verify active scrolling and trajectory reversals. No builds, tests, UI Automation
or offline analysis overlap capture. The normal worktree app stays alive beside
both arms.

Native reserve counters are copied once after settling/before the initial window
grab and serialized once at result collection. Deltas include the benchmark run
and any pause between its input windows; they are not exact per-frame scope counts.
No per-frame reserve probe is installed.

| Run | Display intervals >9.375 ms | Maximum (ms) | Block p99 (ms) | Prepared delegates consumed |
| --- | ---: | ---: | --- | ---: |
| A1 | 3 | 12.5030 | 6.3154 / 6.2787 | — |
| B1 | 10 | 12.5450 | 6.3966 / 6.2947 | 70 |
| B2 | 4 | 12.5044 | 6.3020 / 6.2827 | 72 |
| A2 | 2 | 18.7439 | 6.3268 / 6.3043 | — |
| A3 | 3 | 18.7520 | 6.2814 / 6.2966 | — |
| B3 | 3 | 12.5047 | 6.3025 / 6.2958 | 69 |

Across 60 seconds per version, A has 8 long intervals and B has 17. All B runs
have zero reported reserve-construction failures and maximum ready count two.
The reserve is exercised, rather than remaining unused during measurement.

The maximum observed interval is lower in B, but missed refreshes are more
frequent. This small alternating trial does not establish a general regression
magnitude or a reliable tail-latency improvement. It provides no consistent cadence
gain that justifies adoption. These are displayed intervals, not exclusive CPU
cost or QML creation durations. First-visible image latency and memory overhead
are not separately quantified in this clean comparison; the preserved synchronous
fallback alone does not prove absence of indirect latency effects.

## Decision and remaining work

Keep the reserve isolated. Moving construction ahead of demand demonstrably
supplies completed roots, but it does not meet the frame-delivery goal. The next
measurement should correlate the remaining misses with row assignment, shared
QML incubation, GC and render-thread work in this exercised prototype, with the
original host as control. Do not assume root creation explains every sampled
prepareDelegateRow stack, and do not combine several unproven optimizations then
attribute a result to one of them.

The diagnostic panel was restored and only its exact-path launcher tree was
stopped. The normal worktree build remains on host SHA-256
509c4b470b89b3171bb4f8f7e31faf9d9fc9011800c2e1bc1bdebfb50aa893ca.
No commit or push was made. Compact source snapshots, before/after receipts,
selected rendered captures, measurement JSON and SHA-256 identities are archived
under profiler/experiments/async-delegate-spare-reserve-2026-10-19. Executables,
libraries, raw CSV/ETL and build outputs are excluded.

> AI assistance has been used to create this output.
