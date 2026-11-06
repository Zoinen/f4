# Masonry: event-filter and keyword controls

## Outcome

Constant 160 Hz remains unachieved. A matched broad/filtered/filtered/broad
comparison removes most graphics trace events without removing the long
compositor cycles or missed deliveries. A further probe removes unnecessary
keyword groups from the added logger and also fails. Neither experiment is a
production optimization or a passing acceptance run.

A read-only provider inventory exposes a remaining shared observer: PresentMon
enables Dwm-Core with MatchAnyKeyword `0x8000000000000081`, MatchAllKeyword zero.
The high diagnostic bit occurs on all sixteen required DWM event schemas.
Thus reducing only our additional logger's keywords does not establish that
the provider has no broader enable state during measurement. This proves
configuration, not that PresentMon causes the CPU cost. A control without that
shared collector is now the next discriminating experiment.

The preceding goal turn made progress by joining application presents, surface
tokens, compositor cycles and scheduling on the same failing capture. This turn
tests an observer-effect explanation and identifies another concrete confounder.
No application source, Qt package, window geometry, quality, animation clock,
portable payload, commit or push changed.

## Controlled capture

All cases use the same pinned host (`631b87d7976664b5f55b3400d0ef23cc48576a58b85d7aa788000b8f4c608a11`),
the same experimental renderer options including compact dependency sets, and
frame-latency limit 2. Only graphics tracing changes. Every case starts a fresh
process and isolated empty gallery cache, uses actual Downloads with 192 entries,
Masonry density 152, 3840×2160 output, actual DPR 1.75/160 Hz, and two ten-second
wheel windows. The warm tour is skipped for this cold/loading workload.

Native foreground checks pass throughout. Input counts are 1708, 1718, 1692 and
1710 for A1/B1/B2/A2; the additional lean-keyword case delivers 1713 events.
There are zero inactive inputs, buffer overflows, resource errors, or coverage
holds. Durations and input policy match; delivered counts, loading/membership
churn and paths are not identical. There are no screenshots, UIA polling, builds
or offline analysis during gestures. Native recording, kernel profiling and
PresentMon remain the same in all cases.

The trace controller uses inclusive EVENT_FILTER_TYPE_EVENT_ID filters. It
creates a new session, retains the returned handle and stops only that handle.
A named event and 180-second bound prevent an abandoned logger from persisting.
Tests verify filter descriptors/IDs/counts/uniqueness, actual emitted event IDs,
normal stop, and refusal to replace or stop an existing session. Every final
capture reports zero event/buffer loss, metadata/property errors and scheduler
schema errors. The controller executable is SHA-pinned in its test receipt.

Broad and filtered modes initially use identical keyword masks/levels:
DxgKrnl `0x1`/5, Dwm-Core `0x10008b`/5, DXGI `0x2`/5, Win32k `0x1000`/4.
Filtered DWM IDs are 6,7,8,9,10,12,13,14,15,16,63,64,65,194,195,196;
DXGI 42/43; Win32k 201/301; DxgKrnl 17/110/181/273.
The actual filtered traces contain no other IDs from those providers.
ForceVsync is not enabled.

## Results

| Case | Graphics events, whole trace | Misses in windows 0 / 1 | DWM scheduled CPU median in missed cycles, ms | DWM cycle maximum, ms |
| --- | ---: | ---: | ---: | ---: |
| A1 broad | 1,418,434 | 22 / 52 | 5.8022 | 11.2296 |
| B1 filtered | 152,571 | 31 / 57 | 5.8843 | 10.9763 |
| B2 filtered | 163,915 | 27 / 54 | 5.9231 | 10.0354 |
| A2 broad | 1,640,011 | 23 / 44 | 5.6266 | 14.4091 |
| Additional lean keywords | 203,929 | 51 / 55 | 5.9013 | 10.3629 |

Whole trace counts include startup/activation and have different total recording
durations; they are not normalized per-window event rates. The exact filter-ID
guard, rather than a raw count ratio alone, proves event suppression.

Broad misses total 141 and filtered misses 169. This does not support event
filtering as a scrolling fix. Most compositor time in these associated cycles
is scheduled CPU; unknown scheduling time is zero. Reducing emitted events
does not reproduce a reduction in that cost. Kernel profiling, required graphics
events, shared collectors, and keyword-driven work remain possible observer costs.

All 310 ABBA misses have matched driver refresh evidence: 309 advance two refresh
numbers and one advances three. B1 also has two ordinary display intervals with
an unresolved cross-session timestamp association. The original 100-microsecond
association guard is preserved: those remain explicit unknowns and are not
discarded or counted as passing evidence. Every failed display interval remains
in the unchanged performance result. No acceptance threshold is relaxed.

Cold preparation maxima in the eight windows range 5.0962–9.0732 ms; several
exceed the 6.25-ms budget. Display maxima range 12.5646–18.7051 ms, and maximum
motion errors exceed 100 physical pixels. Every original full performance gate
fails. The additional lean probe likewise fails, with 106 matched two-refresh
misses and preparation maxima 7.1465/7.5372 ms.

## Keyword-specific probe

An offline metadata-only replay extracts runtime event keywords from the same
saved trace. DWM frame/surface/present events require Scheduling `0x80`, history
events Composition `0x1`, and wait events DwmFrameRate `0x8`. All sixteen selected
events therefore remain enabled by `0x89`, without DetailedFrameInformation `0x2`
or WorkPerFrame `0x100000` in our added logger.

A separately built controller records its actual `0x89` mask and exact ID list.
Its lifecycle, existing-session-preservation and emitted-ID tests pass. The
single full lean-keyword capture above is an attribution probe, not a replicated
keyword comparison or proof that keyword work contributes zero overhead.

All these captures still run PresentMon 2.6.0 with `--no_track_gpu`. A separate
three-second no-scrolling probe inventories Dwm-Core's runtime enable entries
before, during and after that collector. Before and after, entries are empty;
during, one logger enables MatchAny `0x8000000000000081`, MatchAll zero, reported
for three provider instances. These are three instances of one logger, not three
independent collectors. State restores exactly after the owned collector exits.
This probe has no scrolling or performance claim.

The consumer's existing ID filters may avoid work at some instrumentation sites;
the broad keyword enable state alone does not prove expensive work executes.
Nevertheless it prevents attributing all measured compositor CPU to normal
unobserved operation without another control.

## Reliability and retained failures

The first B2 attempt stops at the existing 250-MiB disk guard before launching an
application or queuing input. Its short traces/controller results are retained
and excluded from performance aggregates. Completed analysis folders are copied
and hash-verified on C:, then their D: paths become junctions. Recursive retirement
checks absolute roots, nested reparse points, file hashes and running executables.
The actual B2 capture uses fresh `filtercpur2` case/cache/observer names. A missing
parent directory in its preparation is corrected before starting any logger.

The first filtered chain analysis stops on its unchanged timestamp-association
guard. The analyzer now publishes unknown associations explicitly while keeping
all display failures and the original guard. This is reporting robustness, not
a new motion-time basis. Archived raw inputs are hash-checked before replay so
that rerunning analysis does not reuse an unrelated shared summary.

No new static UI layout exists in this change. Scene fidelity and the existing
175% requirements remain intact. Source inspection also finds that the host
already disables its default alpha buffer and system transparency, and the Qt
backend already uses flip-discard. No speculative duplicate window-opacity fix
was added; actual runtime swapchain alpha flags were not measured here.

## Next action and state

Run the same workload with the minimal custom presentation/scheduler logger and
without PresentMon to isolate the shared collector. Treat this as diagnostic
attribution only until presentation identity/cadence can be independently
validated. Alternatively, a collector with narrower provider keywords must
demonstrate identical required event coverage and decoding before being used
for performance claims. Keep the original cold/warm/middle, two-pixel motion,
0.002-pixel middle native-velocity and zero-coverage-hold gates.

The independent app-side cold preparation tail still needs optimization. Do not
substitute a collector change, a warmed-only run, fullscreen, reduced content or
disabled composition for the requested full result.

Owned diagnostic process trees and trace sessions are stopped. Both display
holds are released and terminal; their bounded lifetimes and inner capture
display checks are retained. The normal worktree app remains alive unchanged.
Evidence is frozen under `profiler/experiments/masonry-trace-filter-controls-2026-10-31/`;
large traces/exports and binaries remain at externally hashed local paths.

> AI assistance has been used to create this output.
