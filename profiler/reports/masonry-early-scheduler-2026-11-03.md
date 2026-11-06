# Masonry early scheduler trace, 2026-11-03

The first capture, o172, hit the 128 MiB trace cap and lost 66 buffers.
It is unsuitable for scheduler attribution. The retry, o173, used
context-switch and wakeup events without stackwalking. Its ETL covered
373 ms before to 5,944 ms after the planned input start and decoded with
zero lost events, lost buffers or schema errors.

The o173 capture used the same diagnostic host bytes as o171. It
reproduced two early display gaps at physical 160 Hz and DPR 1.75.

| Display gap | Present interval | GUI running | Render waiting |
|---:|---:|---:|---:|
| 62.524 ms | 62.828 ms | 62.132 ms | 60.763 ms |
| 12.490 ms | 11.443 ms | 10.218 ms | 8.691 ms |

The longer interval straddled the planned input start, ending 39.335 ms
after it. GUI runnable-but-unscheduled time was 0.014 ms. Qt's GUI thread
was executing during virtually the entire interval while the render
thread was waiting. The context-switch trace cannot name the GUI
functions. No single recorded catalog or row-work span accounts for the
full interval. This attribution applies to o173. The earlier o171
outlier has no matching scheduler trace.

The fixed-input gate confirmed y=0 and native foreground on all 2,820
delivered events. The first input arrived 243.947 ms after navigation and
180 scheduled slots were skipped. A follow-up needs GUI CPU samples and
matching symbols to attribute the running time, with a separate
uninstrumented run for cadence acceptance.
