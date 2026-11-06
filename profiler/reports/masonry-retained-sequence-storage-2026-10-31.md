# Masonry: heap contention and retained render-order snapshots

## Outcome

Constant 160 Hz remains unachieved. A warm scheduling capture identifies
render-order snapshot allocation blocking on the Windows heap while Qt's image
loader parses an SVG. A bounded retained-storage prototype removes repeated
snapshot allocations after a fixed working set is warmed, preserves native
rendering tests, and slightly reduces render-list p99 in this comparison.
It does not produce a reliable delivery improvement: the candidate repeat has
six long intervals while both controls have none. Every window still fails
the original maximum-motion limit. Keep the prototype experimental.

The preceding goal turn made progress by isolating competing buffer prewarming.
This turn keeps that pool off, attributes another actual wait, implements and
tests a specific allocator avoidance mechanism, and measures it in A/B/B/A
order. No production source, installed Qt archive, portable payload, normal
application, commit or push changes.

## Warm scheduling evidence

Case `real-scroll-warmwait-b1-20261031-wheel-masonry` uses the preceding pinned
host SHA256 `631b87d7976664b5f55b3400d0ef23cc48576a58b85d7aa788000b8f4c608a11`.
Actual Downloads has 192 entries including parent; Masonry density is 152,
output 3840×2160, actual DPR 1.75 and refresh 160 Hz. The normal full-folder
tour verifies every index, quiescent previews and a swap after at least 500 ms
stability. Both ten-second wheel windows have stable native foreground
ownership: 1791 inputs, zero inactive inputs. Asynchronous buffer prewarming
is disabled. PresentMon is absent; the narrow custom logger has Dwm-Core mask
`0x89` and all-keyword zero. Kernel and graphics logging remain intrusive.
There are no screenshots, UI Automation queries, builds or offline analysis
during gestures. All event/buffer-loss and decoder-error checks pass.

This recording does not reproduce the preceding warm test's 7.5752 ms
material/upload spike. Preparation maxima are 3.7861 and 2.7898 ms. All 3202
frames account for 2394.9943 ms of summed preparation wall duration:
2390.9832 ms scheduled, 2.1603 ms runnable, 1.8508 ms blocked and zero unknown.
These sums are not capture wall time. CPU-prefix verification matches all 476
xperf reference samples; all 2277 preparation CPU samples have joined stacks.

A 2.8348 ms preparation frame includes a 1.7627 ms blocked interval in
render-list construction. Exact switch/ready/resume events identify the caller
and waking thread in the same host:

```
Render thread 39096:
  RtlpLowFragHeapAllocFromContext / RtlAllocateHeap
  malloc / operator new
  QHash bucket insertion / findOrInsert<Element*, int>
  f4CompleteUnmergedPositions
  f4UnmergedPositions
  Renderer::buildRenderListsForTaggedRoots

Readier thread 24996:
  RtlLeaveCriticalSection / RtlpLowFragHeapAllocFromContext
  RtlAllocateHeap / malloc / QArrayData::allocate2
  QStringDecoder / QXmlStreamReader
  QSvgHandler / QSvgDocument / QSvgIOHandler
  QImageReader::size / QQuickPixmapReader::processJob
```

This identifies heap contention and the participating call paths. The exact
SVG URL is not captured; do not infer which file or interface icon was loading.
ICF aliases are retained in the detailed app attribution rather than claiming
one ambiguous bucket helper name. App symbols use the previously verified
code/unwind-identical map relink; Windows modules are identity checked before
symbol lookup. The new prototype's executable is not used to symbolize this
earlier host.

## Prototype

The original snapshot creates an ordered list plus a `QHash<Element*, int>` on
every rebuild. The candidate retains four independent thread-local stores.
Each has the complete ordered sequence and a flat position map with generation
tags. Pointer values are identities only and are never dereferenced by this
storage. A new lease invalidates stale generations; release clears the logical
sequence. Previous/current opaque/alpha snapshots remain independent. Duplicate
keys retain the original last-position behavior. Generation wrap is handled.

Each store supports at most 16384 elements. Oversized lists or nesting beyond
four leases use the original complete-snapshot path. The retained allocation
bound is 2.5 MiB per render thread for the four stores. No scene item, material,
order comparison, interleaved outsider, resource admission check, image quality
or source residency rule is removed. There is no raster fallback or layer
capture. The candidate is selected by a diagnostic environment flag; default
behavior remains the original implementation.

## Tests and packaging

The standalone helper compares 1048576 lookups against an independent map,
including duplicates, missing keys and many generated sequences. It checks
stale generations, generation wrap, moves, simultaneous leases, exhaustion,
oversized fallback and thread isolation. After warming the maximum working
set, 4000 snapshots perform zero heap allocations, counted through global
allocation hooks. Retained bytes are 2621440. This proves that steady-state
property, not zero allocations for every changing live scene.

Both native renderer paths pass 45 QtTest results with zero failures/skips at
actual DPR 1.75. Added test-end assertions verify zero candidate requests in
the control and actual candidate use when enabled (2052 hits, zero bypasses).
Seventeen PNGs match byte for byte: fifteen rendered captures and two fixture
inputs. Small-caption/thin-stroke output is visually inspected. Existing
tests cover full/partial ordering, alpha interleaving, culling, clipped and
retained branches, insertion/deletion, upload admission and scene-space text
alignment/transform checks at fractional scale.

The diagnostic host retains the complete static plugin/resource response file.
Three import-isolation QtTest results pass, with the existing undefined-point
QML warning retained in the log. The system-DLL dependency audit passes. No
installed SDK or archive is modified and no release/portable rollout is claimed.

## Matched live A/B/B/A

All arms use one new host SHA256
`35834bf878c26f7179a851cded7b82d10021623645ac04263a8b6fedc08ed91f`.
Only retained snapshot storage changes; buffer prewarming stays off, inline
dependency sets stay on, frame-latency limit stays 2, and sparse snapshots stay
off. Every arm starts a fresh isolated gallery cache and completes the full
normal warm tour before measurement. No kernel or additional graphics logger
is running; PresentMon supplies actual delivery intervals. Native foreground
is stable with zero inactive inputs. Input counts A1/B1/B2/A2 are
1853/1832/1847/1849. Workload and input policy match; exact traversal and scene
membership churn are not identical.

| Arm/window | Preparation max (ms) | Render lists p99 (ms) | Intervals >9.375 ms | Display max (ms) | Max motion error (physical px) |
| --- | ---: | ---: | ---: | ---: | ---: |
| A1/0 | 2.9390 | 0.3183 | 0 | 6.4381 | 2.63577 |
| A1/1 | 2.4366 | 0.3282 | 0 | 6.4133 | 3.14972 |
| B1/0 | 2.3157 | 0.2911 | 0 | 6.4006 | 2.52609 |
| B1/1 | 3.0160 | 0.2904 | 0 | 6.3891 | 2.53597 |
| B2/0 | 2.9462 | 0.3047 | 6 | 12.5077 | 116.16303 |
| B2/1 | 2.7586 | 0.2873 | 0 | 6.4182 | 2.37216 |
| A2/0 | 2.3865 | 0.2975 | 0 | 6.3990 | 2.01343 |
| A2/1 | 3.5534 | 0.3560 | 0 | 6.4422 | 2.66612 |

Coverage holds are zero throughout. All eight windows fail the unchanged
maximum-motion requirement of at most 2 physical pixels. Candidate requests
are 4896/4980, all served without fallback; growth counts are 122/120 across
the whole captures, including warm tours. Controls have zero candidate requests.
The render-list p99 differences do not establish a whole-view benefit. There
is no scheduler trace for the six long intervals, so their cause remains open.

## Limits and next work

The prototype's ordered vector currently reserves each exact new maximum.
The live growth counts expose unnecessary reallocations as that maximum grows.
Before promotion, add a gradually growing-working-set allocation regression
and use bounded geometric capacity growth. Preserve this first variant and
its unfavorable repeat rather than overwriting its executable or receipts.

The original sporadic material/upload spike remains unattributed. Wheel
display-time motion errors remain open even in windows with no long interval.
No cold-wheel or quarter-height middle-scroll acceptance run is added for this
prototype; all original cold, verified-warm and middle requirements remain in
scope. Do not treat the allocation helper, image equality, small render-list
savings, or one clean delivery run as completion of the 160 Hz goal.

Evidence is frozen in
`profiler/experiments/masonry-retained-sequence-storage-2026-10-31`.
Raw traces, packets, executables and rendered images remain external with hash
receipts. Owned capture sessions, diagnostic trees and display holds are
stopped. The normal worktree application is preserved.

AI assistance has been used to create this output.
