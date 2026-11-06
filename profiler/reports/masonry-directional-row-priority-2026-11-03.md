# Masonry directional row priority, 2026-11-03

The diagnostic candidate preserves visible-row priority and the existing
delegate and metadata tiers. Within each overscan tier it admits rows
ahead of the wheel target first, nearest edge first in either direction.
It changes both queue-plan entry points and keeps the two-root limit.

| Metric | o176 control | o177 directional |
|---|---:|---:|
| Delivered wheel inputs | 2,805 | 2,838 |
| Display interval p99 | 6.283 ms | 6.288 ms |
| Maximum display interval | 68.744 ms | 62.522 ms |
| Intervals above 9.375 ms | 3 | 2 |
| Coverage holds | 217 | 244 |
| Queued blocker occurrences | 1,354 | 1,654 |

Both captures passed the first-viewport and native foreground checks,
at physical 160 Hz and DPR 1.75 on the 194-row workload. Both skipped
scheduled input slots. The experiment did not show a coverage benefit
and remains diagnostic. The two runs do not establish a reliable
cadence difference.

## Admission comparison

For eventually admitted blockers, the median elapsed time from the
first queued readiness snapshot to construction start rose from
29.75 ms (77 requests) to 53.83 ms (76 requests). Median spare-slot
time rose from 5.69 to 7.98 ms, and the median number of other roots
admitted first rose from three to four. The largest requested-to-visible
error rose from 475.01 to 510.78 physical pixels. All holds occurred
within the first 2.43 seconds in both arms.

These results reject promoting this ordering heuristic. They do not
isolate a specific bad priority decision because the live traces do
not include the actual rank of each waiting request. Earlier anonymous
spare-pool experiments and a larger root limit also lacked consistent
cadence gains. The remaining evidence supports measuring and reducing
the per-row construction and initialization cost with the current
two-root control before trying another priority heuristic.
