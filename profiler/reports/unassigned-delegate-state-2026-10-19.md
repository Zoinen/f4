# Unassigned delegate hover and cursor churn

Date: 2026-10-19. Goal: constant 160 Hz without spikes. The goal remains active.

## Cause and implementation

GalleryEntryDelegateBase compared viewIndex directly with hoveredIndex and visualCursorIndex. An unassigned delegate and a panel with no hovered/current item both use -1. Equality therefore made an unassigned delegate count as hovered/current. During materialization and recycling this unnecessarily ran the color-composition path before the final row was assigned.

Both predicates now require viewIndex >= 0 before comparing indices. This preserves normal hover, selection, palette changes, and cursor state for valid rows. No geometry, pixel fitting, GC setting, dependency, or wire protocol was changed. The initial hypothesis about lost type information was not needed to explain this portion of color churn; broad type or palette rewrites were not applied.

## Regression evidence

unassignedDelegateHasNoHoverOrCursor covers all five modes. It clears both panel indices, unassigns a real delegate, verifies neither state matches, then assigns a valid row and verifies both states remain reactive. It also exercises subsequent recycling. A controlled build with the old predicates fails at the false-hover assertion in all five modes. The guarded build passes.

An initial test invocation lacked QT_QUICK_CONTROLS_STYLE=Basic and failed at panel creation because QtQuick.Controls.Windows is not shipped in this static build. That invocation is not the bug reproduction. The valid before/after tests explicitly select Basic and use actual DPR 1.75.

Direct3D and OpenGL each pass 16 selected mode cases plus 3 import cases. The selected cases include live theme updates, semantic icon colors, hover tracking over recycled delegates, cursor reveal/animation, resting physical alignment, and Columns text alignment. The static Windows dependency audit passes.

## Allocation census

Separate instrumented captures use continuous wheel input, Downloads 186 entries, density 152, 160 Hz, DPR 1.75, 5 ms GC budget, and no frame recorder. Aggregate counts add native overhead and are not frame-time measurements. Both blocks and their inter-block gap are included. No census cap overflow.

| Metric | Before | After |
| --- | ---: | ---: |
| Created QColor wrappers | 6,419 | 3,681 |
| All created value wrappers | 12,520 | 9,791 |

Color wrappers decrease 42.7%; all wrappers decrease 21.8%. The remaining color sites and destruction connection counts are retained in the comparison JSON. The source fix and regression establish the sentinel defect; the census establishes its allocation impact for this workload, not a proportional frame-time benefit.

## Clean display comparison

A1/B1/B2/A2/A3/B3 fresh-process order. A is the preceding scalar-preview implementation; B adds only the sentinel guards. Both use minimal recorder, no census, original 5 ms GC budget, and 120-unit wheel events every 30 ms in two four-second boundary-bounce blocks. Workload, DPI, screen, and active-window state are validated. No build, test, WPR, QML profiler, or UI Automation observer overlaps the captures. Both display endpoints must be inside the input window; internal stalls are retained. All six runs are included.

| Run | Block 0 p99 / max (ms) | >9.375 ms | Block 1 p99 / max (ms) | >9.375 ms |
| --- | ---: | ---: | ---: | ---: |
| A1 | 6.3365 / 12.4997 | 3 | 6.2837 / 12.5048 | 2 |
| B1 | 6.2996 / 12.4991 | 2 | 6.2753 / 12.4978 | 1 |
| B2 | 6.3045 / 12.4986 | 2 | 6.3295 / 12.4987 | 2 |
| A2 | 6.3250 / 12.5035 | 2 | 6.2882 / 12.4941 | 1 |
| A3 | 6.3087 / 12.5055 | 3 | 6.3141 / 12.5002 | 2 |
| B3 | 6.3130 / 12.4961 | 2 | 6.3061 / 12.6187 | 1 |

Long intervals: A 13, B 10. Short interleaved runs are evidence about this workload; they do not establish a deterministic frame-time bound. Missed refreshes mean the overall objective is still incomplete.

## Rollout and next work

The normal host is rebuilt, the embedded payload regenerated, and the CGO-disabled Go launcher rebuilt with the system cache. Payload/extraction tests pass. The archive verifies the restarted worktree Qt child hash against the new host, direct --gui=qt launch, and empty startup stderr.

Next: attribute remaining misses on the reduced-allocation implementation. Separate atomic GC sweeps from GUI incubation and render preparation; do not infer that the remaining QColor count explains every miss. QSizeF thumbnail-size bindings and repeated QPoint wheel-delta reads remain measurable allocation sites.

Compact evidence and reproduction tools are archived with SHA256 hashes under `profiler/experiments/unassigned-delegate-state-2026-10-19/`. Raw traces, executables, libraries, and build outputs remain local and are excluded from the archive.

> AI assistance has been used to create this output.
