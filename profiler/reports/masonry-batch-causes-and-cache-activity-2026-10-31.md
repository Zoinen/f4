# Masonry batch causes and thumbnail-cache activity, 2026-10-31

## Decision

Investigate repeated cold thumbnail requests before changing batch granularity
or isolating the scrollbar. This pass adds observational diagnostic builds,
validated counters and two full captures; it introduces no production
optimization. Constant 160 Hz without spikes remains unachieved.

The cold and verified-warm scenes have comparable geometry traffic, but the
cold renderer takes longer to process it. Within the cold captures, intervals
with a quiet shared-thumbnail cache have preparation tails close to the warm
capture. The reverse cold pass admits 312 more cache misses while retaining
only one more frame. Identifying those requests is the next concrete target.
This is an association: scrolling position/content covary with cache activity,
and cache counters alone do not prove worker CPU contention or cancellation.

## Probe and verification

The initial `batch-invalidation-causes-20261031` host observes GallerySceneItem
blocking, Qt node changes, batch invalidation call sites, recycling, creation
and upload. `batch-matrix-causes-20261031` adds separate synchronous context
tags around the native viewport and scrollbar-thumb matrix changes. All normal
camera, media, geometry, clipping and rendering decisions remain intact.
The rejected prepared-root retention mode remains off; pipeline-key mode is
original, typed registry and uniform word hash remain on, uniform snapshot
observer remains off.

Counters are bounded to 16 pending renderers, 64 normalized owner categories
and 32,768 frame records. Renderer destruction clears pending state. Owner
names group instance suffixes rather than keeping a pointer to a QML object.
Counters record into memory; they do not write a log during measured gestures.
Overhead is included, so these are attribution captures, not an optimization
A/B throughput claim.

Validation in both enabled/disabled modes:

- 38 helper tests on the final probe, including bounded storage, immutable
  labels, nested source scopes, renderer separation and thumb/viewport tags.
- 46 native renderer tests and seven existing motion cases at DPR 1.75.
- Eight native callback-attribution motion cases. The added case verifies
  that a binding-triggered camera block labels recursive geometry removals,
  clearing the owner labels geometry additions, publication remains protected,
  and the actual visual leaf maps to physical (21,21) with unit scene vectors.
- 25 renderer-image pairs and eight motion-attribution-image pairs match
  byte-for-byte between modes.
- Three isolated-resource QML import checks and system-DLL audit.

The first source build attempted to call the private QQuickItemPrivate q_func;
it now uses QObjectData's public q_ptr. A helper needed its observer event call
qualified because QObject also has an event method. Failed compile logs were
preserved, and completed production objects were hash-checked before resuming.
Neither repair changes drawing behavior or an installed Qt package.

## Captures and acceptance

Actual Downloads folder, 192 entries including parent, Masonry density 152,
3840×2160 desktop, 160 Hz, DPR 1.75. The other panel remains present. Cold starts
from the normal initial viewport; warm completes the existing verified full
tour. No forced raster pins: both captures report zero pinned raster frames.
Both have the same 4339.5714 logical-pixel scroll extent. Two 30-second wheel
windows each, stable native foreground, no inactive wheel events. No UIA,
screenshots, build or analysis during measured gestures; no priority, affinity,
VM or external-workload change.

| Capture / pass | Preparation median ms | Renderer preparation p99 ms | Display misses >9.375 ms | Maximum motion error px |
| --- | --- | --- | --- | --- |
| Cold / 1 | 0.55495 | 3.4001 | 104 | 123.301 |
| Cold / 2 | 0.57810 | 3.7343 | 141 | 127.203 |
| Warm / 1 | 0.39865 | 1.5708 | 0 | 3.383 |
| Warm / 2 | 0.40515 | 1.6755 | 0 | 3.039 |

These p99 values use the renderer-phase summary. All windows have zero coverage
holds. All fail the unchanged full display/motion gate; the warm maximum motion
error still exceeds 2 px. No pair, missing presentation or outlier is removed.
The middle-motion acceptance scope remains required for a future candidate.

## What actually invalidates batches

Every measured frame's observed blocked-call count exactly matches the
independent existing Qt preparation counter. Geometry-source totals match
owner totals, upload totals match opaque plus alpha totals, and the four
invalidation call-site totals match the invalidation-function total. No owner,
renderer, frame or upload-route capacity overflows occur.

Cold invalidation calls are 8,552/8,528 per pass: matrix changes account for
8,433/8,421; geometry incompatibility for 119/107; material and opacity for
zero. Direct native viewport matrix changes cause zero batch invalidations.
Direct native thumb changes cause 2,494/2,472. The remaining matrix sources
are not identified by those two tags. DirtyGeometry notifications include
clip nodes and unbatched nodes; their count is not a count of uploaded meshes.

Thumb-invalidating frames are not the worst group: cold medians are
0.489/0.512 ms and p99 3.176/3.328 ms, versus 0.737/0.727 and
3.660/4.054 ms without a direct thumb invalidation. These overlapping event
groups are not causal controls; the thumb is confirmed work but not a proven
explanation for the cold tail.

Cold geometry removals total 78,793/76,083. Camera ticks account for
27,018/26,965, binding for 4,744/4,131, and unattributed removals for
46,982/44,984. The latter are not GallerySceneItem blocking callbacks and must
not be called pool retirement without further evidence. Visibility and
publication have many blocked notifications but remove almost no geometry,
because an ancestor often already blocks those branches.

Warm has at least as much work: geometry removals 82,968/79,094, alpha batch
creations 208,715/203,367, versus cold 196,402/193,661. Thus fewer logical
scene events do not explain the warm advantage. Owner and trigger histograms
are separate marginals, not joined owner/trigger pairs; batch creations are
not lifetime-joined to individual uploads.

## Comparable upload volume, different cost

The steady reverse pass uploads approximately the same smooth-color and text
data in cold and warm: smooth-color vertex bytes 119.58 MB versus 118.36 MB;
text 98.71 versus 100.79 MB. Smooth-color upload body durations total 285.14
versus 217.76 ms; text 228.22 versus 178.28 ms. Both copying and unmap scopes
cost more when cold. Those scopes include staging/copying/driver work and must
not be labelled solely GPU waits.

There are 54 cold first-pass curve uploads (45 fill, nine stroke), and none in
the cold reverse or either warm pass. Repeated SVG scene upload is therefore
not required for the steady cold slowdown. Native SVG rendering is preserved.

## Cache quiet intervals narrow the next investigation

A quiet span requires consecutive samples with pending count zero and
unchanged monotonic miss/store/frame counters. Spans must exceed 250 ms;
analysis discards the first 150 ms and includes preparation only when wholly
inside the remaining span. This observes the shared-thumbnail queue, not
independent metadata/icon work or other processes.

| Capture / pass / group | Frames | Preparation median ms | p99 ms | Frames over 3 ms |
| --- | --- | --- | --- | --- |
| Cold / 1 / quiet | 918 | 0.37075 | 1.6678 | 1 |
| Cold / 1 / not proven quiet | 3878 | 0.61210 | 3.6600 | 78 |
| Cold / 2 / quiet | 1108 | 0.38820 | 1.5892 | 1 |
| Cold / 2 / not proven quiet | 3685 | 0.65220 | 4.0673 | 88 |
| Warm / 1 / quiet | 4800 | 0.39865 | 1.5708 | 0 |
| Warm / 2 / quiet | 4800 | 0.40515 | 1.6755 | 0 |

Cold first-pass cache misses increase by 461 while stores increase by 62.
Reverse misses increase by 312 while stores increase by one. Warm miss/store
counters remain unchanged throughout both windows. This does not distinguish
failed decoding, cancellation, incompatible target requests, version changes
or successful reuse of an already-stored frame. A source-level trace is needed
before applying a failure cache, changing cancellation or altering priorities.

## Next bounded probe

Instrument a copied `src/embed/ThumbnailMemoryCache.cpp`, leaving the product
implementation unchanged initially. Aggregate by the complete immutable tuple
of source identity, version token, source file size and normalized transform;
do not key only by filename. Bound sources (for example 512) and event rows
(32,768), with explicit overflow reporting. Aggregate hits; record timestamped
owner misses, coalescing, successful publication/reuse with actual dimensions,
release retry/final flags, and cancellation. Include requested bounds and
session owner in event rows. Buffer in memory and serialize only at capture
completion; existing synchronous per-event media logging would perturb this
workload unnecessarily.

The relevant seams are acquire, storeDecoded, releaseRequest and cancelRequests.
Store publication must distinguish insertion from reuse of an existing frame:
the store counter alone cannot tell them apart. Preserve locks, callback order,
compatibility checks, source revisions and all cancellation behavior. Add tests
for immutable-key separation, bounds, concurrent recording and dropped events;
run the existing ThumbnailMemoryCache tests with the probe off/on. Then use the
same cold full-folder workload to identify the repeatedly admitted sources
and targets. Only after identifying the actual path should a product fix be
designed. Reuse the complete static link graph and existing resource audits.

## Artifacts and state

Per-case cause-attribution files have validated frame joins and counter
cross-checks. `batch-matrix-causes-20261031/upload-material-analysis.json` and
`cache-quiet-analysis.json` contain the upload and quiet-span comparisons.
Raw packet paths and hashes remain in the postsync preservation receipts;
only verified duplicates were retired. Completed older ETL files and previous
raw JSON packets were transparently compressed with before/after SHA-256
verification to retain usable disk space. Their logical content and paths are
unchanged.

Both owned live captures are stopped, while the normal worktree app remains
running. No product source, portable payload, installed dependency, commit or
push was changed in this pass.
