# Alignment handle experiment — 2026-10-19

## Decision

Reject both alignment-binding candidates and restore the verified coherent-row
implementation. The cached candidate slightly reduces allocation traffic but
does not establish smoother scrolling; its clean trial includes a 31.2617 ms
display interval. Constant 160 Hz remains unachieved. This iteration makes
diagnostic progress; no performance improvement or production rollout is claimed.
No commit or push was made.

## Evidence and implementations

A fresh QML profiler attach capture uses the current coherent-row host, Downloads
186 entries, density 152, actual 160 Hz and DPR 1.75, 30 ms boundary-bounce wheel
events, two four-second input windows and the normal 5 ms GC budget. Native
frame recording/census and UI Automation are disabled. The QML profile selects
memory/creation/binding/JavaScript/signal events. It perturbs execution and is
used for allocation attribution, not display-cadence comparison. Both sustained
contentY-change clusters select approximately 8.27 seconds of activity including
the animation tails; this is not exact native QPC input-window alignment.

The largest attributed small-allocation site is iconPixelAlignment, 2,571 events
and 246,816 bytes. Attribution assigns positive small allocations to the
earliest-ending active QML range; it is a proxy rather than a C++ allocation
stack. Overall allocation traffic must be checked because a refactor can simply
move allocations to a different binding.

The first candidate replaces helper-returned alignment wrappers with attached
object lookups in each x/y binding, and binds native enabled state to the existing
geometry-animation/visibility predicate. The regression first fails on the old
implementation in Grid and Masonry: JavaScript ignores offsets during tile
animation, but native alignment remains enabled. The candidate suspends native
alignment and applies zero offsets during animation while restoring at rest.
No layout arithmetic, DPR, source request, GC budget or native traversal is added.

Repeated attached lookups allocate wrappers in the x/y expressions. The uncached
candidate increases overall allocation traffic and is rejected. A second
candidate retains one alignment handle per visual and reads its cached offsets;
this avoids lookup-wrapper churn but retains more objects during GC.

| Profile activity sample | Positive small allocations | Allocated bytes | Reclaimed bytes |
| --- | ---: | ---: | ---: |
| Coherent-row baseline | 100,619 | 7,615,296 | 7,107,776 |
| Uncached attached lookups, rejected | 116,304 | 9,126,240 | 8,539,008 |
| Retained alignment handle, rejected | 98,989 | 7,451,200 | 6,975,392 |

Cluster lengths and row activity differ slightly. The cached sample is about
2.2% below baseline by allocated bytes, insufficient to assume a frame-cadence
benefit. The complete current-trace binding/source diagnosis is documented in
profile-report-f4-alignment-2026-10-19.md. No frame percentiles are inferred from
QML range totals, and filtered allocation deltas are not total process heap.

## Correctness checks

Both candidates pass the selected Direct3D11/OpenGL checks at actual 175% scale:
27 layout engine, 41 mode, 7 session, 3 embedded-import and 7 interaction/video
QtTest totals per backend (including setup/cleanup). These cover actual static
leaf origins/unit transforms and rendered output, all modes, source replacement,
recycled geometry, SVG retention, video badges, transition suppression and native
animation-enabled state. Passing checks do not establish performance. The
experimental enabled-state regression and its before failures are archived;
they are not left failing in the restored production suite.

## Clean cadence trial

Only the cached candidate reaches a clean display comparison. A is the immutable
coherent-row profile host; B is the cached-alignment host. A1/B1/B2/A2/A3/B3 use
fresh processes, the same folder/entries/density/DPR/display and 5 ms GC budget.
Each has two four-second wheel windows. Frame recording, QML profiler, census,
WPR and UI Automation are absent; no build/test/offline analysis overlaps capture.
Input/motion is validated, captions and top-level model/snapshot notifications
remain zero, and every run has one Composed: Flip swapchain. Display gaps require
both endpoints inside input windows.

| Run | Display gaps >9.375 ms | Maximum gap, ms | Block p99, ms |
| --- | ---: | ---: | --- |
| A1 | 4 | 12.5052 | 6.3541, 6.3314 |
| B1 | 2 | 12.5157 | 6.2979, 6.2884 |
| B2 | 6 | 18.7565 | 6.3342, 6.3523 |
| A2 | 2 | 12.5010 | 6.3455, 6.2947 |
| A3 | 4 | 12.5077 | 6.3070, 6.3485 |
| B3 | 6 | 31.2617 | 6.3496, 6.3325 |

A totals 10 long intervals over 24 seconds, B totals 14. These small counts
do not prove a causal regression, but do not support promotion. The exact
31.2617 ms clean-run outlier has no simultaneous scheduling trace; its cause
is unresolved. A separate trace cannot be used to assign its cause or dismiss it.

## Sampled follow-up

A distinct immutable candidate host/map captures bounded native GC, root,
incubation and preparation phases with WPR CPU scheduling and PresentMon.
This is instrumented attribution, not the clean comparison. Observers remain
below their bounds, the cold/warm engine identity is stable, and raw-QPC alignment
has 900 ns spread. GUI and render running times overlap; do not add them as
serial work or treat sampled symbols as exclusive timing.

Warm marking visits increase from 139,271 to 140,240 (about 0.7%) in separate
captures with one warm sweep and 14 drain batches each. Cold captures have three
sweeps each. Cached-candidate warm drain/sweep maxima remain 5.6222/5.4190 ms;
the alignment cache does not remove the GC constraint.

One cold missed interval ends at QPC ns 149007938803900: producer 11.3805 ms,
display 12.4799 ms, GC union 6.6954 ms, GUI running 10.6315 ms and render running
3.5895 ms, GPU active 0.4647 ms. Another cold interval ends 149012645680000:
producer 12.7976 ms, render running 7.2306 ms, GC zero and GPU active 0.4556 ms.
Warm misses ending 149027278210900 and 149029866205100 have zero GC, producer
9.0701/8.7852 ms, GUI running 3.1290/2.9510 ms and render running 3.3721/3.8662 ms.
The 31 ms clean outlier is not reproduced or explained by these samples.

## Restoration and next target

All eight modified QML files are restored byte-for-byte from the pre-experiment
snapshots. The test file is restored from the verified coherent-row archive after
reviewing its exact diff; only this experiment's additional animation assertions
are removed. Coherent row commits, lazy badges and earlier accepted work remain.
The normal host is rebuilt, system-only imports audited, embedded-only QML and
restored alignment/row-commit checks rerun on both backends. The payload is
regenerated and extraction tests/CGO_ENABLED=0 Go build rerun with the system
cache. Only the worktree launcher is restarted directly with --gui=qt; the
extracted child is hash-verified and startup stderr is checked. Canonical F4
is untouched. Candidate payloads are not installed.

The trace's larger binding fan-out is activePresentation -> thumbnailEnabled ->
bitmap source selection. Next measure whether parking/identity guards clear
bitmap sources and release/recreate ready textures unnecessarily. Evaluate
retaining the last valid bitmap source while presentation is disabled, publishing
the replacement only after coherent geometry, with strict old-entry pixel tests,
bounded pool memory and actual source/decode/resource counts. Do not apply this
blindly to vector leases or accept stale pixels. Render-resource churn and
atomic GC are stronger targets than another alignment getter refactor.

Sources for before/uncached/cached variants, failed/final checks, screenshots,
parsed QML profiles, compact native attribution, clean summaries and binary
hashes are retained in profiler/experiments/alignment-handle-experiment-2026-10-19
with a SHA-256 manifest. Raw .qtd/ETW/PresentMon CSV, binaries, libraries, build
outputs and generated payloads are excluded.

AI assistance has been used to create this output.
