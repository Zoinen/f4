# Visible transaction phases and current GC-budget comparison, 2026-11-01

## Decision

The new phase totals identify visual binding and transaction completion as
substantial contributors to current long visible commits. A delayed zero-loss
CPU trace confirms GC inside one such completion-dominated commit. A fresh
5/1/1/5 ms GC-budget comparison shortens completion maxima in both 1 ms runs,
but does **not** consistently improve displayed-frame delivery. The default is
unchanged; the 160 Hz goal remains unachieved.

This is a current-graph confirmation and a new phase/budget measurement, not a
claim that GC was previously unknown. The [earlier activation investigation](masonry-current-row-activation-gc-2026-10-31.md)
already found GC during completion. Earlier null-string reuse and GC-budget
experiments were inconclusive; they are not silently promoted here.

## Complete six-phase accounting

The diagnostic accumulates six elapsed totals inside each property commit and
records six small counter rows at completion. It does not append one record for
every row-stage visit. Counter values are nanoseconds in field A; their tiny
record spans are not the measured phase durations. Correlation selects the
innermost containing commit, rejects missing/duplicate/negative counters and
requires phase totals not to exceed the complete elapsed span. Four parser
regressions pass, including nested-commit assignment and reading the aggregate
value instead of the record width.

`cold-y1` records 78,317 spans with zero overflow and 2,787 post-input commits.
Actual Downloads contains 192 entries; Masonry density is 152, with the real
160 Hz/DPR 1.75 desktop. Input lasts 30 seconds and starts at the first viewport
with 15 cached frames and 13 pending requests. Gallery storage is empty before
QML; the OS cache is not flushed. Observer checks before/after are clean.

| Commit begins | Complete elapsed | Visual binding | Completion | Other phases/remainder |
|---:|---:|---:|---:|---:|
| +14923.6539 ms | 11.1680 ms | 7.7475 ms | 1.2112 ms | 2.2093 ms |
| +11992.7537 ms | 8.7587 ms | 1.2994 ms | 5.8747 ms | 1.5846 ms |
| +17890.6629 ms | 8.5216 ms | 0.4477 ms | 7.2032 ms | 0.8707 ms |

Over the full post-input observation, phase totals are preparation 477.9718 ms,
identity binding 210.7657 ms, visual binding 1050.0478 ms, layout 522.8812 ms,
finalization 481.2152 ms and completion 430.4192 ms. These are elapsed phase
measurements, including instrumentation and any waits, not exclusive CPU time.
Nested contexts remain explicit. The probe changes no product scheduling,
geometry, notification policy, preview coverage or GC behavior.

## Delayed CPU attribution

`cold-y2` requests a three-second Profile/CSwitch trace 15.5 seconds after the
pre-launch hook. It covers mid-scroll rather than startup. The inherited
configuration/closure files still say `startupAttributionOnly`; this inaccurate
inherited label is preserved and explicitly corrected by a supplementary scope
annotation. The event envelope lies at +14200.2660 to +19099.6091 ms relative
to input. Rundown can extend the envelope beyond actual profile coverage;
samples are required at the selected interval rather than inferred from that
envelope alone.

Both native readers report 878,873 events, zero event/buffer loss, zero schema
errors and successful ProcessTrace completion. The documented profile-prefix
fallback independently matches **214/214** xperf reference rows. A symbol-only
link has identical `.text` and `.pdata` bytes/RVAs to the captured host and is
never run. Unknown/JIT addresses and ICF aliases remain explicit.

A commit at +15748.0699 ms lasts **7.6421 ms**, all of which the GUI thread is
scheduled. Its completion total is 5.6442 ms. Of seven GUI samples, six contain
`QV4::MemoryManager::runGC`; sampled states include global-object marking,
weak-value marking and mark draining. The allocation route is
`MemoryManager::allocString` → `ExecutionEngine::newString` → `QV4::loadProperty`
→ `Lookup::getterQObject`, ending at an unresolved JIT address. This does not
identify the exact QML property, nor prove GC is responsible for every spike.
Sample counts are not converted into exclusive CPU percentages.

The current source also contains synchronous icon/Loader paths, but this trace
does not justify labeling the measured completion spike as icon decoding.
Neither asynchronous icon loading nor a speculative GC algorithm patch is made.

## Current budget comparison

One new host adds only a pre-QML read of the actual engine GC-state-machine
budget, written once outside timing. Each fresh-process capture explicitly
clears an inherited budget override, then selects default or `QV4_GC_TIMELIMIT=1`.
Native values verify **5000/1000/1000/5000 microseconds**, respectively. Both
arms use the same host and phase instrumentation. No analysis, compilation,
tests or Computer Use helper run during the four timed observations.

| Case | Native budget | Cached frames/pending at input start | Display intervals >9.375 ms | Dropped presents | Max display interval | Max commit | Max completion phase |
|---|---:|---:|---:|---:|---:|---:|---:|
| cold-z1 | 5000 µs | 4 / 23 | 4 | 0 | 12.5141 ms | 14.0817 ms | 7.5582 ms |
| cold-z2 | 1000 µs | 3 / 24 | 3 | 0 | 12.5049 ms | 6.9839 ms | 1.8450 ms |
| cold-z3 | 1000 µs | 5 / 4 | 5 | 0 | 18.7507 ms | 15.4924 ms | 3.1625 ms |
| cold-z4 | 5000 µs | 24 / 12 | 2 | 1 | 12.5056 ms | 12.4964 ms | 6.4781 ms |

The maximum commit beginning after +5 seconds is 10.0283/6.5703/5.4287/10.1871 ms
in the same order. This is an additional descriptive split, not a replacement
for complete-run acceptance; all startup/boundary misses and drops remain.
Startup loading states vary and a quiescent full-folder warm tour is not proven.
The 1 ms arm still has a 13.6200 ms layout phase in one early commit; without a
same-interval CPU trace, that elapsed maximum is not assigned to a specific
function or to GC.

Both 1 ms runs shorten completion maxima, but display results remain mixed:
controls have six long display intervals combined and candidates eight. One
control has a dropped present. These four runs do not establish a statistically
reliable display win or prove all required behavior at 1 ms. No environment
default, Qt source, SDK or production executable is changed.

Coverage holds are zero in all four runs. Missing commits, frames without motion
commits and every excluded wheel pair remain in the manifest/raw analyses.
There are no valid wheel physical-error samples and no optical acceptance.

## Identity, failures and preservation

Phase host SHA-256:
`9679502b630507f73c13ace6b67c4deee7f17c7ec4c6ea09936438ad32e54be1`.
Budget-observer host SHA-256:
`14009d9735a71c6118b6da725d0b47b761d5ddb5e119d579a39a359a4596ac4b`.
Windows GUI subsystem and system-DLL import audits pass for the published hosts.
These remain worktree diagnostic overrides, not portable payload rollouts.

The first stage audit used a wrong relative path in its dumpbin command and
failed before stopping the preceding diagnostic or publishing the host. The
command was corrected and both audits passed. No performance attempt was
discarded. Two delayed-trace harness regressions pass, covering cancellation
before a scheduled start and propagation of a delayed start failure.

Only explicitly owned diagnostic trees are stopped. Closed preceding diagnostic
hosts are copied to the dedicated P: profiling archive, hash-checked and retained
at their original paths via symlinks. Closed trace compression verifies hashes
before/after. All captures and all six-phase records remain. The normal
ac68c2e7/653cbd8a application is preserved. No commit or push occurs.

## Remaining work

Reduce verified allocation/notification churn, and separately attribute the
remaining long layout commits. Do not equate fewer allocations or shorter GC
phases with no-spike display delivery. Initialized-app cold folder entry,
quiescent full-folder warm wheel and quarter-height middle-scroll, physical and
rendered motion verification, sustained display acceptance and portable rollout
gates remain open.

AI assistance has been used to create this output.
