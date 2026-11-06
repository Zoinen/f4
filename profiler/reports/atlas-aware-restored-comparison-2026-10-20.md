# Atlas-aware shader: restored-display comparison rejects the candidate

The user restored the display. Fresh diagnostic QQuickWindow captures now
confirm 160 Hz and DPR 1.75. Adapter WMI continues reporting 120 Hz and is not
used as the authority for the window's display context. The goal is active and
incomplete. No candidate was promoted and the ordinary application is unchanged.

## Matched comparisons

One control/enabled pair and a reversed enabled/control pair use the same linked
executable and successful-link receipt. Only the atlas-aware resource flag is
changed. The standalone-provider flag is disabled throughout. Material-part
instrumentation is disabled in both arms; the common native stage probes remain.

Each arm has two ten-second cold inputs and two ten-second warm inputs, using
quarter-window middle-button boundary-bounce scrolling in Downloads. Window
2194x1186, DPR 1.75, density 152, 186 entries, graphics API and per-stage scroll
extent match across each pair. Cold and warm extents differ from each other, so
the comparison matches corresponding stages rather than treating warm as a
matched cold-layout baseline. All accepted captures remain active throughout,
with bounded, non-overflowing native and phase recorders.

Preparation p99, control -> atlas-aware, milliseconds:

| Pair | Stage/block | Control | Atlas-aware |
| --- | --- | ---: | ---: |
| First | Cold 0 | 2.7241 | 3.2516 |
| First | Cold 1 | 1.8557 | 3.1630 |
| First | Warm 0 | 2.0639 | 2.9481 |
| First | Warm 1 | 2.2103 | 2.4512 |
| Reversed | Cold 0 | 3.3116 | 3.7256 |
| Reversed | Cold 1 | 1.8496 | 4.0356 |
| Reversed | Warm 0 | 1.7812 | 5.3280 |
| Reversed | Warm 1 | 2.0328 | 4.4559 |

Recording p99 also worsens in all eight blocks. The reverse ordering rules out
calling the first result a sufficient basis for promotion: the repeat reinforces
the regression. Rendering correctness and removal of an extra resource path
did not translate into a performance improvement. The added shader work/shared
atlas behavior needs attribution before assigning a specific cause; these data
do not isolate GPU arithmetic from driver/resource interactions.

Displayed intervals over 9.375 ms, summed across the four blocks per arm:

- First pair: control 5, atlas-aware 2.
- Reversed pair: control 7, atlas-aware 65.

The inconsistent first displayed count is another reason not to select a change
on a short count alone. The repeated preparation/recording tail regression is
the basis for rejecting this candidate. Producer long-present counts are a
different metric and are not substituted for these displayed counts.

## Motion and remaining baseline stall

Strict motion commands retain commit p95 <=0.1 physical pixels, displayed-step
maximum <=2 pixels, zero missed refreshes, zero coverage stalls and complete
present coverage. Both pairs fail; thresholds were not relaxed. Separate
diagnostic runs without gate arguments retain all cases for inspection but are
not reported as successful motion verification.

Commit-step p95 is approximately 0.0003011 physical pixels in every block. The
first control's cold block 0 nevertheless reaches a 54.1173 pixel displayed-step
error with a 25.0018 ms displayed interval. Its other three blocks have no
display misses and displayed-step maxima below 0.52 pixels. In the reversed
control, cold block 1 has a 2.6399 pixel maximum even without a >9.375 ms miss;
this still fails the unchanged strict ceiling.

Around the first control's cold stall, consecutive producer intervals reach
16.5245, 19.8036, 24.7738, 28.9615 and 15.5720 ms. The 28.9615 ms interval's
public preparation is 7.0065 ms, but batch preparation is only 0.4540 ms /
1,535,440 thread CPU cycles. A nested beginRenderPass scope consumes 6.2253 ms.
That scope has no CPU-cycle measurement, so it cannot yet be classified as CPU
work versus waiting. Neighboring long intervals also occur with much shorter
render work. These findings point beyond thumbnail batch construction alone;
nested scope durations must not be summed as independent work.

The atlas-aware candidate remains off. Next compare the already-tested standalone
thumbnail provider under the same executable, then investigate the surviving
baseline scheduling/begin-pass stalls rather than declaring 160 Hz achieved.

## Excluded runs and interaction constraint

The first reversed-enabled attempt lost window focus and was rejected by the
existing validity check. A fresh reversed pair passed capture validity and is
the one used above. The first standalone-provider attempt also lost focus and
was rejected. Neither excluded trace contributes to the performance comparison.
The user has been asked for a two-minute interval without desktop input before
the next standalone comparison; that dependent capture is pending their reply.

## Evidence

Accepted prefixes: real-scroll-atlas-aware-restored-control/enabled and
real-scroll-atlas-aware-reversed2-control/enabled. Excluded prefixes:
real-scroll-atlas-aware-reversed-enabled and real-scroll-standalone-restored-enabled.

The two phase/motion summaries have separate preserved names:
atlas-aware-restored-first-phases.json, atlas-aware-restored-first-motion.json,
atlas-aware-reversed2-phases.json and atlas-aware-reversed2-motion.json. The
aggregate atlas-aware-restored-pairs-analysis.json verifies matching receipts,
flags, geometry, recorder bounds, and all eight preparation regressions.
Strict failure logs are retained separately from diagnostic-only motion output.
