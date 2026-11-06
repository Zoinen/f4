# Masonry early GUI CPU sampling, 2026-11-03

The o174 profile-only trace ran at physical 160 Hz and DPR 1.75 on the
194-row Downloads workload. The first-viewport and sampled native
foreground checks passed. The run skipped 204 of 3,000 input slots.

The ETL decoder reported zero lost events and buffers. Because the
sampled-profile decoder used its prefix fallback, xperf independently
verified all 499 reference timestamps, thread IDs and instruction
pointers in the early window. Function labels come from the map for
the exact profiled executable. Identical-code-folding aliases remain
ambiguous.

| Present time relative to input | Display gap | GUI samples | Work found on sampled stacks |
|---:|---:|---:|---|
| 39.71 ms | 43.75 ms | 29 | 15 QML binding updates, 6 decoded-frame applications |
| 134.26 ms | 37.50 ms | 41 | 19 QML property writes, 10 panel catalog frame applications |
| 171.56 ms | 37.50 ms | 32 | 9 panel catalog frame applications, 4 delegate-row preparations |

Counts overlap because a single stack may include several listed frames.
They are sample counts and do not give exact durations. Earlier gaps
also showed incubation and thumbnail/metadata completion callbacks.
The largest early gaps contain both QML property propagation and
publication work on the GUI thread. Instrument individual QML bindings
and their triggering frame operations before selecting a fix. This
run does not attribute the separate o173 spike to a specific function
or establish uninstrumented frame cadence.
