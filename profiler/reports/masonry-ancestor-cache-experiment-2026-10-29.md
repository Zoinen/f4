# Frame-local ancestor-block cache experiment

2026-10-29, `zoin_branch/cached-pixel-grid`. The preceding goal turn made
progress through upload-stage attribution and a tested queued-update experiment.
This continuation tests repeated ancestor-block checks identified in sampled
render-thread CPU stacks. Constant 160 Hz without spikes remains incomplete.
The cache is **not promoted**: its bookkeeping does not establish a reliable
scene-preparation or display improvement. Production sources and the embedded
portable payload are unchanged; no commit or push is made.

## Contract and bounded implementation

Qt requires node changes to notify connected renderers through `markDirty`;
subtrees may be blocked and preprocessing may change the graph.
[Qt's node contract](https://doc.qt.io/qt-6.11/qsgnode.html) and the
[6.11.1 notification implementation](https://github.com/qt/qtdeclarative/blob/v6.11.1/src/quick/scenegraph/coreapi/qsgnode.cpp#L1239)
are the primary references. The checked-out `SceneTransformNode` marks
`DirtySubtreeBlocked` whenever its blocking value changes.

The diagnostic cache exists only inside one default D3D11 renderer preprocess
loop, ending before accumulated node-state updates. It stores both blocked and
unblocked ancestor results, excludes the specified root, and bypasses reuse
when another root is requested. Every root notification invalidates every
currently active cache before dispatching any renderer callback. Each new pass
uses a fresh generation; nothing survives a frame boundary as a valid result.

Storage is four thread-local tables of 4,096 entries (256 KiB total table entry
storage per thread), with at most eight hash probes. A 64-node temporary path
uses stack storage. Excess nesting, lookup saturation and deeper paths safely
fall back to the original traversal. There is no per-frame heap allocation,
new setting, QML layout change or altered rendering/admission budget. Aggregate
diagnostic counters are published per pass, not per ancestor step.

## Native regression exposed an invalidation-order error

The first prototype cleared results in `QSGRenderer::nodeChanged`. Helper tests
passed, but the real native mutation fixture crashed in `buildRenderLists`.
Inspection established why: the batch renderer checks `isNodeBlocked` while
handling an added node **before** calling its base callback. A cached blocked
result could therefore prevent re-adding shadow nodes during an unblock.

The fixed prototype invalidates at `QSGRootNode::notifyNodeChange`, before any
renderer handles the notification. The original failing executable, logs and
debugger output are retained. This is a defect caught and fixed in the
experiment, not an application crash being attributed to production.

The focused native fixture runs inside Qt's actual preprocess loop. It primes
the cache, changes blocking and opacity, reparents a geometry leaf and destroys
and reconstructs a node at the same address. It checks current traversal
results after notifications, rendered foreground/background pixels, actual DPR
1.75, scene-space physical origin (14,14) and unit scene transforms. The control
passes rendering/mutation checks but fails the assertion that the cache was
active; the corrected candidate passes all three results including setup and
cleanup. The rendered image was inspected. The broader native suite passes
48 results with zero failures/skips, before and after the hash revision.

Four embedded-import checks and the Windows system-DLL-only audit pass for each
successful host build. These diagnostic host overrides are not portable rollout
verification and do not replace an embedded-payload build.

## Hash regression and an attribution correction

The first hash uses low address bits. Its live runs record 249,855 and 374,046
failed bounded insertion probes. A new deterministic test constructs 32 actual
nodes spaced 64 KiB apart. Before the fix it records 24 saturated insertions
and fails; mixing upper address bits makes the test pass. All 13 final helper
results pass, covering this regression, shared ancestors, root exclusion,
blocking changes, reparenting, nested frames, replacement across passes, deep
fallback and generation wraparound.

The hash fix **does not eliminate live saturation**: revised runs still record
315,442 and 356,815 failed insertions. I initially described the table as having
spare capacity, but occupancy was not measured, so that explanation was too
strong. The aligned-node regression proves a hash weakness; it does not prove
the live pressure comes from that same weakness. Occupancy and table pressure
must be measured before any capacity increase is justified.

## Eight valid rapid-wheel captures

All runs use real Downloads, 191 entries, Masonry density 152, actual
3840×2160 / 160 Hz / DPR 1.75, fresh application caches and two ten-second
rapid wheel blocks. OS caches are not flushed. Standard sources, hardware media
decoding, vector content, clipping and rendering remain enabled. The original
node-walk counter writes and detailed RHI recorder are disabled. All other
backend policies and capture guards match. No compiler, tests, offline analysis,
UI Automation observer or ETW sampler runs during the gestures.

Each four-run group uses control/candidate/candidate/control in the **same
executable**, changing only the diagnostic cache flag. The first group uses the
initial hash; the second uses the mixed hash. All activity, target PID, actual
display, input count, capacity and feature-use guards pass. No capture is
excluded because of an unfavorable performance result.

| Hash / arm | Preparation median, blocks 0 / 1 ms | Preparation p99, blocks 0 / 1 ms | Preparation maximum, blocks 0 / 1 ms | Misses, blocks 0 / 1 |
| --- | --- | --- | --- | --- |
| Initial / A1 | .8380 / .7982 | 3.3597 / 4.1806 | 8.3127 / 12.7574 | 7 / 12 |
| Initial / B1 | .8513 / .7808 | 3.7581 / 4.2213 | 5.6826 / 6.9707 | 7 / 4 |
| Initial / B2 | .8613 / .7688 | 3.7972 / 4.6990 | 8.9394 / 6.3581 | 4 / 4 |
| Initial / A2 | .8354 / .7613 | 3.7950 / 4.2830 | 8.8883 / 7.8828 | 3 / 8 |
| Mixed / A3 | .7930 / .6088 | 3.6569 / 2.6002 | 7.6356 / 5.1394 | 10 / 0 |
| Mixed / B3 | .8722 / .7349 | 3.3364 / 2.9662 | 5.7266 / 5.6421 | 4 / 0 |
| Mixed / B4 | .8388 / .7808 | 3.2965 / 3.9930 | 6.9660 / 6.4504 | 5 / 4 |
| Mixed / A4 | .8470 / .7728 | 3.7497 / 4.5271 | 6.6104 / 10.0818 | 4 / 5 |

A miss remains a display interval above 9.375 ms. Every block has zero coverage
holds but fails the maximum 2-physical-pixel motion requirement. Maximum motion
errors range from 5.9608 to 193.0128 px. No threshold is relaxed and no
warm-only subset substitutes for these cold/loading results.

Feature counters are cumulative snapshots, not restricted to input windows.
Initial B1/B2 record 1,453,701/1,457,746 ancestor-cache hits across
1,689,396/1,735,798 queries, and 9,352,933/9,665,337 walked nodes. Mixed B3/B4
record 1,454,805/1,452,321 hits across 1,713,576/1,710,285 queries, and
9,565,430/9,523,462 walked nodes. No deep-path fallback or in-pass notification
invalidation occurs in these live folder runs. The separate mutation test
establishes notification correctness; zero live notifications is not a claim
that scene changes never occur outside the cached preprocess interval.

## Why it remains unpromoted

The cache clearly reuses results, but its own lookup/insertion work consumes
the potential gain. In the mixed group, work before the renderer body has
medians .3463/.3004 ms for B3 and .3351/.3063 ms for B4, compared with
.2864/.2369 for A3 and .3127/.2890 for A4. This phase includes the targeted
preprocessing as well as other work; it does not establish a reduction in the
targeted aggregate. Candidate display misses sometimes improve, but preparation
medians and tails are mixed, and all strict motion gates fail.

The result rejects promoting this implementation based on cache-hit counts.
If revisiting it, measure occupancy before changing table size, and retain the
early notification and preprocess-lifetime regressions. The broader goal also
still includes the separately proven buffer-update/submission waits and
material/resource preparation costs; this cache does not address them.

## Reproduction and preserved evidence

Scripts are `build-blocked-ancestor-cache-20261029.py`,
`build-blocked-ancestor-native-20261029.py`,
`capture-blocked-ancestor-cache-20261029.py`,
`analyze-blocked-ancestor-cache-20261029.py` and
`analyze-blocked-ancestor-mixed-hash-20261029.py`, under `artifacts`.

Capture stems are `real-scroll-blocked-ancestor-{a1,b1,b2,a2,a3,b3,b4,a4}-20261029-wheel-masonry`;
fresh caches are zj through zq in that order. Compact evidence is in
`blocked-ancestor-cache-20261029`: `first-hash-live-comparison.json`,
`mixed-hash-live-comparison.json`, corresponding phase analyses, helper and
native logs, the aligned-node baseline failure, first-hash header, original
Qt source URL/SHA256, source hooks and build receipts. The first-hash host is
preserved under its own filename. The original flawed native executable and
debugger trace remain available; map offsets were used diagnostically and are
not claimed as a separately byte-verified sampled CPU attribution in this turn.

Space recovery uses lossless NTFS compression of completed JSON files. The
first-hash captures have before/after SHA256 checks. Raw traces and compiled
outputs remain local and hash-referenced by the snapshot. The normal worktree
app stays alive; completed diagnostics are stopped only by exact launcher path.

AI assistance has been used to create this output.
