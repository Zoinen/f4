# Small preprocessing dependency sets

## Result

A diagnostic prototype stores up to four dependency IDs inline, retaining Qt
hash sets for larger groups. Synthetic rebuild/path-update benchmarks improve,
live peak heap-backed dependency slots fall by about 94%, and the measured
before-body p99 is lower in both candidate runs. Native rendering and mutation
checks pass.

The complete scrolling comparison does **not** show a repeatable display win:
one candidate arm is worse than the first control, the other improves, and all
arms still fail the original display/motion gates. Keep the prototype optional
and unpromoted. Constant 160 Hz is not achieved.

The previous goal turn was progress: it implemented and verified deferred SVG
directory cleanup and rebuilt/launched the normal portable app. This turn
changes diagnostic code only. That normal app remains unchanged and running.

## Motivation and storage

Recent CPU traces place dependency-table insertion/path construction inside
slow preprocessing frames. The existing reverse table is
`QHash<QSGNode *, QSet<int>>`. On this Qt build even a one-ID set has a minimum
64-entry allocation. The cache's actual peaks are roughly 2650 ancestor keys
and 10700 edges, with about 177000 retained heap-backed integer slots.

[SmallDependencySet20261031.h](../../artifacts/SmallDependencySet20261031.h)
uses four inline integers, promoting to `QSet<int>` when a fifth distinct ID is
inserted. Existing graph squeezing can move a sufficiently small hashed set
back inline. Set uniqueness, copy/move independence, notification semantics,
entry/node identity, mutation fallback and the original graph limits remain
unchanged. The wrapper reports heap-backed capacity separately from its inline
object footprint: 48 bytes versus 8 bytes for `QSet<int>` on this build. A zero
heap-capacity report does not mean zero storage.

The first optional wrapper still used 48-byte bucket values when disabled and
made the synthetic control slower. That control was rejected for live use.
[PreprocessDispatch20261031.h](../../artifacts/PreprocessDispatch20261031.h)
instead chooses either the original cache implementation or the candidate in
separate namespaces. The original header is copied with only its namespace
renamed. The original graph retains actual `QSet<int>` bucket values. Both
arms share the same dispatch interface and executable; only
`F4_PROFILE_INLINE_DEPENDENTS` changes (default off).

This is an artifact-only Qt source/object experiment. The installed Qt SDK,
production archives, embedded payload, Go wire protocol and production defaults
are unchanged.

## Correctness and compatibility

Each mode passes 28 helper results: five new differential/copy/move/promotion
results, 14 existing bounded dependency-cache results and nine eligibility
results. Checks cover random set mutation against `QSet`, duplicates/removal,
copy/move independence, repeated promotion/squeezing, retained-capacity bounds,
targeted and unrelated notifications, reparenting, address reuse, callback-time
mutation, nesting, membership changes, overflow and updater fallback.

Both modes then pass three real preprocessing mutation results and all 45 native
rendering results, zero skips, on D3D11 at actual DPR 1.75. The native mutation
fixture reads the dispatch scope's original snapshot/position and verifies cache
hits, topology and lifetime mutations. Tests exercise the new node/renderer
objects rather than silently using the stock archive's preprocessing path.

Fifteen rendered window PNGs match exactly across modes, as do two generated
PNG inputs. The small-caption fixture checks the 16 named text/image/effect
leaves for whole physical scene origins and unscaled unit vectors; its thin
caption strokes were also visually inspected. No static UI layout changes.
Three static import-isolation results and the Windows system-DLL audit pass.
This is not a claim of a completed portable rollout or a green full UI suite.

Two build/fixture issues were corrected and retained as evidence. Windows RPC
headers define the token `small`; the original member name collided in the real
host build, so it was renamed `inlineValues`. The differential test now includes
the Windows/RPC headers before the candidate header. The mutation fixture first
failed to save a PNG because its old relative output directory did not exist
under the new test working directory. Outputs now go to each mode's directory;
the pixel/behavior assertions are unchanged. The initial native link recipe also
assumed preprocessing override objects already existed in every test response
file; where absent, the verified objects are explicitly prepended.

## Synthetic benchmark

This CPU-only benchmark constructs 384 preprocessing leaves, 2386 ancestor keys
and 9600 dependency edges. Node construction is outside the measured regions.
It measures full graph construction/destruction and repeated eight-path
retirement/reconstruction. It is a synthetic topology near the observed graph
size, **not a replay of captured mutations or a frame-time benchmark**.

The final order is original / disabled wrapper / candidate / candidate /
disabled wrapper / original, using the same executable for wrapper/candidate
and a separate exact-original-header executable. Each rebuild trial has 200
samples and each eight-path trial 2000, after warmup. All six outcome checksums
match. The final run follows the Windows-header correction.

| Median of trial medians | Exact original | Disabled wrapper | Candidate |
| --- | ---: | ---: | ---: |
| Rebuild/destruct graph | 1.6819 ms | 1.8725 ms | 0.8734 ms |
| Eight path replacements | 25.10 us | 32.30 us | 18.35 us |
| Heap-backed integer slots | 160768 | 160768 | 13312 |

Candidate reductions versus the exact original are about 48% and 27%. The
disabled-wrapper overhead is why the live control uses the original graph
through dispatch instead. These numbers alone do not establish smooth scrolling.

## Live A/B/B/A comparison

Cases `real-scroll-smalldepsr2-{a1,b1,b2,a2}-20261031-wheel-masonry` use the
same pinned host. Real Downloads, catalog 192 including parent, Masonry density
152, 3840×2160, actual 160 Hz/DPR 1.75, new process and empty isolated cache per
arm. OS caches are not flushed. Existing merged-uniform sharing stays enabled
in **all** arms, sparse mode stays off and frame latency stays 2. The Go launcher
is pinned unchanged across the matrix rather than replaced by the recently
rebuilt portable launcher. Its explicitly selected native host is the new one.

All four original capture-validity checks pass. Active wheel inputs are
1728 / 1700 / 1755 / 1709, with stable native foreground ownership during each
measured window. No CPU/paging trace, builds, tests, screenshots, accessibility
queries or offline analysis runs during gestures. Raw packets are archived
after each completed analysis with byte/hash/location receipts. The second
direction is not a verified-warm claim.

An initial A1 attempt failed before app launch/input because `Get-FileHash`
was unavailable in the launcher's PowerShell environment. It has no performance
packet and is retained separately. The existing .NET hashing helper replaces
that cmdlet, and fresh `r2` case/cache/observer names prevent reuse of its state.
There is no retry of a rejected performance result until a favorable one occurs.

| Arm | PresentMon target rows | Inputs | Peak heap slots | Bucket bytes | Path builds / retirements | Full graph resets |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| A1 original | 3459 | 1728 | 176832 | 8 | 35182 / 31566 | 12 |
| B1 inline | 3402 | 1700 | 9984 | 48 | 28245 / 24752 | 12 |
| B2 inline | 3421 | 1755 | 9984 | 48 | 21025 / 17545 | 12 |
| A2 original | 3412 | 1709 | 176640 | 8 | 31574 / 28033 | 12 |

The PresentMon row counts are independent of preparation pass counts and come
from `abba-comparison.json`. Path totals include startup/loading and differ
between arms. The roughly 94% heap-slot reduction is repeatable, but the larger
inline object and other graph allocations must be included in any complete
memory accounting. There are no graph failures/capacity fallbacks in any arm.

| Arm/window | Before-body p99, ms | Preparation p99 / max, ms | Display misses >9.375 ms | Display max, ms | Motion error max, physical px |
| --- | ---: | ---: | ---: | ---: | ---: |
| A1/first | 1.5034 | 3.6984 / 5.9639 | 27 | 12.6464 | 120.5598 |
| A1/second | 1.1943 | 3.8447 / 5.3965 | 36 | 12.8749 | 127.9816 |
| B1/first | 0.9945 | 3.6250 / 5.8297 | 43 | 12.7534 | 117.8953 |
| B1/second | 0.9828 | 3.5523 / 7.4796 | 38 | 12.7830 | 115.9199 |
| B2/first | 0.9856 | 3.6975 / 7.6106 | 23 | 12.6095 | 105.9819 |
| B2/second | 0.5432 | 2.0112 / 3.4223 | 10 | 12.5548 | 112.9815 |
| A2/first | 1.3094 | 3.2546 / 5.8784 | 20 | 12.5324 | 92.6060 |
| A2/second | 1.1269 | 4.2198 / 9.0513 | 47 | 12.6915 | 116.8427 |

The before-body scope includes more than dependency-cache work, and different
membership/loading churn prevents attributing every time difference solely to
this container. Candidate before-body p99 values are lower in all measured
windows, but overall preparation/display results are mixed. Control misses total
130 and candidate misses 114; that aggregate does not establish a repeatable
display gain when B1 regresses and B2 improves. All original motion/display gates
remain unchanged and fail. All eight measured windows have zero coverage holds.
Complete cold/verified-warm/middle acceptance remains
unfulfilled.

## State and next target

Keep the compact dependency representation available for subsequent comparisons;
do not promote it as the scrolling fix. Further work should separate the
remaining render-list, material and geometry-transfer costs from this improved
preprocessing portion, retaining the complete acceptance gates.

The owned diagnostic tree has stopped and its finite display hold is released
and terminal. Cleanup reads the archived packet receipt after analysis, rather
than assuming the original raw packet remains on D:. The normal worktree app
remains the previous turn's verified portable build (Go PID 23488, Qt PID 84780).
No product source, SDK archive, embedded payload, commit or push changes here.

Evidence: `profiler/experiments/masonry-small-dependency-sets-2026-10-31/`.

> AI assistance has been used to create this output.
