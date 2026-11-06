# Requested-row concurrency — 2026-11-01

Increasing the diagnostic Masonry construction limit from two to four does not
provide a reliable scrolling improvement. Do not adopt it as the fix for
constant 160 Hz. The four-row runs retain 67/68 held camera frames, incur a
refresh miss each, and have longer maximum render spans than either control.

## Experiment

`RequestedRowRootLimit.h` reads a diagnostic-only limit when the layout creates
its row-incubation state. Default remains two. The experiment compares two and
four in the same binary; the helper bounds diagnostic input to 2–8, but eight
was not tested. No per-frame environment lookup was added. The normal binary
was not changed.

Both arms enable stable source identity, ready-first handoff and earlier
directional preparation. Only `F4_REQUESTED_ROW_ROOT_LIMIT` differs. Qt's
incubation controller, libraries and budget remain unchanged; no synchronous
completion was added. The larger limit can nevertheless increase work when
starting, assigning or presenting a batch, so unchanged controller configuration
does not establish unchanged total GUI/render cost.

Host SHA256: `6f6cc0ebb254866e4bfcbc6b4599aee65307b191fb58ee64f1544db76834b1d1`.

## A/B/B/A results

Initialized-app cold folder entry into Downloads, 192 entries, Masonry density
152, 3840×2160 at 160 Hz and actual DPR 1.75. All input samples confirm these
runtime properties and native foreground. The observer helper was absent during
timing. Each case has 30 seconds of rapid wheel input.

| Case | Root limit | Held frames | Display samples | Display p99 ms | Display max ms | Intervals >9.375 ms | Maximum render span ms |
|---|---:|---:|---:|---:|---:|---:|---:|
| cold-j111 | 2 | 106 | 4799 | 6.301218 | 12.5049 | 2 | 3.9223 |
| cold-j112 | 4 | 67 | 4800 | 6.289203 | 12.4650 | 1 | 4.7587 |
| cold-j113 | 4 | 68 | 4799 | 6.297628 | 12.4943 | 1 | 4.8292 |
| cold-j114 | 2 | 64 | 4800 | 6.294302 | 12.4848 | 1 | 4.1277 |

There are zero dropped presents in each capture. Queue traces confirm the
configured maximum of two/four/four/two constructed or unclaimed roots per
layout, zero orphan queue events and zero unfinished requests. Every held frame
matches a same-job scene-coverage snapshot and a geometric blocking row.

The apparent benefit against the first control does not repeat against the
last control. These four cases do not establish an isolated causal estimate of
render cost, but they do reject treating four roots as an accepted solution.

## Additional construction attempts are often canceled

For foreground requests whose construction starts in the first 1,500 ms of
input, the cohort is:

| Case | Started | Eventually ready | Pool-satisfied cancellation before ready | Construction elapsed median ms | Construction elapsed p95 ms |
|---|---:|---:|---:|---:|---:|
| cold-j111 | 91 | 90 | 1 | 9.73635 | 20.14532 |
| cold-j112 | 106 | 83 | 21 | 10.50630 | 98.58124 |
| cold-j113 | 102 | 83 | 19 | 9.86140 | 71.04358 |
| cold-j114 | 91 | 91 | 0 | 9.55130 | 13.37635 |

These are complete lifetimes of a start-time-selected cohort, not a count of
completions within 1,500 ms. Construction elapsed includes suspended incubation
and is not CPU time. The first four-row run also has two preempted requests in
this cohort.

`MasonryLayout::popBrickItem()` cancels a pending request with reason four after
the delegate pool supplies a usable item. Ready-first handoff protects completed
roots, but roots still under construction can be superseded by a recycled
delegate. The four-root candidate increases these incomplete attempts sharply.
This identifies a concrete reason why admitting more construction alone is
unproductive; the trace does not quantify the CPU cost already spent on each
canceled root.

## Admission and analysis limits

- Cache-before frames/pending were 48/0, 47/5, 52/0 and 48/0; misses were
  66/74/60/66. Faster startup changes work still pending when the readiness-driven
  recorder begins. These initial conditions are retained rather than normalized
  away. A separate fixed-early-input scope remains unverified.
- Source caches were empty before QML; OS file-cache coldness is not claimed.
- Unmatched native commits are 0/0/1/1. Full records remain in motion-analysis
  JSON. Unmatched presents and display-pair gaps are zero.
- Every analysis excludes 4,794 wheel-curve pairs and admits no physical-error
  pairs. Cadence is not proof of optical or physical-motion accuracy.
- Full-folder warm tour, quarter-height middle-button motion, final rendered
  175% leaves and portable payload extraction were not verified here. No static
  visual geometry was changed by this configuration experiment.

## Tests and artifacts

The new test uses a real suspended incubation controller, requests four roots,
resets the catalog before publication, then verifies replacement identity and
the four-root bound. The unchanged two-root implementation fails the new case
(two started versus four expected), while the candidate passes. Existing
two-root suspension, rapid reversal/mode reset and presentation-delta cases
still pass. Positive: six QtTest results including fixtures. Negative: five
passes and one expected failure.

Selected resource-only QML imports and Windows GUI-subsystem checks each pass
three results including fixtures; the system-DLL audit passes. Existing warnings
about PinchArea's overridden enabled property, fixture MS Sans Serif, and an
undefined QPointF remain. Integration tests also retain the no-TLS-backend
warning. No complete-suite or release claim is made.

Raw data are under
`C:/Users/xs/AppData/Local/Temp/f4-current-cold-motion-20261031/cold-j111` through
`cold-j114`. Sources, scripts, build/test receipts, comparison and cleanup
records are in `artifacts/requested-row-concurrency-20261101`. Large closed
files were hash-verified and archived on P: with exact symlinks preserving
their original paths. No samples were removed.

All diagnostic process trees were closed and the temporary display mode
restored. The normal application remained alive. Keep the two-root baseline;
next attribute actual row-construction CPU and pool handoff work instead of
increasing concurrency again. The full 160 Hz goal remains unproven.

AI assistance has been used to create this output.
