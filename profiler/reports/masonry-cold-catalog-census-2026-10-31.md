# Masonry cold catalog publications, 2026-10-31

## Scope and outcome

The constant 160 Hz goal remains unachieved. Three diagnostic-only cold-start
scroll captures were taken in the current worktree, using the actual Downloads
folder (192 rows), Masonry density 152, 3840×2160, 160 Hz and DPR 1.75. Each
started with an empty isolated gallery cache; the OS file cache was not flushed.
Both panels displayed Downloads. These are startup/first-scroll stress cases,
not cold folder entry in an already initialized application.

The first two counter captures had no display misses, but the third reproduced
three. No application optimization or production rollout was made. Variable
startup admission timing matters: c1/c2 had 48 cached frames at input start;
c3 had only two, with 25 requests pending. Do not use the earlier-starting c3
as a controlled comparison of instrumentation versions or as proof of a
regression caused by the extra counters.

| Case | Recorded spans | Overflow | Cached frames before → after | Display gaps >9.375 ms | Dropped presents | Maximum display interval |
|---|---:|---:|---:|---:|---:|---:|
| cold-c1 | 42,236 | 0 | 48 → 119 | 0 | 0 | 6.4897 ms |
| cold-c2 | 53,901 | 0 | 48 → 121 | 0 | 0 | 6.3983 ms |
| cold-c3 | 50,119 | 0 | 2 → 118 | 3 | 0 | 56.2647 ms |

The 56.2647 ms display interval straddles input start. It is retained, but its
whole duration cannot be attributed to wheel input. Two further misses occurred
after input start: 24.9851 and 25.0042 ms. Corresponding application
present-to-present envelopes were 25.4072 and 16.4646 ms; these are distinct
clocks/metrics and must not be silently substituted for display intervals.
Coverage holds were zero. c2 and c3 each retain one unmatched native commit.
Native matrices are not rendered-pixel or wheel-curve validation.

## Instrumentation

The host uses the current production renderer and animation probe, with
diagnostic object overrides for catalog data/snapshot/facade access, catalog
transactions and Masonry model-delta handlers. A fixed 131,072-entry array holds
GUI-thread begin/end timestamps, owner identity and method arguments. There is
no per-call file logging or growing trace container. It is serialized after
input ends. Startup spans are retained separately from spans during input.

v2 additionally separates materialized-row updates, image-count refresh,
thumbnail planning and immediate lightweight rewrap. v3 separates the latter
into ready-row geometry commit, rewrap and thumbnail planning. No behavioral
changes, decoder suppression, visual pruning, thread-affinity changes or SDK
modifications were used. Counter overhead is not independently calibrated;
these are attribution captures, not zero-overhead acceptance runs. Durations
are elapsed inclusive spans, not exclusive CPU time. Nested methods overlap and
must not be added together.

The Computer Use helper was absent at before/after checks. No Computer Use
client was reconnected. No kernel trace ran. Only this task's preceding
diagnostic processes were replaced; the ordinary worktree app remained alive.

## What changed the investigation

1. In c1, a 64-row `applyCatalogRows` call took **8.1771 ms**, including
   **7.759 ms** in its synchronous layout notification. It contained 396 role
   reads, six snapshots and four new facade constructions. It updated rows
   128–191, while snapshot reads caused by the work included currently visible
   rows 88–93. A batch affecting offscreen rows can therefore perform visible
   layout work synchronously.
2. c2 localized the corresponding **7.3527 ms** update: its notification took
   6.9604 ms, of which **6.7656 ms** was `flushLightweightRewrap`. Materialized
   state refresh was .1916 ms, image counting .0056 ms, and the subsequent
   delta-specific thumbnail plan .0007 ms. This identifies the immediate flush,
   rather than ordinary role lookup, as the large branch in that sample.
3. c3 split that branch. Across 34 flushes, ready-row geometry commit had a
   **.0894 ms maximum**, rewrap **4.9717 ms**, and thumbnail planning **1.1332 ms**.
   The rewrap maximum occurred before input; during input it reached 1.1975 ms
   in the last 64-row transaction. These are different events from the c2
   6.7656 ms flush; do not claim the finer probe decomposes that earlier event.
4. The first definite post-start c3 presentation gap includes catalog batches
   for **both panels**, lasting **5.6149 and 3.9824 ms**. Their respective
   synchronous layout notifications reached 4.9760 and 3.3257 ms. Each created
   25 facades. The next gap includes child-directory catalog publication and
   rewrap. The overlap shows application work on the critical interval; without
   scheduler sampling it does not explain every millisecond or prove sole
   causation.
5. Ordinary snapshots are relatively small in these captures: 5,488–5,721 calls
   take about 48–51 ms over startup plus 30 seconds, with individual maxima
   below .156 ms. Existing-facade access is also small. A separate visual-row
   publication still reaches **5.6125 ms** in c3, so synchronous property
   notifications/binding execution remain relevant. An elapsed span alone does
   not distinguish GC, binding execution, scheduling or other work beneath it.

Source inspection confirms `flushLightweightRewrap` always calls `rewrap(false)`
and viewport thumbnail planning after `commitReadyMasonryRows`, irrespective of
that function's result. The scheduled variant already skips when no rows can
commit. This difference is a candidate for investigation, **not yet a proven
redundancy**: semantic changes may require rewrap even with unchanged image
dimensions. Skipping solely on the ready-row return value would be unsafe.

The proposed next investigation is to distinguish real geometry/semantic
changes from notifications that only refresh visual metadata, then avoid or
coalesce only the unnecessary synchronous work. Preserve atomic row geometry,
viewport anchoring, thumbnail/viewer priorities, folder previews and SVGs. Do
not remove the actual UI work or narrow the acceptance scenario to make it pass.

## Verification and evidence

The census analyzer has five passing regressions covering nested inclusive
durations, overflow, unfinished spans, startup/input separation and empty data.
All three hosts passed the Windows system-DLL import audit and retain Windows
GUI subsystem headers. QML resources/layouts were not modified; no new embedded
portable payload was built or claimed validated.

Raw capture data remains in
`C:/Users/xs/AppData/Local/Temp/f4-current-cold-motion-20261031/cold-c1`, `cold-c2`
and `cold-c3`, including the full counter buffers, native motion, all PresentMon
rows, admissions, identities and observer checks. The curated evidence manifest
is [here](../experiments/masonry-cold-catalog-census-2026-10-31/manifest.json).
Raw traces and executables are excluded from that curated copy. Three closed
diagnostic executables were relocated to verified archives with original paths
preserved as symlinks to make space; no captured bytes or user files were
deleted. The normal app and SDK remain unchanged.

Remaining goal acceptance includes initialized-app cold folder entry, full
wheel physical-motion/curve checks, rendered-pixel verification, exact render
wait attribution where needed and sustained no-spike delivery. No commit or
push was performed.

AI assistance has been used to create this output.
