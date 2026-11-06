# Masonry endurance: remaining presentation tail

## Finding

Longer tests contradict completion of the no-spike goal. The prepared-content
fix continues to work: every measured block has zero coverage holds. However,
one 30-second middle-scroll block contains a 12.5069 ms displayed interval.
Rapid-wheel blocks have no missed refreshes but still exceed the unchanged
two-physical-pixel maximum motion-error gate on a few timestamp comparisons.
Do not substitute the good p99 values for zero spikes.

The previous goal turn was progress: it verified the bounded look-ahead fix and
rolled it into the portable worktree app. This turn increased the input windows
from ten to thirty seconds, found a remaining failure, collected a lossless
scheduler trace and made failed analysis publication robust. No application
clock, rendering policy, alignment rule or acceptance threshold was changed.

## Production-equivalent diagnostic

The timing host was freshly built from the current production archives pinned
by `scroll-window-portable-build-20261027.json`. Its controller instrumentation
is generated reversibly from the current source; removing those additions
reconstructs that source exactly. It contains no desktop-pixel observer, marker,
experimental renderer, experimental RHI object or materializer object override.
Four import-isolation cases and the system-DLL audit pass.

The normal portable worktree app remains alive separately (launcher 43936,
Qt 119460). It was neither replaced nor stopped during these diagnostic runs.

All three captures below used the same diagnostic Qt process 78152 on the actual
160 Hz / DPR 1.75 display, Downloads, 190 entries and density 152. Each case
contains two 30-second blocks. Middle displacement is one quarter of the window
height, with endpoint reversals. The rapid workload sends wheel events every
10 ms and reverses near the directory ends. Input/focus and bounded-record gates
pass. No builds, tests, UIA polling or offline analysis ran during input.

The native controller/phase record totals after all three cases were
28,795 / 65,536 and 246,254 / 262,144 respectively. They did not overflow. The
diagnostic was closed deliberately after analysis because little phase-buffer
capacity remained; this was not an observation-timeout restart.

## Endurance results

| Input/block | Display gap p99 | Display gap max | Misses over 9.375 ms | Coverage holds | Motion error max |
| --- | ---: | ---: | ---: | ---: | ---: |
| Middle 0 | 6.2897 ms | 12.5069 ms | 1 | 0 | 17.8198 px |
| Middle 1 | 6.2963 ms | 7.4059 ms | 0 | 0 | 3.2921 px |
| Rapid wheel 0 | 6.3013 ms | 6.5411 ms | 0 | 0 | 3.0493 px |
| Rapid wheel 1 | 6.3028 ms | 6.4012 ms | 0 | 0 | 2.2668 px |
| Middle + scheduler 0 | 6.2998 ms | 6.4560 ms | 0 | 0 | 0.5868 px |
| Middle + scheduler 1 | 6.3016 ms | 6.5119 ms | 0 | 0 | 0.7471 px |

The first middle capture's strict gate fails; the rapid capture's strict
maximum-motion gate fails. The scheduler repeat passes all middle-scroll gates.
It does not erase or explain the earlier failure.

## Missed-interval timeline

In `real-scroll-endurance-middle-20261027-middle-masonry`, the qualifying missed
interval occurs about 5864.052 ms into block 0 (target CSV row index 990).
That row has:

- Producer interval: 4.5702 ms.
- Present call: 0.0360 ms.
- GPU-active time: 0.4109 ms.
- Until render complete: 5.7337 ms.
- Until displayed: 24.2464 ms.
- Between displayed changes: 12.5069 ms.

The adjacent rows have short Present calls and changing queue/display latency;
a later producer interval reaches 12.2694 ms. The app's recorded render work
around the missed interval is short. This is evidence to investigate pacing,
GPU/compositor scheduling and presentation assignment. It is not sufficient to
attribute the miss to DWM, the GPU driver or an external process. The trace that
could classify scheduled/runnable/blocked time was collected on the subsequent
passing repeat, not retroactively on this missed interval.

An idle-to-first-presentation gap also appears near input startup. The unchanged
display-window predicate requires both the previous and current displayed
timestamps to lie inside the input window, so that idle gap is not counted as
the 12.5069 ms in-window miss above. Keep startup response latency separate.

## Scheduler trace

The owned kernel trace records process/thread, context-switch and readiness
events. Existing kernel trace ownership was checked before starting, and only
the matching owned session was stopped. The exported records include the Qt UI
and render threads plus the enumerated DWM thread IDs. Metadata reports zero
events lost, buffers lost, schema errors and ProcessTrace errors. The raw trace
was 191,496,192 bytes; its verified compressed copy remains under artifacts.
No unrelated trace session, process, application or display setting was changed.

The repeat did not reproduce the miss. Its most expensive render-thread spans:

| Span | Wall | Scheduled CPU | Runnable | Blocked |
| --- | ---: | ---: | ---: | ---: |
| Block 0 preparation maximum | 2.4082 ms | 2.4082 ms | 0 | 0 |
| Block 1 preparation maximum | 3.4798 ms | 3.3893 ms | 0.0240 ms | 0.0665 ms |
| Block 1 render total maximum | 4.2538 ms | 4.1633 ms | 0.0240 ms | 0.0665 ms |

Unknown scheduling time is zero for analyzed spans. Long pacing/sync spans are
mostly blocked waiting, while the worst preparation span is predominantly
scheduled CPU work. This identifies remaining CPU headroom to investigate; it
does not establish a cause for the separate, untraced missed refresh.

## Analysis reliability change

Strict analyzers previously asserted before writing their structured summary.
An assertion could therefore leave an older successful summary in that shared
output path. The first attempt to read the new strict runs encountered that
stale-output risk. No stale values are used in this report: both cases were
rerun without strict exit solely to obtain their full current results, and
their strict failures were independently reproduced.

The middle, single-case wheel and general wheel analyzers now clear the shared
output at invocation and publish each current case before evaluating strict
gates. An integration regression runs the known failing middle and rapid
captures. Both must return nonzero, publish only the requested current case,
and contain more than 4500 display intervals per block. Both checks pass.
This changes failure reporting, not acceptance thresholds.

## Next actions and limits

1. Capture the remaining missed refresh with selected-output GPU/compositor
   timing and thread scheduling together; the passing scheduler repeat is not
   causal evidence for that miss. Preserve raw timing and the two-pixel gate.
2. Profile the scheduled preparation work in the 3.48 ms tail with CPU samples
   or focused renderer instrumentation before choosing another cache/renderer
   change. Earlier merged-span and opacity experiments remain useful context,
   not automatically approved optimizations.
3. Add cold thumbnail/preparation tests using an isolated diagnostic cache root
   rather than deleting the user's caches. A new root can exercise cold gallery
   caches; it does not make the operating-system file cache cold. Starting input
   after the existing two-second settling delay is insufficient proof of
   scrolling during preparation, so that scenario needs an explicit start
   bracket and observable in-flight work.
4. Broaden directory size/content and run ordering after those measurement gaps
   are covered. A passing Downloads run does not prove every folder or mode.

The normal app still includes the validated directional preparation fix and
current publication/staging changes. No new production optimization or rollout
was made in this turn, and no commit or push was requested. The full constant
160 Hz goal remains active and unachieved.

Evidence snapshot:
`profiler/experiments/masonry-endurance-and-scheduling-2026-10-27/` contains
sources, capture packets/CSVs, scheduler export/metadata, analyses, verification
logs and a SHA256 manifest. Executables, SDK libraries and binary ETL traces
are excluded from the snapshot; their receipts or external trace manifest pin
the originals kept in artifacts.

> AI assistance has been used to create this output.
