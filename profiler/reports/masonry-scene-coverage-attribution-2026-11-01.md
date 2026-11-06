# Masonry scene coverage attribution — 2026-11-01

## Finding

The cold-h91 diagnostic delivered regular display frames but still held the
scrolling camera on 40 frames. The held camera positions correlate with rows
that had no assigned scene item at the renderer's latest synchronization.
This is a preparation/scene-publication pipeline limit, not evidence that GPU
rendering itself exceeded the frame budget.

At synchronization, the 78 blocking-row occurrences were constructing (48) or
queued (30). These are occurrences, not unique rows or independent stalls:
27 unique row requests contributed to 40 held frames. Directions were forward
29, backward 5, and an unavailable anchor viewport 6. All 40 holds matched a
prior same-job snapshot, identical coverage bounds and anchor state, and a
geometric blocker. No unmatched hold was discarded.

The clocks must remain distinct. At the later held-render timestamp, 27 of
those occurrences had already been published by the GUI, 27 were still
constructing, 19 queued, 3 ready, and 2 canceled. GUI publication therefore
does not imply that the renderer has synchronized that item. Snapshot age at
held render: median 5.9784 ms, p95 12.25425 ms, maximum 12.3562 ms.

Queue waits for the 27 unique blocking requests: median 75.6263 ms,
p95 103.23085 ms, maximum 107.667 ms. Incubation durations include suspended
time and must not be described as CPU consumption.

## Scope and admission

- Case: `cold-h91`, raw directory
  `C:/Users/xs/AppData/Local/Temp/f4-current-cold-motion-20261031/cold-h91`.
- Downloads: 192 entries, Masonry density 152, actual DPR 1.75 and 160 Hz.
- Initialized application, empty gallery cache before QML, cold initial
  viewport; OS file-cache coldness was not claimed.
- 30-second wheel capture; 2,762 input samples passed native foreground checks.
  The automation observer was disconnected during timing.
- Ready-first asynchronous prototype, stable source identity, two constructed
  or unclaimed roots per layout; unchanged Go launcher.
- Qt host SHA256:
  `4c3ed131a4614d2e4161808e2ac52a1974f9bcd6ce8bf335b11231df15575fec`.
- The coverage probe records the first 3 seconds only: 243 synchronization
  frames, 4,090 candidate rows, no overflow and no wrong-writer event.
  All 40 held frames fell inside that window.

Display intervals: 4,800 samples, p99 6.291104 ms, maximum 6.4231 ms,
zero missed intervals and zero dropped presents in this capture. Maximum
recorded render span was 3.9115 ms. These results establish cadence for this
one instrumented case, not constant smooth motion or acceptance of every mode.

The existing motion analyzer excluded 4,794 wheel-curve pairs and admitted no
physical-error samples. Its display cadence result must not be used to claim
the requested physical motion accuracy. Coverage holds are independently
measured from requested versus constrained camera positions.

## Reproducible attribution

`profiler/coverage_row_attribution.py` joins a held render to the last completed
same-job coverage snapshot, checks both bounds and anchor readiness, and finds
all unprepared cells defining the limiting boundary (or overlapping an invalid
anchor viewport). Candidate horizontal intersection is performed by the
captured native layout before the probe records a row. Queue lifetimes join by
layout owner and row index. The stage is evaluated separately at synchronization
and held-render time; later readiness never rewrites the earlier stage.

Unmatched records remain in output. The analyzer rejects trace overflow,
multiple render writers, invalid row slices, reversed synchronization spans,
and overlapping active lifetimes. It refuses to extrapolate snapshots outside
the probe window. The CLI also requires accepted native foreground admission
and complete queue lifetimes.

Run:

```powershell
C:/Dev/Python/python.exe -m unittest discover -s profiler -p test_coverage_row_attribution.py -v
C:/Dev/Python/python.exe profiler/coverage_row_attribution.py C:/Users/xs/AppData/Local/Temp/f4-current-cold-motion-20261031/cold-h91 cold-h91
```

Eight regression tests pass, including forward/reverse/anchor geometry,
cancellation and replacement, future snapshots, incomplete/corrupt data,
and GUI publication before the held render but after the last synchronization.
The production-case output is `coverage-row-attribution.json` beside the raw
capture. Earlier `preliminary-attribution.json` handled forward boundaries only
and is superseded by this tested join.

## Limits and next experiment

No scheduling change was made in this analysis. Increasing concurrency without
measuring GUI work could trade camera holds for frame spikes. The next experiment
must distinguish preparation throughput from insufficient leading coverage, and
measure publication-to-next-synchronization latency. Preserve the two-root
control arm and the existing CPU budget while testing any candidate. Do not
shrink the prepared viewport or suppress the coverage guard to hide holds.

Sixteen prepared-but-unassigned candidate occurrences also appeared in the
probe. They did not define the attributed boundaries. The probe does not retain
the scanned scene-root identity for those occurrences, so a stale-root defect
is not proven by this count.

The probe is diagnostic only. This report makes no release, complete test-suite,
portable-payload, full-folder warm-tour, optical motion, or goal-completion claim.
The normal application was not replaced with this diagnostic host.

The diagnostic link's resource-only QML import test and GUI-subsystem test each
passed their selected case plus fixtures (3 results each). The system-DLL import
audit passed. The import test retained warnings about PinchArea's overridden
`enabled`, the fixture's MS Sans Serif font, and an undefined QPointF assignment;
this was not a rendered-text quality check. No packaged-payload release check
was performed. The diagnostic process tree was closed afterward, its executable
was hash-verified and archived, and the normal application remained alive.
The temporary display lease restored its prior 60 Hz mode after capture.

AI assistance has been used to create this output.
