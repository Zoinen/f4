# GUI update delivery, 2026-10-20

The current normal scrolling path still occasionally misses display refreshes.
The new evidence locates delays between timely Windows vsync callbacks and GUI
update delivery, with overlapping QtShellController and QML-engine handlers.
No additional production change was made. A 1 ms GC-budget experiment is not
promoted: it did not remove spikes and increased the number of engine handlers.
The goal of constant 160 Hz remains incomplete.

## Scope and controls

Current source and library include the compiled 16 MiB-per-category geometry
pool. The diagnostic actual-link receipt pins the linked Quick library,
explicit renderer/controller objects, and sources. No pool environment override
was used. The normal application remains the verified embedded worktree build
with Qt host SHA256
`cc59ce7f9aa7bf6d2e40154bee453666b75424b6df079b725fd127697851756d`.

Downloads, 186 entries, Masonry density 152, DPR 1.75, 160 Hz display, wheel
input every 30 ms. Each run contains cold and warm stages, each with two
10-second input windows. Default GC budget is 5000 us; diagnostic pixmap cache
is 2 MiB. Native scrolling is explicitly selected per capture, not silently
enabled in production. No builds, tests, offline analysis, scheduler tracing,
or UI Automation observer ran during measurement input.

Long producer/display intervals mean greater than 9.375 ms. That threshold is
looser than the desired 6.25 ms period. Producer intervals require both endpoints
inside a measurement window; display timing is separately taken from PresentMon.

| Capture | Input seconds | Long producer intervals | Long display intervals | Max display gap |
| --- | ---: | ---: | ---: | ---: |
| Normal GUI, existing phase probes | 40 | 7 | 3 | 12.4990 ms |
| Native, same build/probes | 40 | 7 | 0 | 6.3711 ms |
| Normal GUI, Qt loop scopes | 40 | 7 | 3 | 12.5005 ms |
| Normal GUI, loop + vsync observer | 40 | 7 | 3 | 12.5060 ms |
| Normal GUI, loop + vsync + dispatch | 40 | 5 | 3 | 12.5109 ms |
| Same dispatch build, GC budget 1 ms | 40 | 5 | 1 | 12.4968 ms |

These are bounded runs, not proof that native scrolling always prevents drops.
The smaller GC run is not a demonstrated cure: producer outliers remain, and
the display counts are too small for a single paired run to establish a stable
reduction. No native-default rollout or GC-budget change was made.

## Added diagnostics

An explicit diagnostic replacement of qsgthreadedrenderloop.cpp records GUI
polish, swap-chain proxy preparation, mutex acquisition, sync posting/waiting,
animation advancement, and incubation. The source clone retains the original
control flow. A stale generated moc include path in the local Qt build cache
was corrected only in the diagnostic copy. No installed Qt source was modified.

The vsync observer uses preallocated records and target-window filtering. An
event filter records UpdateRequest delivery. Callback timestamps are observations
after Qt's platform callback has posted its GUI event; the GUI can handle that
event slightly before the observer runs. Matching permits a 0.1 ms ordering
tolerance and never treats a tiny negative offset as a negative queue delay.
Qt's service timestamp is relative to its own clock start; only differences are
used, not an invented alignment to absolute QPC.

The dispatch probe records outermost GUI events lasting at least 0.2 ms using
fixed storage and receiver-class copies. It performs no GetThreadTimes calls,
per-frame logging, objectName queries, or UI Automation. Handler measurements
are wall time, not proven CPU execution. All buffers remained below capacity.
The dispatch threshold means recorded duration sums omit shorter events.

## Findings

Vsync observations stayed close to 6.25 ms. In the vsync-only capture, maximum
callback periods were 6.4717 ms cold and 6.6806 ms warm; none exceeded 9.375 ms.
Some long producer frames received GUI UpdateRequest 3.2011, 5.4505, and
4.8512 ms after the corresponding observer callback. UpdateRequest-to-polish
entry was then only approximately 0.003–0.033 ms in those cases. The delay lies
before ordinary GUI render-loop entry, not in swap-chain proxy creation or its
mutex acquisition. This does not prove all missed display frames share one cause.

The dispatch run identifies actual overlapping handlers:

- Cold callback-to-update lag 2.0011 ms: QtShellController MetaCall took
  3.2531 ms, overlapping 1.7874 ms of the lag interval.
- Another cold lag 0.8777 ms: QtShellController MetaCall took 1.0639 ms,
  overlapping 0.8445 ms.
- Warm lag 5.8109 ms: QQmlApplicationEngine MetaCall took 6.1450 ms,
  overlapping 4.4233 ms; a Windows GUI dispatcher timer then occupied
  another 1.3565 ms before update delivery.
- Other cases include the host window's own frame MetaCall and dispatcher
  timers. One late producer interval followed a 12.2142 ms between-cycle gap
  despite only 0.1954 ms callback-to-update delay; a late producer interval
  is not synonymous with one specific blocked handler.

The loop-only capture observed 5.8171 ms in incubation and 7.0705 ms in animation
advancement. Neither maximum aligns with that run's long producer frames;
they are useful leads, not causal proof for those frame drops. Typical proxy
and mutex scopes are sub-microsecond, so optimizing them is not supported.
Between-cycle summary statistics exclude measurement-boundary idle gaps.

## GC experiment and next work

Local Qt source queues incremental MemoryManager::onEventLoop work on the QML
engine. That makes GC a plausible source of the long engine MetaCalls; receiver
class and wall duration alone do not prove the exact queued function or GC state.
The 1 ms experiment uses MemoryManager::setGCTimeLimit through the existing
diagnostic hook. Captured actual budgets verify 1000 us, with no
QV4_GC_TIMELIMIT environment override.

| Engine MetaCalls >=0.2 ms in input windows | Default 5 ms | Diagnostic 1 ms |
| --- | ---: | ---: |
| Cold count | 8 | 25 |
| Cold maximum | 5.3288 ms | 6.9472 ms |
| Warm count | 5 | 17 |
| Warm maximum | 6.1450 ms | 5.5166 ms |

Smaller incremental budgets do not bound indivisible work. These measurements
also do not establish memory neutrality; no memory-growth claim is made.
Next identify the exact QML-engine queued call/GC phase and QtShellController
scene-application subphases, correlate each with display deadlines, and remove
or split the demonstrated expensive units. Preserve complete scene ordering,
input responsiveness, bounded source preparation, and memory reclamation.

The analysis now accepts slow captures instead of requiring an achieved frame
count. It supports both GUI and native controllers and validates thread identity,
probe capacity, interval coverage, and explicit GC configuration. This improves
measurement fidelity; it does not weaken the requested completion criterion.

All timing inputs, generated probe sources, link identities, and findings are
hash-archived with a manifest. Only exact diagnostic launcher trees were stopped.
The normal worktree app remains responsive. No commit or push was made.

AI assistance has been used to create this output.
