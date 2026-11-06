# Display cadence attribution for the first standalone-texture pair

This analysis uses only the accepted real-scroll-standalone-ready control and
enabled captures. It does not replace the pending reversed-order comparison.

## Timing validation

Every displayed row's absolute timestamp is calculated as QPC present time plus
msUntilDisplayed. Consecutive displayed timestamp differences reconcile with
msBetweenDisplayChange to less than 0.000000069 ms in all four capture files.
The arithmetic preserves the irregular display cadence; it does not smooth it.
Process, swapchain and timestamp ordering assertions also pass.

The definitions distinguish display intervals from time spent in Present and
from GPU work. PresentMon defines the render-present latency as the interval
between Present and GPU completion, not active GPU execution alone. See the
[upstream metric definitions](https://github.com/GameTechDev/PresentMon/blob/main/README-CaptureApplication.md).

## Correlation, without assigning cause

The control has 181 displayed intervals exceeding 9.375 ms across four blocks.
None coincides with a same-row producer interval exceeding 9.375 ms. Only one
has such a producer interval in the immediately preceding row. Render work for
the matching frames is generally short: median render-to-frame-end time is
0.689, 0.893, 0.868 and 0.972 ms by block. These observations do not exclude
earlier scheduling, resource or composition effects.

For those long-display rows, median Present-to-GPU-completion latency is
5.769, 5.606, 5.628 and 5.588 ms, while active GPU time is approximately
0.597-0.608 ms. They are different measures and must not be added or substituted.

However, the enabled arm also has a median completion latency of approximately
5.66 ms across all displayed frames, even though it has no >9.375 ms display
intervals. All-frame GPU activity medians are approximately 0.41-0.42 ms in
both arms. Thus the completion-latency observation alone does not explain the
cadence difference and does not identify a shader execution bottleneck.

All rows in the input windows report Composed: Flip. No present in those windows
is marked dropped. Long/short timing variations remain visible even without
dropped frames, and the strict displayed-motion limit remains failed.

The next discriminating evidence is the reversed-order texture comparison,
followed by native render-pass CPU attribution. Do not promote the standalone
candidate based solely on this temporal pair or blame the compositor/driver
solely because local preparation is fast.

Reproduction: artifacts/analyze-standalone-display-cadence.py. Outputs:
artifacts/standalone-ready-display-cadence.json and
artifacts/analyze-standalone-display-cadence.log. The verifier retains raw fields
and same-frame correlations rather than attempting a GPU-wait decomposition.
