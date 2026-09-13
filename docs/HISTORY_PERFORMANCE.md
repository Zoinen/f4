# History dialog performance

Command, viewer/editor, and folder histories use the same `historySearch`
implementation and native menu presentation. Imported histories can contain
thousands of records; navigation must not serialize those records again.

## Retained rows

History menus increment `VMenu.SemanticItemsRevision` whenever filtering,
deletion, pinning, or column options rebuild their rows. Zero disables retention
for other menus whose owners may mutate items directly. A nonzero revision is
an immutable-content contract, not a selection revision.

The native projection reuses rows from the last accepted scene with the same
menu identity and items revision. A navigation `scene_patch` carries the menu
header, `itemsRevision`, and `itemsUnchanged: true`, omitting `items`. The sender
keeps complete rows in its accepted snapshots. The Qt reducer restores the
shared row list only when identity and revision match; missing or stale bases
are rejected. Changed rows and newly opened menus carry complete items.

Structured history rows serialize their column details without duplicate
console labels or unused default menu fields. Qt retains its list model during
navigation, reuses visible delegates, renders unfiltered columns as plain text,
and creates general-menu/drive visuals only for rows that use them.

## Reproducing measurements

`BenchmarkLargeHistory` in `internal/app/history_benchmark_test.go` exercises the
three real dialog actions, Page Up dispatch, native projection, and JSON
encoding. It uses 2,432 commands, 999 files, and 1,294 folders, with long paths.
Run from the repository with the system Go cache:

```text
go test ./internal/app -run ^$ -bench ^BenchmarkLargeHistory$ -benchtime=30x -benchmem
```

Add `-cpuprofile=<output.pprof>` and inspect with `go tool pprof`. The benchmark
removes closed frames from the frame manager just as the event loop does.

The opt-in Qt test `largeHistoryLatencyProfile` uses the same record counts,
five opens, and forty 30-row jumps per dialog. Set `F4_HISTORY_PROFILE=1`,
`QT_SCALE_FACTOR=1.75`, `QT_QUICK_BACKEND=software`, and
`QT_QPA_PLATFORM=offscreen`, then run `F4QuickViewSurfaceTests` with that test
name and `-o <report>,txt`. It measures decoding, typed-store application, and
the next frame swap. Navigation shares accepted rows as the protocol reducer
does; controller tests separately exercise revision validation.

## Windows measurements, September 2026

Ryzen 9 5950X, Go 1.26, Qt 6.11.1. Times below cover Go preparation/projection/
encoding, not display latency. Synthetic text is used, not private history.

| History | Open before | Open after | Page before | Page after |
| --- | ---: | ---: | ---: | ---: |
| Commands | 93.5 ms | 7.6 ms | 10.5 ms | 0.023 ms |
| Viewer/editor | 19.5 ms | 4.9 ms | 3.6 ms | 0.021 ms |
| Folders | 24.1 ms | 4.7 ms | 5.1 ms | 0.021 ms |

Initial synthetic JSON payloads fell from 1.79/0.57/0.74 MB to
0.52/0.16/0.21 MB. Navigation is approximately 1.1 KB including the benchmark's
root chrome, regardless of history length. Actual transport menu patches were
411–421 bytes during a live check with imported histories.

In the separate Qt software-rendering benchmark, median open-to-frame times
fell from 179/130/139 ms to approximately 87/55/62 ms. After correcting the
fixture to forward selection-only signals, page-to-frame medians were
10.5/9.9/8.0 ms, including frame scheduling; corresponding 95th percentiles
were 14.8/13.6/11.8 ms. Earlier synthetic page-to-frame numbers did not exercise
QML selection changes and are superseded by these measurements. The live
transport measurements below are independent of that fixture.

Live Qt traces of repeated Page Up inputs measured median input-to-patch-apply
times of 5.16/5.02/5.11 ms and Qt application times of 0.41/0.38/0.40 ms for
commands/files/folders. These measurements precede the final initial-payload
reduction, which does not change navigation. Paint scheduling adds display
latency; these are not key-to-photon claims.

Regression coverage includes retained snapshot integrity, bounded wire size,
filter invalidation, stale/missing Qt revisions, Unicode truncation, search
highlighting, stationary-pointer selection, and actual text-leaf transforms and
rendered images at 175% scale. The wider controller suite has an independent
handshake test expecting three capabilities while the existing host advertises
four; the history/menu controller cases pass.

## Search titles and continuous scrolling

Filtering updates the menu's native title with the query and prefix marker,
including when there are no matching rows. The console painter continues to
highlight the same query in its custom title.

At DPR 1.75, repeated-arrow testing reproduced a text origin at 69.499997
physical pixels and a viewport-edge jump from 70 to 68 physical pixels. History
delegates now snap their shared row immediately when scrolling/recycling changes
its position. The list disables logical-pixel alignment, settles recycled-row
layout, and preserves the exact row-relative scroll offset at either edge.
Snapping the local offset alone is insufficient: a fractional row origin must
cancel against the same content offset before scene coordinates are rounded.

`historyHeldUpKeepsRowsOnPhysicalPixels` checks 81 Up moves followed by 80 Down
moves, all three text columns' scene origins and unit transforms, stable
viewport-edge positions, query title alignment, and a rendered capture.
