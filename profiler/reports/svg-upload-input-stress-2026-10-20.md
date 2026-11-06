# SVG upload prototype: rapid wheel and middle scrolling, 2026-10-20

The 160 Hz goal remains incomplete. Broader input testing caught a real missed refresh
in sustained middle scrolling. The upload prototype is still not deployed. The running
worktree application remains the visible-node-admission build, launcher SHA256
552e2f7322e04f34caf9ebc131dfdc7a648c51f056339c26010b9a9f338e20d8.

## Workload and instrumentation

Downloads, 186 entries, Masonry density 152, DPR 1.75, 160 Hz display. Each capture
has two cold and two warm ten-second input windows. No builds, tests, analysis or
UI Automation observer ran during input. Actual successful-link receipts are identical
across rapid wheel, middle enabled, and middle disabled captures. Prototype source
and admission budgets are unchanged from the budget-filling candidate.

Rapid wheel sends actual QWheelEvent objects to the window every 10 ms. Each case
has at least 1500 events and checks focus per event. It exercises production wheel
handling, not an artificial linear animation.

Middle input calls the production auto-scroll controller's start/updatePointerMotion/end
methods with a synthetic pointer-source map. This tests controller/rendering behavior,
not OS mouse-event dispatch. Pointer offset is exactly one quarter of window height:
296.5 logical pixels, speed 1627.4334 logical pixels/second. A diagnostic 20 ms timer
reverses direction near document boundaries so ten-second windows stay busy. This
is a sustained extension of the old 1.5-second single-pass test; simply extending its
duration would mostly measure idle boundary time. Command-file polling stops during
middle input as it already does for wheel input.

Middle cases recorded 1014/1015 focus checks each, zero inactive checks, and 8/9
turns. More than 95% of native samples moved (in practice almost every sample).
No requested-to-visual camera clamps or coverage stalls occurred. Probe counts stayed
within capacity and gateOverflow was zero. All controls record their on/off switch.

Render-thread cycle counters now also cover recording, end-pass and resource-update
scopes. Cycles are raw QueryThreadCycleTime observations, not converted to CPU ms.
They add attribution overhead; these diagnostic runs are not exact uninstrumented
production measurements.

## Rapid-wheel result

Prefix real-scroll-vector-chunks-rapid. All four windows had zero missed display
intervals (>9.375 ms). The motion gate passed cold input but failed warm block 1:
maximum displayed step error 2.0407 physical pixels against the existing 2 px limit.
The failure remains retained; no threshold was relaxed.

That step had a 6.3675 ms display interval versus a 6.25 ms animation-clock step.
Consecutive declared curve poses matched committed camera positions, and the
present indices were consecutive. This points to display-phase jitter amplified by
high scroll velocity rather than an extra missed refresh at that step. It is still a
failed quality gate, not a claim of constant perfect motion.

Public preparation maximum 3.2471 ms; recording maximum 1.6352 ms.
The phase and descriptive motion reports retain all blocks, including the failing one.

## Sustained middle result

Prefix real-scroll-vector-chunks-middle, prototype enabled:

| Window | Missed refreshes | Largest display interval ms | Largest motion error px |
| --- | ---: | ---: | ---: |
| Cold 0 | 0 | 6.3364 | 0.2469 |
| Cold 1 | 0 | 6.3884 | 0.4105 |
| Warm 0 | 0 | 6.4277 | 0.5060 |
| Warm 1 | 1 | 12.4448 | 17.8604 |

Middle commit error is measured against animation-clock deltas; an observation
stamp occurs after variable render work and must not be mistaken for that clock.
Displayed error uses actual display timestamps. Pairs spanning a command revision
or geometry rebase are excluded from the constant-velocity calculation, with camera
clamps and moving coverage checked separately. The missed refresh and doubled
following step are retained as failures.

The old display-window analyzer asserted that wheel events must exist. It was
extended to accept middle captures only when their independent focus/turn proofs
are present. No fake wheel events were inserted. An initial generator quoting error
was corrected before compilation. Initial failed analysis logs are preserved.

## Correlated incident, not the largest unrelated frame

The warm-block-1 display miss is present row 2639; row 2640 was dropped. Row 2641
arrived after a 14.2992 ms producer-present interval and advanced two normal camera
steps over one subsequent displayed interval. Correlation is through the exact
PresentMon QPC timestamp and the matching render/present bracket.

Frame 2641:

- Pacing and sync: 9.1828 ms, including 2.0383 ms synchronization.
- Public preparation: 4.1507 ms; batch renderer preparation: 1.8284 ms.
- Gap before the measured batch renderer begins: about 2.29 ms.
- Recording: 0.7384 ms; submission: 0.2275 ms.
- Pending upload scope: 0.0043 ms. No chunk transfer occurred in this frame.

A separate worst-recording frame, present row 2691, took 4.0509 ms recording.
Its native recordRenderPass scope took 4.0414 ms and 13,648,280 thread cycles;
nearby ordinary recording scopes took roughly 0.45 ms and 1.5 million cycles.
This shows substantial CPU execution, so pure descheduling alone does not explain
that separate spike. It is not the same event as the missed refresh. An initial
exploratory JSON paired unrelated present rows with the worst-recording frame; it
is preserved under an explicit unrelated-presents filename and the standard artifact
now contains correctly correlated rows.

## Disabled control

Prefix real-scroll-vector-middle-control uses the same executable with chunking
disabled. All four windows had zero missed refreshes, maximum display interval
6.3915 ms and maximum motion error 0.4223 px. Preparation maximum 3.9814 ms;
recording maximum 1.0646 ms. No chunk records were produced.

One clean forty-second control versus one failure does not establish causality or
prove that the prototype alone caused the miss. Conversely, the lack of chunk work
in the failing frame does not exonerate earlier resource/visibility effects. These results do not yet justify promotion.

## Next attribution

Split before-render callbacks, scene preprocessing and node-updater work to explain
the 2.29 ms gap, and add draw/driver attribution for the independent recording spike.
The exact Qt 6.11.1 renderer source was obtained from the official Qt mirror tag:
[Qt 6.11.1 renderer source](https://raw.githubusercontent.com/qt/qtdeclarative/v6.11.1/src/quick/scenegraph/coreapi/qsgrenderer.cpp).
SHA256 e3d6da1304099d23d378e4554b260206aae4388d491479d2de2d14301f29d10c.
Its provenance is saved; it has not yet replaced a compiled renderer object. Older
6.10.1 sources found in another Conan cache are not authoritative for this build.

Keep failed captures, instrument the missing stage, then make the next optimization
from that evidence. Do not reintroduce per-frame pixel-rounding traversal or raise
performance thresholds to make these runs pass. No production rebuild/restart,
commit or push occurred in this input-stress investigation.
