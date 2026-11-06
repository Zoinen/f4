# Incubation and retirement attribution — 2026-10-19

The 160 Hz objective remains incomplete. Both eight-second scrolling captures
contain four displayed intervals longer than 9.375 ms, approximately one missed
160 Hz refresh. These are diagnostic captures, not clean performance comparisons.

## Acquisition and validity

`incubation-v2-{cold,warm}-sampled-wheel-masonry` exercise Downloads (186 entries),
Masonry density 152, wheel events every 30 ms, two four-second moving windows.
Both runs use PID 43472, GUI thread 12888, render thread 76440, engine `15cff8`,
window 2194×1186, DPR 1.75, actual and primary screen refresh 160 Hz. Caption
measurements and model/snapshot changes were zero. The minimal recorder disables
per-frame native recording. Native incubation, preparation and GC spans share
the QPC clock; ETW records scheduling and sampled stacks. Analysis was offline.

“Cold” means the first scrolling pass in a fresh Qt process with an already-warm
disk cache. “Warm” is the subsequent pass in the same engine, with the same reset
and settle protocol; it is not a fresh heap or a controlled GC-state comparison.
The extent changes from 4101.142857 to 4118.142857 logical pixels between passes.

Raw switch alignment matches 46,298 cold and 38,568 warm switches at 10 MHz with
900 ns offset spread. ETW reports no lost events or buffers. Final cumulative
preparation arrays contain 27,076 and 50,570 events, below the 65,536 capacity.
The initial non-v2 warm capture reached its old 32,768-event cap; it was rejected
and repeated, not used for complete warm preparation attribution. Its local trace
and log are retained.

## Measured work

Inclusive wall times across each run's eight seconds of input:

| Scope | First pass count / total / maximum (ms) | Warm count / total / maximum (ms) |
| --- | --- | --- |
| Nearby callback | 395 / 404.8674 / 7.8052 | 385 / 367.4404 / 6.9846 |
| Property commit | 597 / 323.4060 / 2.2276 | 586 / 336.0090 / 2.3360 |
| Row preparation | 22,858 / 135.9944 / 1.2840 | 22,433 / 61.1804 / 0.9108 |
| Incubation | 169 / 78.9389 / 2.1426 | 120 / 50.3250 / 2.3242 |

All observed active incubation requests specify **2 ms**. No 4 ms or larger
request was measured. The longest incubation samples contain no GC. A theoretical
budget derived from Qt source must not replace these measured values.

The longest first-pass nearby callback (7.8052 ms) includes 6.0721 ms of GC.
Another 7.0154 ms callback includes 5.2935 ms of GC. The warm maximum (6.9846 ms)
includes 5.0150 ms of GC. Reducing the row count alone would not eliminate these
pauses. Nested row, nearby, incubation and GC spans must be unioned/intersected,
never summed as independent work. Subtracting GC gives non-GC wall time, not CPU.

## Confirmed non-GC miss and corrected stack interpretation

The warm producer interval ending at QPC 132968692063200 lasts 13.0316 ms with
no GC. Nearby preparation runs from 132968683130700 to 132968686539600
(3.4089 ms), reaches its existing budget timer at 132968686501200, and prepares
**zero rows**. Thus **3.3705 ms occurs before the 2 ms timer starts**. The loop
itself takes 0.0044 ms and the tail 0.0340 ms.

The exact captured GUI stack at 132968683261100 includes
`retireInactiveDelegates+0x5c7` under `prepareNearbyDelegates`. Object-file
disassembly locates this instruction immediately after the component-property
lookup/conversion/destruction, at the next retained-set membership check. It does
not establish that the lookup alone costs 3.37 ms. The pre-timer region also
includes plan construction, retained-vector selection, release and scheduling.

Crucially, the separate `strcmp` sample at 132968686819100 is **after that nearby
callback returns**. Its full stack is `QMetaObject::indexOfProperty` →
`PixelGridContext::add` → `PixelGridContext::flush`. It must not be attributed to
the retirement component lookup. Earlier abbreviated stack summaries were too
coarse to distinguish these callers.

This producer interval includes 5.113 ms of scheduled GUI work and 5.1256 ms of
scheduled render work, with waits on both threads. These overlap and must not be
added as serial wall time. The sparse CPU samples and offline nearest-map/ICF
symbols do not identify a single exclusive 3.37 ms CPU hotspot.

## Next action

Measure retirement, retained-vector selection, release, and component lookup
separately before changing their algorithms. Native pool component identity
already exists, and the vector LRU stamp currently uses a private dynamic
property on every visibility publication; replacing internal metadata is a
candidate, not yet a demonstrated fix for this miss. Separately account for
pixel-grid re-registration when scene branches change. Preserve prefetch limits,
scene-space mapping and the existing 16-vector retention semantics.

No production source, Qt SDK, embedded payload or default GC budget changed in
this diagnostic checkpoint. The existing portable worktree app is restored,
with host SHA-256 `cf591bc8a0745c0daf0dfcbab06dd0873c48feb8526511dd52d021f65308d42c`
and launcher SHA-256 `7e19827aedbc681b08a9406048999eb64a60031b3ebbe0d5bd802ad99dcdfdbc`.

The archive under `profiler/experiments/incubation-and-retirement-attribution-2026-10-19`
preserves raw native JSON, derived attribution, selected full stacks, probe
sources, analysis tooling, identity hashes and a SHA-256 manifest. Raw ETL/CSV,
executables, libraries and build outputs remain outside that archive.

AI assistance has been used to create this output.
