# Masonry: typed scene discovery registry

## Outcome

Constant 160 Hz remains unachieved. Typed scene discovery consistently lowers
median `postSync` cost and reduces cold display misses in this A/B/B/A series.
It preserves direct traversal results through lifecycle and hierarchy changes.
However, the candidate middle-scroll capture contains three genuinely dropped
presents, so this remains a diagnostic prototype with no production rollout.

The previous goal turn made progress by rejecting the transform-only cache. This
turn retains the original Qt transform mapping and instead removes repeated
QObject type discovery and vector-subtree searches. It implements and tests the
registry, verifies it during real scrolling, measures cold, fully verified-warm
and quarter-height middle workloads, and diagnoses the failing middle capture.
No product source, installed Qt archive, portable payload, commit or push changes.

## Implementation

The two existing item constructors register `GalleryDelegateItem` and
`GalleryVectorUploadGate` instances in bounded typed tables. QObject destruction
removes entries; weak state references allow the registry itself to be destroyed
before an item. No strong item ownership is introduced. Registrations from the
wrong thread fail closed. Registration and reads are mutex-protected; discovery
borrows the tables only during Qt's GUI-blocked synchronization stage.

At each `postSync`, the prototype:

1. Retains Qt's existing ordered surface/branch child iteration for delegates.
2. Resolves delegate type by typed-table lookup rather than QObject metadata.
3. Walks the small typed gate list's **current** ancestors to find relevant roots.
4. Reads current child indices and sorts by depth and path to reproduce the old
   breadth-first gate order, including nested delegates and the existing rule
   that a gate stops traversal into its own subtree.
5. Resolves QSG nodes, readiness, geometry, retained state and Qt transforms fresh.

Thus this is a lifetime/type registry, not a persistent geometry or QSG-node
cache. Loader replacement, reparenting, stacking changes and source replacement
need no guessed generation key. The scratch path array is cleared before reuse;
old identities are never consulted on a later pass. The original ClearCamera
cleanup path is unchanged.

Limits are 16384 delegates, 4096 gates and a 64-edge discovery path. Scratch
capacity is explicitly bounded at 4096 path records. A registration capacity or
foreign-thread violation disables the fast registry for the remaining process
lifetime; the complete original path remains available. An overlong hierarchy
falls back for that pass. Partial membership is never published as complete.
This conservative fallback is a limitation of the prototype, not a promise of
automatic recovery after every capacity violation.

`F4_PROFILE_SCROLL_REGISTRY` defaults off. A separate
`F4_PROFILE_SCROLL_REGISTRY_VERIFY` compares types and ordered gate lists against
direct traversal. Both are diagnostic flags, not user settings. The previously
rejected inverse-only transform cache is absent from this host.

## Regression and build verification

The old repeated-discovery path visits 10200 subtree items in the budget test and
fails a zero-visit budget. The candidate returns the identical gate list with
zero subtree visits. Eight final QtTest results pass at actual DPR 1.75, covering:

- Reparenting and sibling-order changes over 100 rounds, nested delegates,
  nested gates, and exclusion of another surface's gates.
- Actual QML Loader teardown and replacement with a different component.
- Destruction and placement-new address reuse under a different root.
- Weak callbacks after registry destruction, both capacity bounds, overlong
  hierarchy fallback, and foreign-thread registration fallback.

The initial test compile failed on a missing Gallery include directory. The
failed log is preserved. The include was corrected and completed host objects
were hash-pinned before resuming. Final helper checks did not require rebuilding
unchanged host code. The original seven-result helper log is retained alongside
the final eight-result run.

Both native renderer arms pass 46 results with no failure or skip. Twenty-five
PNG files match byte for byte: 23 rendered captures and two fixture inputs.
The 175% leaf-origin and effective-transform checks remain enabled; the small
caption/thin-stroke capture was inspected. Import isolation passes three results,
and the Windows system-DLL audit passes. No static layout arithmetic changes.

In live validation, **266934** type/order/membership checks pass with zero
mismatches. All 1620 discovery passes use the fast path, with no capacity/depth
fallback. Oracle work is excluded from performance comparisons. The diagnostic
host is `artifacts/f4-qt-host-scroll-registry-20261031.exe`; its exact hash and
the four recompiled object/source identities are in the frozen build receipt.

## Isolated discovery measurement

A fixed fixture has 154 delegates, 11 gates and 317 items visited by the original
gate searches. It checks the same checksum in all arms, warms 100 passes, then
records 5000 passes per run. The fixture isolates discovery; it does not measure
QSG updates, real QML metadata complexity, scene submission or display delivery.

| A/B/B/A arm | Discovery median (µs) | p99 (µs) |
| --- | ---: | ---: |
| Original A1 | 11.9 | 15.0 |
| Registry B1 | 7.1 | 9.7 |
| Registry B2 | 6.9 | 9.3 |
| Original A2 | 12.0 | 15.4 |

This supplies an isolated discovery cost rather than attributing the entire
membership/configuration loop to searching. Live `postSync` also includes item
metadata, rectangle mapping and node configuration. Its remaining cost is much
larger than this simple C++ fixture.

## Cold A/B/B/A

All live arms use the same diagnostic host and pinned Go launcher, fresh isolated
gallery caches, real `C:\Users\xs\Downloads`, 192 entries, Masonry density 152,
3840×2160 desktop, actual DPR 1.75 and 160 Hz. Shared ancestry, retained geometric
storage and inherited-transform skipping are enabled in both arms; buffer
prewarming and the rejected transform cache are off. Registry enablement is the
differing flag.

No screenshots, UI Automation, builds or offline analysis run during gestures.
Native foreground observations cover the measured intervals. All wheel events
are active; counts are 1723 / 1757 / 1778 / 1756. Foreground observations do not
write during capture and report no overflow. Preparation-observer whole-capture
maxima are 54.8 / 47.3 / 72.8 / 66.8 µs, retained as measurement limitations.

| Arm | `postSync` median, directions 0 / 1 (ms) | `postSync` p99, directions 0 / 1 (ms) | Display max, directions 0 / 1 (ms) | Misses, directions 0 / 1 |
| --- | --- | --- | --- | --- |
| A1 original | 0.2626 / 0.2580 | 0.7306 / 0.8143 | 12.7019 / 12.5617 | 34 / 30 |
| B1 registry | 0.2129 / 0.1979 | 0.5367 / 0.5652 | 12.5857 / 12.6490 | 18 / 29 |
| B2 registry | 0.2047 / 0.1820 | 0.6207 / 0.5979 | 12.7007 / 12.5945 | 27 / 15 |
| A2 original | 0.2543 / 0.2471 | 0.7433 / 0.8455 | 13.0172 / 12.6910 | 30 / 58 |

Both candidate runs have fewer total display misses than either control in this
series. This is one A/B/B/A series, not a universal causal or statistical claim.
All cold runs still fail the unchanged display and motion gates. Motion maxima
are A1 117.33 / 107.65, B1 117.46 / 108.08, B2 123.81 / 121.59 and A2
132.12 / 114.36 physical pixels. Coverage holds are zero in all blocks.

Whole-capture gate-search visits fall from 472728 / 495576 in the controls to zero
in the candidates. The candidates still perform fresh ancestor walks and child
order reads: about 87000 parent steps and 70000 order scans per capture. These
are not included in the zero-subtree-visit claim. Final tracked state has about
214–232 delegates and 11 gates; there is no registry fallback or mismatch.

## Verified-warm wheel and quarter-height middle

Warm and middle captures first display and settle **all 192 entries**, preserving
normal visibility. Their receipts explicitly confirm the full visited-index
range and `all-entries-displayed-and-quiescent`; a cold reverse direction is not
relabeled warm.

| Verified-warm wheel arm | `postSync` median, 0 / 1 (ms) | Display max, 0 / 1 (ms) | Misses | Motion max, 0 / 1 (physical px) |
| --- | --- | --- | --- | --- |
| Original | 0.2198 / 0.2208 | 6.4189 / 6.4670 | 0 / 0 | 2.3268 / 3.3055 |
| Registry retry | 0.1780 / 0.1761 | 6.4137 / 6.4515 | 0 / 0 | 2.6555 / 2.6118 |

The first warm candidate attempt stopped at the free-space guard **before
launch**. Its failure logs and observer metadata remain. Older diagnostic files
were copied, hash-verified and replaced by checked junctions, then a new `b1r2`
capture ID was used. No failed result was overwritten or accepted as a capture.

The middle control passes the original strict analyzer: zero display misses and
coverage holds; display velocity error maxima 0.3738 / 0.7066 px; native velocity
error maximum 0.0007284 px, below the original 0.002 px gate.

The registry middle candidate fails the original continuity assertion
`Missing presented motion frame` at present indices 391→393. The original failure
is retained. Supplemental analysis finds **three actual dropped presents** at
indices 392, 2301 and 3507. Display maxima are 12.3762 / 12.5272 ms, with 1 / 2
misses. Native/display velocity metrics for this candidate are not accepted or
filled with zero after the continuity failure. Its `postSync` median is still
lower, 0.1801 / 0.1814 ms versus the control's 0.2208 / 0.2276 ms; that does not
make it a passing middle-scroll result.

The original complete-goal gates remain: no displayed interval above 9.375 ms,
maximum motion error at most two physical pixels, no coverage holds, and native
middle velocity error at most 0.002 px. Neither this prototype nor the full goal
passes them. Production promotion is therefore unproven.

## What the dropped-frame diagnosis changes

The failure is not merely absent probe data: PresentMon marks the intervening
rows `Dropped=1`. Around the gaps, preparation, recording and submission all
increase together on some frames. For example, frame 393 has 2.1917 ms preparation
(including 1.2958 ms alpha-material work), 1.1199 ms recording and 0.3496 ms
submission. Frame 3507 totals 10.5592 ms, including 2.0841 ms preparation and
0.7358 ms recording. These are not explained by a five-microsecond discovery
operation alone.

Ordering matters: present 391, whose displayed interval first reaches 12.3762 ms,
has ordinary application-side preparation 0.3137 ms, recording 0.2017 ms and
submission 0.1263 ms. The expensive later frames must not be casually labeled the
cause of an earlier displayed gap. CPU submission duration is not GPU completion
time. Expected `beginFrame` pacing is included in before-render spans, so their
roughly 5 ms baseline is not treated as an application CPU hotspot.

The all-process PresentMon rows contain 3200 presents from this diagnostic host,
3197 from DWM and one from Telegram during the measurement windows. The normal
worktree Qt process has **zero presents** there. This rules out competing presents
from the preserved normal F4 window in this capture; it does not rule out every
form of background CPU/GPU work. Hardware inspection identifies a Ryzen 9 5950X,
16 cores / 32 logical processors; no Intel P/E-core inference is warranted.

The next useful trace must correlate application CPU/scheduler activity, kernel
interrupt activity, DWM scheduling and GPU/presentation completion at these gaps.
Current phase probes do not distinguish those causes. Do not keep optimizing
only delegate discovery, assume a driver cause from elapsed spans, or relax
continuity/display gates to make the candidate pass.

The registry also adds constructor/destruction bookkeeping outside `postSync`.
Its allocations and callbacks could affect other work or shared heaps; lower
`postSync` timings do not exonerate the prototype. The next trace must retain
those call paths rather than presuming all remaining stalls are external.

## Preservation

Sources, helper/native results, matching rendered captures, cold/warm/middle
summaries, intentional baseline failure, failed build/capture logs, and dropped
present diagnostics are in
[the frozen manifest](../experiments/masonry-typed-scene-discovery-2026-10-31/manifest.json).
Large raw packets and CSVs remain hash-verified on C:. Exact owned diagnostic
process trees were stopped; the display hold reached its bounded terminal state,
and per-capture display/power guards remained active. Normal Go PID 23488 and Qt
PID 84780 were preserved. No performance threshold or rendering quality changed.

> AI assistance has been used to create this output.
