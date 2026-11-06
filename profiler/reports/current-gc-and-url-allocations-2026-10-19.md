# Current GC stalls and URL boxing — 2026-10-19

The constant-160-Hz objective remains incomplete. This investigation uses the
current native-bookkeeping implementation and changes diagnostic tools only.

## Current-source attribution

`current-preparation-{cold,warm}-sampled-wheel-masonry` uses one process
(43112), engine `15cff8`, GUI thread 48384, render thread 66896, window
2194×1186, actual/primary refresh 160 Hz, DPR 1.75, Downloads 186 entries,
density 152, original 5 ms GC budget. Two four-second moving blocks use
120-unit wheel events every 30 ms and boundary bounce. First pass means
fresh Qt process with warm disk cache; warm pass reuses its engine/heap.
Extents are 4101.142857 and 4118.142857. Caption measurements and model/
snapshot signals are zero. No native frame recorder or UIA observer runs.

The probe is generated from current MasonryLayoutDelegateCommit.cpp, not the
old copied GUI preparation TU. Its source calls the pool key accessor and
native vector stamp. Additional spans isolate hiding a row, unassigning its
index and trimming spares. The full Qt translation units are replaced by
exact archive member, without duplicate-symbol forcing or SDK mutation.

- cold: 71,834 matched switches, 900 ns QPC offset spread; 106,504 native component checks total 7.9776 ms; 11 displayed intervals exceed 9.375 ms.
- warm: 62,570 matched switches, 900 ns QPC offset spread; 105,093 native component checks total 7.8725 ms; 5 displayed intervals exceed 9.375 ms.

ETW lost-event/buffer checks pass. Preparation arrays contain 39,161/71,572
records below their 131,072 capacity; GC arrays 2,539/2,899 and incubation
arrays 229/364 remain below 32,768. Counts and span timings add observer
overhead. These are diagnostic runs, not comparisons against clean captures.

## The remaining atomic sweep

DoSweep lasts 4.4134 and 7.7828 ms in the first pass and 8.0032 ms in the
warm pass. Its regular block-allocator portion reaches 7.3999/7.6143 ms.
These are wall durations, not exclusive CPU measurements. Selected exact-QPC
GUI stacks show Chunk::sweep, VariantObject destruction, QUrl destruction,
StringOrSymbol destruction, QObject connection cleanup and QQml notifier
disconnects. Unknown kernel/DLL symbols remain unresolved; ICF aliases are
preserved. Sparse samples do not assign an exact percentage to each type.

Qt checks its GC deadline between state executions. DoSweep calls the full
block allocator sweep synchronously, so reducing the 5 ms setting cannot
interrupt that operation or guarantee a 6.25 ms frame bound. Mark-drain
slices and unrelated GUI/render work also remain. GC does not explain every
displayed miss.

## Sweep pass breakdown

A second diagnostic host adds three spans inside BlockAllocator::sweep:
object sweep/partition, rebuilding bins, and freeing empty chunks. It uses
the same current core and original GC algorithm. Two PresentMon/native
captures run without WPR and without offline work, tests, builds or UIA
overlapping acquisition. They share process 44648, GUI 63408, render 56976
and engine `15cff8`, with the same display/workload protocol. These durations
must not be treated as a before/after improvement over the WPR captures.

| Pass | DoSweep ms | Regular object sweep ms | Rebin ms | Decommit ms | Empty chunks |
| --- | ---: | ---: | ---: | ---: | ---: |
| cold | 3.3557 | 2.7434 | 0.3189 | 0.0002 | 0 |
| cold | 5.8084 | 5.3145 | 0.2166 | 0.0001 | 0 |
| warm | 5.7500 | 5.2563 | 0.2058 | 0.0002 | 0 |

No empty regular chunks were released. The dominant pass destroys managed
objects, rather than decommitting pages. An empty-page cache is therefore
not justified by these captures. An initial diagnostic compile hit a removed
Conan source path in generated moc output; copying that moc locally and
retargeting its include to the identical installed 6.11.1 private header
fixed the build. The SDK was not edited.

## Allocation census blind spot

The previous value-wrapper census instruments QQmlValueTypeWrapper. It does
not count QV4 VariantObject allocations, which include QUrl values. The new
probe adds creation-site counters in VariantObject::init(QMetaType,data),
sharing the existing inline census storage while retaining one export TU.
It does not count default init() or claim complete JS-heap coverage.

`current-variant-census-wheel-masonry` counts the two scroll blocks plus
intervening work through final serialization. Native string/hash aggregation
adds overhead; this capture is solely allocation evidence, not timing data.
No counter overflow occurred. The original counter sees 9360 value-wrapper creations; the extension adds 18631 variant-wrapper creations in the same capture. These categories must not
be compared as if older census totals covered both routes.

| Additional variant type | Creations |
| --- | ---: |
| QUrl | 14,091 |
| QRectF | 2,128 |
| QColor | 1,658 |
| QSizeF | 657 |
| QPointF | 97 |

Located QUrl sites include GalleryEntryPreview.qml thumbnail source checks,
fallback-icon component selection and sourceSize predicates, and thumbnail
factory/source bindings. 3,774 QUrl creations have no current QML frame; their
callers remain unassigned rather than guessed. Type totals alone do not
measure destruction cost.

## Next implementation

Reuse thumbnailHasSource instead of repeatedly reading/boxing its URL.
Cache textual URL classification for fallback icons and reuse primitive
flags/strings across bindings. Keep the actual typed renderer source, URL
resolution, failure fallback, SVG pinning and source replacement semantics.
Add regressions for reactivity through source/identity changes, then compare
the extended allocation census and clean display cadence. Do not disable
Qt reference-object notification semantics or add unchecked GC yields.

Six interval-union regression tests pass. The production host, launcher,
SDK, embedded payload and GC default remain unchanged in this investigation.
The normal worktree portable app is restored before analysis, with cached
child/host hash verification and empty stderr recorded by the archive.
Raw native JSON, selected GC stacks, derived findings, source/probes and
SHA-256 manifest are preserved in
`profiler/experiments/current-gc-and-url-allocations-2026-10-19/`.
Executables, libraries, raw traces and CSV exports are excluded.

AI assistance has been used to create this output.
