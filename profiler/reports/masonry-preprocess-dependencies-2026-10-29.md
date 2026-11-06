# Masonry: dependency-specific preprocessing invalidation, first prototype

Constant 160 Hz remains **unachieved**. This prototype is correct in the tested
mutation cases but regresses cold-scroll preparation. It remains diagnostic
and is **not promoted** to the production Qt library or embedded host payload.
No commit or push was made.

## Change and regression-first verification

The previous trace found repeated ancestor checks in `QSGRenderer::preprocess`;
the reason probe showed topology and subtree-block notifications dominating
cache invalidations. This prototype records reverse dependencies from ancestor
nodes to preprocessing entries. Local opacity/block notifications invalidate
dependent answers. Unrelated topology notifications retain existing answers.
Known-entry topology changes still discard the entire dependency graph, and
the next snapshot rebuilds it. Snapshot order/membership changes remain
conservative. That full rebuild is the principal unresolved design issue.

The table applies only to the exact default `QSGNodeUpdater` or Qt batch updater
types. Unknown updater overrides use the original query path. Forced updates
and notifications without provenance keep global invalidation. Callback-time
changes abort reuse for the rest of the current pass. The original Qt copied
snapshot and deleted-node skip guard remain unchanged. Renderer destruction and
updater replacement defer active storage destruction as before.

Bounds per renderer: 16,384 preprocessing entries, 32,768 ancestor keys and
131,072 dependency links; at most eight renderer records per thread. Overflow
uses original queries and suppresses repeated graph rebuilds until relevant
state changes. These are logical element bounds, not an exact allocator-byte
measurement. Notification counters accumulate in thread-local integers and are
aggregated after preprocessing, rather than performing an atomic per link.

Before implementation, the new regression runs against the exact old header:
`unrelatedNotificationsPreserveCachedAnswers` fails because unrelated changes
repeat both original ancestor queries. The failure output is preserved. The
candidate then passes:

- 11 dependency-cache helper checks, including unrelated and targeted changes,
  reparenting, equal-size membership replacement, destroyed-ancestor address
  reuse, callback mutation, root exclusion, forced fallback, exact updater type
  guards and bounded-link overflow without repeated rebuilds.
- All nine existing eligibility regressions, recompiled against the expanded
  cache state, including snapshot order, deletion, nesting, capacity and active
  updater replacement.
- Three real native preprocessing mutation results (including init/cleanup),
  exercising block/unblock, opacity, reparenting, destruction/reconstruction at
  the same address and disabling preprocessing. Actual DPR 1.75, rendered
  geometry checks and captures; the visible fixture was inspected.
- 48 native scene tests, 27 shader tests, three embedded-QML import-isolation
  results and the Windows system-DLL audit.

The initial broad native run omitted the test suite's existing initial-dynamic,
uniform-capacity/reuse and native-query flags. Its three failures correspond to
those missing flags and are preserved. Restoring the required environment gives
48 passes without changing the cache implementation or relaxing assertions.
No static UI layout was changed.

## Same-host cold ABBA capture

All four runs use `f4-qt-host-preprocess-dependencies-20261029.exe`. Only
`F4_PROFILE_PREPROCESS_DEPENDENCIES` changes. The current eligibility cache,
vector-owner lookup, AV heap/device reuse and original resource limits remain
enabled in both arms. Normal thumbnail workers are used; no serial conversion
lane or startup-context prewarm is enabled.

Real Downloads, 191 entries, Masonry density 152, 3840×2160 / 160 Hz / actual
DPR 1.75. Each process has an initially empty isolated application cache; OS
file cache is not flushed. Two ten-second rapid-wheel blocks per process.
Order A1 control, B1 candidate, B2 candidate, A2 control. All capture-validity
guards pass, with 1,676 / 1,723 / 1,701 / 1,717 active wheel events. No tests,
builds, offline analysis or UI Automation observation run during gestures.

| Case | Block | Preparation median/max, ms | Display max, ms | Gaps >9.375 ms | Motion max, physical px | Holds |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| A1 control | 0 | 0.6948 / 8.8465 | 12.5211 | 3 | 87.193 | 0 |
| A1 control | 1 | 0.5625 / 6.4047 | 12.4619 | 1 | 4.488 | 0 |
| B1 candidate | 0 | 1.0881 / 7.5866 | 12.6528 | 2 | 77.532 | 0 |
| B1 candidate | 1 | 1.0634 / 11.1482 | 18.6410 | 7 | 108.309 | 0 |
| B2 candidate | 0 | 1.1077 / 7.9016 | 14.0099 | 4 | 100.288 | 0 |
| B2 candidate | 1 | 1.0722 / 10.4117 | 18.3123 | 9 | 169.147 | 0 |
| A2 control | 0 | 0.6800 / 7.6209 | 25.0036 | 2 | 16.115 | 0 |
| A2 control | 1 | 0.5926 / 8.5973 | 12.6040 | 3 | 81.893 | 1 |

Total observed missed intervals: control **9**, candidate **22**. Neither arm
passes the full acceptance gates. The single control coverage hold is retained,
not excluded from analysis. Motion uses the unchanged original PresentMon-based
analysis; prior notification-time jitter findings do not reclassify these runs.

The before-body scope median rises from **0.1998–0.2531 ms** in control blocks
to **0.6659–0.6956 ms** in candidate blocks. The repeated cost is not removed
merely by reducing original ancestor queries:

| Counter | A1 | B1 | B2 | A2 |
| --- | ---: | ---: | ---: | ---: |
| Original queries | 712,103 | 625,314 | 630,319 | 715,973 |
| Cached answers | 389,575 | 489,757 | 465,250 | 374,556 |
| Graph rebuilds | 0 | 1,835 | 1,874 | 0 |
| Ignored unrelated notifications | 0 | 9,877 | 9,266 | 0 |
| Targeted entries invalidated | 0 | 9,209 | 9,559 | 0 |
| Mid-pass invalidations | 0 | 6 | 12 | 0 |

Candidate peak tables are small: 2,667/2,535 ancestors and 10,711/10,044 links.
There are zero overflows, capacity fallbacks or registry bypasses. Totals include
startup and loading. Full rebuilds still occur in more than half the passes;
their path reconstruction, hash insertion and allocation are a plausible cause
of the measured added cost. A new CPU trace was not collected for this version,
so the entire delta is not attributed exclusively to one function.

## Next implementation direction

Retain entries by stable node identity instead of coupling all dependencies to
the current QSet order and size. A changed/removed preprocessing leaf should
unlink only its own ancestor edges. Reparented ancestor subtrees should retire
only their dependent entries. Preserve unaffected entries across membership
changes, with exact removal/address-reuse semantics and explicit fallback for
unknown updater behavior. Add a regression demonstrating that a known-entry
replacement does not force original queries for unrelated leaves before making
that change. Do not repeat full-table reconstruction under a different name.

The separate driver/Map waits remain unresolved. Any faster cache still needs
cold, verified-warm rapid-wheel and quarter-height middle measurements with the
original display, maximum motion and zero-hold requirements.

## Evidence and state

Frozen source, tests, receipts, capture packets and analyses:
`profiler/experiments/masonry-preprocess-dependencies-2026-10-29`.
Executables/objects remain at externally hashed paths. Inactive executable and
completed packet NTFS compression was checked with before/after SHA256 equality.
The normal worktree app PID 43936 remains alive; only the diagnostic launcher's
exact executable tree was stopped after the final capture.

AI assistance has been used to create this output.
