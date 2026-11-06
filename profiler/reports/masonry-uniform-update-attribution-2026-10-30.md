# Masonry uniform updates: storage reuse and material mix

## Finding

Repeated QRhi uniform-snapshot allocation is not the dominant mechanism in
these captures. About 99.77% of updates reuse existing storage; both observers
record zero copy-on-write detachments and zero large-buffer growth. Do not
bypass deferred update lifetime or write directly into backend shadows on the
assumption that each update allocates. No such optimization was implemented.

The next hypothesis is excessive update/binding work from separately batched
captions and small UI surfaces. Material counts identify where to investigate;
they do not measure CPU cost by class or establish a caption-layer fix.

## Read-only diagnostic and validation

The diagnostic wraps only the renderer's original queued uniform-update call.
It classifies the next QRhiResourceUpdateBatch operation slot before invoking
the unchanged `updateDynamicBuffer` call with identical arguments. No storage,
reference counts, bytes, resource-update order or GPU synchronization changes.
Counters have no per-update timestamps or logging.

Storage inspection is exact-version diagnostic code using Qt 6.11.1 private
headers. It reads the standard-layout, single-pointer QRhiBufferData object
representation with memcpy, guarded by compile-time layout/size assertions.
It does not change access control or modify the pointer. Tests verify empty
storage, pointer stability on reuse, pointer replacement on detachment, shared
contents remaining intact, and large-data capacity. This is not a portable
public Qt API or production implementation.

The second observer also counts updates and payload bytes by RTTI material
class. Its fixed 64-slot table has an explicit overflow counter, no per-call
container allocation, and atomic publication/counters. RTTI may initialize a
class name on first use; snapshots allocate their JSON output after capture.
A fixture fills the table and verifies bounded overflow and reuse. Both live
captures report zero material overflow; class totals exactly match update and
byte totals. The private header and source/executable hashes are preserved.

Tests: five storage-observer QtTest results and six material-observer results
pass, including setup/cleanup. Each host passes all 44 native renderer results,
zero skips, with actual DPR 1.75 and D3D11. Each also passes three static
import-isolation results and the Windows system-DLL audit. A helper's initial
link needed `QT_BUILDING_QT` to match Qt's private logging namespace; that was
build setup, not a changed buffer policy.

## Capture protocol and limits

Cases: `real-scroll-uniformsnapshot-a1-20261030-wheel-masonry` and
`real-scroll-uniformmaterial-a1-20261030-wheel-masonry`.
Actual Downloads, 192 catalog entries, Masonry density 152, 3840×2160,
160 Hz/DPR 1.75, new processes and empty isolated caches, two ten-second wheel
windows. Default frame-latency limit 2, bounded preprocessing and overlay fast
path, and preceding PixelGrid boundary optimization remain enabled. Sparse
sequence strategy is disabled. These are intrusive attribution captures, not
an A/B performance comparison between the two observers.

Both original capture-validity checks pass, with 3401/3404 target PresentMon
rows and 1705/1732 wheel events. Inactive input is zero and native foreground
ownership is stable throughout the measured windows. No screenshot or UI
Automation observation runs during gestures. Finite owned display holds are
released and only the owned diagnostic launcher tree is stopped afterward.

## Storage results

Counters cover the whole capture, including startup/preparation; they are not
restricted to the two measurement windows.

| Counter | Storage observer | Material observer |
| --- | ---: | ---: |
| Queued uniform updates | 923265 | 925809 |
| Reused backing | 921125 | 923669 |
| New backing | 2140 | 2140 |
| New operation slots | 2140 | 2140 |
| Copy-on-write detachments | 0 | 0 |
| Large-data growth | 0 | 0 |
| Input payload bytes | 98911292 | 99169052 |

Every update is at most 256 bytes. New snapshot backing corresponds to new
operation slots in these observations. This is CPU snapshot storage, not a
count of native/GPU constant-buffer allocations. Payload bytes are input to
the queued updates, not total memcpy traffic, driver uploads or GPU bandwidth.
The observer does not directly time allocation or copy instructions.

## Material update counts

| Material class | Updates | Share of calls | Input bytes |
| --- | ---: | ---: | ---: |
| QSGTextMaskMaterial | 255410 | 27.59% | 40865600 |
| QSGSmoothColorMaterial | 183019 | 19.77% | 13909444 |
| QSGCurveFillMaterial | 140368 | 15.16% | 13475328 |
| QSGOpaqueTextureMaterial | 134299 | 14.51% | 8595136 |
| QSGCurveStrokeMaterial | 118149 | 12.76% | 13232688 |
| QSGRhiShaderEffectMaterial | 60466 | 6.53% | 6772192 |
| QSGVertexColorMaterial | 34090 | 3.68% | 2318120 |
| QSGTextureMaterial | 8 | <0.01% | 544 |

The class count does not prove which item, caption, root, or shader dominates
CPU time. Text and rounded surfaces are concrete next candidates, not grounds
for blindly caching sampled-image state or skipping uniform writes.

## Existing gates and next work

Storage-observer blocks have 37/42 displayed intervals over 9.375 ms, maximum
motion errors 210.03/123.08 physical pixels. Material-observer blocks have
50/51 misses and motion errors 126.24/112.16 px. Coverage holds are zero.
Every block still fails the original display and maximum-motion gates.
Do not compare these one-off intrusive captures as an optimization result.

Current `MasonryEntryDelegate.qml` groups native caption text and its rounded
background beneath the entry's drawing subtree. The renderer can promote
transforms with large subtrees into batch roots. Investigate actual caption
batch/root separation and whether shared caption preparation can reduce
updates while preserving thin-image overlap order, clipping, culling,
selection, animation fallback and scene-space pixel alignment. This remains
a hypothesis to test, not a reason to move labels above all images unconditionally.

Compact sources, scripts, results, capture summaries and external hashes are
frozen in `profiler/experiments/masonry-uniform-update-attribution-2026-10-30/`.
Large packets, CSVs, native captures and executables remain at original paths.
The ordinary worktree application remains alive at PID 43936. No production
source, Qt SDK, CI policy or embedded payload changed in this investigation;
no commit or push. Full cold, verified-warm wheel and quarter-height middle
coverage remains required. Constant 160 Hz is unachieved.
