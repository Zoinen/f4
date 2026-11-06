# Batch upload attribution and rejected reuse prototypes

## Scope and capture validity

Continuation after parent `6bba1ec96` and gallery `63b1a8d`.
These experiments modify artifact-only Qt libraries and diagnostic hosts. The
normal Qt library, product sources, and embedded portable payload are unchanged.

The first attempted baseline (`handoff160-off-wheel-masonry`) accidentally
captured the user's active Test tab: 32 entries at density 200. It is excluded.
The diagnostic harness now activates the existing workspace numbered 4 through
the application's workspace action, records the actual session path, and
restores the previously active workspace when requested. No folder is replaced.

Valid captures below explicitly report `C:\Users\xs\Downloads`, 184 entries,
density 152, initial scroll extent 4161.142857, actual DPR 1.75, screen 160 Hz,
and an active 2194 by 1186 logical-pixel window. Wheel delta is 120 every 30 ms,
four seconds per direction. No UI Automation observer or CPU sampler runs.

Native instrumentation uses fixed thread-local arrays for renderer timing and
counters, marshalled after rendering. It adds observation overhead; these runs
attribute work and do not establish uninstrumented production performance.

## Measured baseline

`artifacts/modes-profile/handoff-target160-off-wheel-masonry.json` and its
PresentMon summary contain the valid baseline. In its slowest render-preparation
frame, preparation takes 10.9926 ms, including 409 uploadBatch calls (8.269 ms),
818 unmaps (7.014 ms), and 532 buffer creations (4.006 ms). The actual render-list
build is complete even though the earlier frame observer recorded rebuild flag
4; a dedicated hook inside buildRenderListsFromScratch confirms it. The frame
uploads about 13.87 MB of vertex/index data. Nested timings overlap and must not
be added together.

Across the capture, unmap uploads total 324,879,064 bytes and buffer creations
total 13,183. Full render-list construction occurs 22 times. Native frame
production p95 is 8.4378 ms, p99 16.9342 ms, maximum 31.3509 ms.

Display-change intervals from PresentMon:

| Direction | Median | p95 | p99 | Maximum | Intervals over 9.375 ms |
|---|---:|---:|---:|---:|---:|
| Down | 6.2503 ms | 6.2812 ms | 12.5026 ms | 25.0130 ms | 22 |
| Up | 6.2501 ms | 6.2643 ms | 6.3299 ms | 31.2587 ms | 6 |

Every recorded present is displayed, but multiple refreshes can still pass
between display changes. Constant 160 Hz is not achieved.

## Unmerged-buffer handoff: exercised, insufficient benefit

The first prototype temporarily transfers QRhi buffer ownership from a recycled
single-element unmerged batch to a matching replacement batch in the same
render pass. It excludes changed geometry, merged batches, render nodes and
visualizer mode, bounds retained memory, and releases unused buffers. It does
not replace vector geometry with textures or skip material preparation.

`handoff-target160-on-wheel-masonry` has matching folder and geometry metadata.
It records 305 reuse hits, representing 44,432,784 vertex bytes. Unmap upload
bytes fall to 272,034,060, but buffer creations remain 13,412 and the largest
render preparation is 12.3033 ms. The two directions still have 24 and 5 display
intervals above 9.375 ms. This single pair does not demonstrate a useful display
cadence improvement; do not promote it based on upload counters alone.

The native D3D11 fixture checks actual exposed rendering at DPR 1.75, unrelated
clip insertion, same-size vertex edits, material changes and node replacement.
Enabled and disabled unmerged runs passed; the enabled case demonstrated a real
reuse hit. Its coverage is intentionally narrower than the application scene.

## Merged-buffer prototypes: rejected for rendering corruption

Most upload volume belongs to merged batches. These bake transforms and depth
values into their vertex buffer and generate index/draw-set data. A prototype
checks element identity/order, geometry dimensions, matrices, index format and
draw sets, and updates only the depth segment when necessary.

`handoff-depth160-on-wheel-masonry` reports 268,135,644 reused vertex bytes and
35,030,544 unmap upload bytes, with 2,749 buffer creations. **Its rendered
screenshot is corrupt:** large pink triangles obscure the left panel. These
numbers cannot support a performance improvement claim. The small native
fixture passes despite this application failure and is insufficient coverage.

A second prototype records element metadata when the original upload completes,
rather than when recycling occurs. This accounts for the possibility that
matrix/order values already changed before recycling. It also preserves those
records across render passes and removes a dangling diagnostic reference after
cache erasure. `handoff-stamped160-on-wheel-masonry.png` still demonstrates
rendering corruption. Recording upload-time metadata is therefore insufficient;
the proposed lifetime/state explanation is not a verified root cause.

Both variants remain outside product sources. Preserve the failed captures and
prototype sources as reproducers. Do not promote merged reuse until a regression
captures the real scene's failure and establishes the missing validity or
ownership constraint. A lower upload count with wrong pixels is a failed result.

## Next investigation

The measured repeated uploads remain a real target. Alternatives include
preventing unnecessary complete render-list rebuilds, stabilizing clip/batch-root
lifetimes, or a correctly validated geometry cache. Qt's matching source forces
a full rebuild on clip-node insertion/removal; this is a source-level fact,
not yet proof of which transitions cause the measured 22 rebuilds. Attribute
those triggers before changing clipping semantics or reservation sizes.

The normal portable worktree application is restored after these experiments;
no experimental renderer library is embedded or committed. Remaining acceptance
criteria include correct rendered pixels, cold/warm scrolling, wheel/middle
input, display cadence and resting 175% leaf alignment.

AI assistance has been used to create this output.
