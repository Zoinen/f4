# Masonry: bounded vector-owner resolution

## Outcome

This continuation makes a verified CPU improvement in vector-gate setup, but
**constant 160 Hz without spikes remains unachieved**. Four clean cold-wheel
captures reduce gate-setup medians from 0.0659–0.0740 ms to 0.0266–0.0288 ms.
The candidate eliminates most repeated ancestor walks. It does not fix the
strict scrolling gate: controls have 12 late display intervals, candidates 19,
and all blocks exceed the two-physical-pixel maximum motion error.

The resolution cache remains a diagnostic candidate pending further integration
and verification. No portable payload, installed SDK library or normal running
application is replaced; no commit or push is made. Verified-warm wheel and
quarter-height middle captures have not been rerun with this candidate. Those
requirements remain part of the goal.

The previous goal turn made progress by testing converter-resource reuse. This
turn tests two owner-lookup revisions and changes the next action through actual
CPU-cost, correctness and display evidence rather than a restated plan.

## From admitted-node shortcut to resolution cache

The earlier revision is preserved separately in
`masonry-vector-owner-lookup-2026-10-29`. It records only about 150,000 direct
hits per candidate process, leaving over 700,000 ancestor walks. Its existing
native tests pass, but the follow-up regression below exposes a broader
assumption that those tests did not cover. It must not be promoted as correct
for every possible batch topology.

The final diagnostic caches the result of **one actual ancestor walk for a
batch's first node**, including a negative result when no vector gate owns that
node. It does not cache every ancestor and does not repeat the rejected
per-ancestor hash-cache implementation. No traversal, sorting, window mapping
or pixel rounding is added to scrolling.

The direct-resolution index is capped at 4,096 entries per renderer thread.
When full it falls back to the original walk. The admission-membership index
remains separate. Neither positive nor negative resolution proves GPU residency
or bypasses full batch membership, upload budgets, source completeness or
presentation gates.

New gates, changed source generations and gate removal invalidate the topology
revision. The existing recursive `nodeWasRemoved` path forgets each node's
resolution before checking whether it has an admission record. This covers
negative records as well as admitted nodes during removal, reparenting and
address reuse. Revision wraparound clears resolution entries and resets stamps.
The index keeps no source lease, frame, GPU allocation or owning node pointer.

The feature flag is `F4_PROFILE_VECTOR_OWNER_RESOLUTION`. With it off, the
resolution index remains empty and the original ancestor walk executes. The
two arms share the same counters; counters publish once per gate-frame scope,
not atomically at every ancestor step.

## Regression caught before live comparison

The prototype initially populated direct owner answers from the existing
admission-membership record. That is too strong: a potentially merged batch's
membership record does not prove the nearest gate ancestor of every member.

A new test puts a node under gate B while giving it an admission record for
gate A. Against the prototype it returns **1 instead of 2** and fails. The
failing executable, source and output are retained under
`vector-owner-resolution-20261029/admission-proof-before`. The final cache
accepts answers only from an actual ancestor walk. Publishing admission
membership cannot populate or overwrite those answers. The test then passes.
This is a diagnostic-prototype defect caught before promotion, not a production
crash or a claim that the actual Downloads batches cross source roots.

Eight helper results pass, including direct reuse, nested-gate invalidation,
reparenting, address replacement, generation/removal, revision wraparound,
positive/negative results, capacity overflow fallback and admission/ancestry
separation.

A native rendered fixture passes three results at actual DPR 1.75. It first
primes a negative answer with no gate registered, then registers a gate on the
existing ancestor. It adds a nested hidden gate, removes it, reparents the leaf
between two roots and changes generation. At each stage it compares the actual
resolved owner and rendered pixels. It verifies the geometry leaf's scene-space
physical origin and unit scene transforms. Initial, nested-hidden and reparented
rendered images are saved and inspected. With the feature off, ownership/pixel
checks pass and only the explicit final direct-lookup execution check fails.

The final build additionally passes:

- 48 native scene results in each arm, zero failures/skips.
- 27 shader results and three embedded-only import results.
- Windows system-DLL-only dependency audit.

These checks use the installed Qt 6.11.1 static build and actual DPR 1.75. There
is no new or modified static product UI layout. This experiment verifies the
native D3D11 path; it is not a claim that other backends or packaged deployment
have been verified. Strong diagnostic objects override the renderer archive
member without replacing the SDK. Reversible source replacements are recorded.

## Live capture conditions

One executable, `artifacts/f4-qt-host-vector-owner-resolution-20261029.exe`,
is used in A1/B1/B2/A2 order. The owner-resolution flag is the sole arm variable.
The earlier private FFmpeg heap and thumbnail decoder-device reuse diagnostics
remain enabled in both arms; converter-resource reuse and the conversion lane
are absent. Existing renderer, upload, image/vector and media policies remain.

Actual Downloads, all 191 entries, Masonry density 152, actual 3840×2160 at
160 Hz and DPR 1.75, fresh isolated gallery caches zzr/zzs/zzt/zzu, and two
ten-second rapid-wheel blocks are retained. OS file caches are not flushed.
Normal cold scrollbar setup is used, without a warm-tour bypass. Display,
content, focus/input, feature-use, bounded resource and error guards pass. No
compiler, tests, offline analysis, ETW scheduler sampler or UI Automation
observer runs during the gestures. PresentMon measures display intervals;
native probes record preparation and motion. The largest preparation observer
sample is 125 microseconds in B1; the other runs report 31.9/38.7/42.7 microseconds.
No capture is excluded for its unfavorable performance.

Cases use `real-scroll-vector-resolution-{a1,b1,b2,a2}-20261029-wheel-masonry`.
All eight blocks have zero coverage holds.

| Arm | Block | Gate setup median ms | Preparation median ms | Preparation max ms | Display max ms | Intervals >9.375ms | Max motion error physical px |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| A1 control | 0 | .0740 | .7363 | 7.6508 | 24.9973 | 6 | 195.056 |
| A1 control | 1 | .0659 | .6795 | 5.3694 | 12.4361 | 2 | 112.072 |
| B1 cache | 0 | .0288 | .6966 | 6.7586 | 12.5087 | 3 | 89.734 |
| B1 cache | 1 | .0266 | .5863 | 9.2237 | 18.7689 | 6 | 104.050 |
| B2 cache | 0 | .0272 | .6507 | 6.0475 | 12.5027 | 2 | 90.741 |
| B2 cache | 1 | .0266 | .5646 | 9.7461 | 18.7373 | 8 | 113.200 |
| A2 control | 0 | .0736 | .7308 | 5.7814 | 24.9880 | 2 | 144.988 |
| A2 control | 1 | .0705 | .7110 | 9.8936 | 12.5167 | 2 | 105.134 |

The targeted median cost is lower in all candidate blocks. Candidate gate-setup
p99 values are .0819–.1207 ms versus .2187–.3280 ms for controls. Maximum gate
setup remains variable: B1 block1 reaches .8059 ms, so this is not a claim of a
hard bound derived from percentiles. Full preparation/display tails remain
variable as well. A smaller median is not a substitute for the unchanged
strict display/motion gates. These four runs do not establish a population-level
display regression or improvement.

Process-cumulative counters, including startup:

| Arm | Positive hits | Negative hits | Ancestor walks | Ancestor steps | Peak resolution entries |
| --- | ---: | ---: | ---: | ---: | ---: |
| A1 | 0 | 0 | 910,050 | 21,608,594 | 0 |
| B1 | 155,286 | 723,067 | 33,721 | 985,478 | 305 |
| B2 | 147,796 | 721,288 | 34,353 | 1,004,412 | 304 |
| A2 | 0 | 0 | 895,950 | 21,268,455 | 0 |

Neither candidate reaches capacity or bypasses insertion. This confirms that
negative results account for much of the previously repeated work. Counts are
not normalized per measured window or perfectly matched source completion.
Slow render-thread geometry-unmap records stay below .58 ms in these runs;
they do not explain every missed display interval. No scheduler attribution of
these particular missed frames is claimed.

## Remaining work

Keep the tested bounded-resolution candidate available as a CPU optimization,
with the proof separation and removal regressions intact. It has not passed
the full cold/warm/middle goal and is not integrated into the portable payload.
The separate preprocessing `isNodeBlocked` hotspot, material/geometry work,
initial video-conversion-context contention and wheel motion errors remain.
The previous CPU/scheduler trace is still the attribution source for those
paths; this experiment does not resolve them.

Sources, reversible hooks, final tests, the pre-fix regression, build receipts,
capture packets, presentation CSVs, screenshots and analysis are frozen under
`profiler/experiments/masonry-vector-owner-resolution-2026-10-29`.
Large snapshot files are losslessly compressed and external binaries/objects
are hashed. The normal worktree application remains alive with its original
binary hash; all owned diagnostic instances are stopped. The goal stays active.

AI assistance has been used to create this output.
