# Verified-warm scheduler attribution and active-list experiment

## Outcome

The active gate list allocates during rendering, but replacing its QList with a
16-slot QVarLengthArray did not establish a scrolling improvement. Keep the
prototype experimental. No production recipe, portable payload, SDK, or normal
application binary was changed in this experiment. Constant 160 Hz remains
unproven; cold wheel, quarter-height middle scrolling, and physical motion
acceptance remain outstanding on the final production graph.

## Full-window scheduler trace

Input: `artifacts/maximized-warm-admission-20261031/admitted-warm-trace-f1.json`.
Analysis: `artifacts/maximized-warm-admission-20261031/wait-warm-f1/`.
The trace covers all 30 seconds of input, following a verified tour of all 192
Downloads entries over 17 quiescent viewports. Density 152, DPR 1.75, 160 Hz,
normal maximized window, and foreground focus were checked. There were no cache
stores, misses, frame-count changes, or pending loads during timed input.

The scheduler trace has no lost events/buffers or reader errors. Independent
xperf comparison matches all 280 selected profile prefixes. Exact resume-stack
joins cover 13,368/13,368 GUI switch-ins (thread 114784) and 8,251/8,251 render
switch-ins (thread 6332). All 32 selected frames have unambiguous selected
PresentMon matches. One partial frame boundary is retained explicitly. Symbols
come from a separate map link with identical .text/.pdata bytes and RVAs; that
symbol-only executable was never run. ICF aliases and unknown symbols remain
explicit in the evidence.

This intrusive run recorded four display intervals above 9.375 ms and two
dropped presents; its maximum display interval was 18.7475 ms. It is attribution
evidence, not a clean acceptance capture. The longest whole frame, at
+29925.9258 ms, lasted 12.5771 ms: 9.7332 ms on CPU and 2.8439 ms off CPU on the
render thread. Its 8.181 ms render-signal range was entirely on CPU. Other long
frames include D3D11 beginFrame waits, with only small ready-to-run delays.
These are not evidence of general scheduler starvation, nor grounds to disable
vsync. A frame envelope includes pacing and must not be reported as exclusive
GUI CPU time.

CPU samples in the longest frame include active gate list reallocation through
QArrayDataPointer::reallocateAndGrow, QMovableArrayOps::emplace, and
F4VectorGateFrame. Other samples include scene visits, gate bookkeeping,
geometry uploads, buffer assignment, binding copies, and driver work. Sampling
does not establish an exclusive percentage for the list allocation or explain
every missed display interval.

## Isolated candidate and regressions

Directory: `artifacts/gate-active-inline-20261031/`.
Only renderer source delta:

```cpp
- QList<QSharedPointer<F4VectorGate>> active;
+ QVarLengthArray<QSharedPointer<F4VectorGate>, 16> active;
```

The container still grows on the heap above 16; order, shared ownership,
admission, ancestor traversal, and painter order are unchanged. The candidate
uses the same main-only warm observer as the control and the current production
library graph. Host hashes:

- Control: `2a366f5b405a828b06e2cba2ebfc452f115eec76cc5947d9d61a8704d935f8ec`.
- Candidate: `e93014a657974cec3495bff4cbd8f319d2cfe63572292d2fb97a632d568c1720`.

The storage test derives the declaration from each actual renderer source. The
original fails 4,000 inline-storage checks across 8,000 scenarios while passing
all semantics. The candidate passes all 8,000 scenarios, including heap growth
at 17/32/128 elements, ordering, ownership after registry clearing, and cleanup.
Both arms pass all 44 native rendering tests at actual DPR 1.75. Nine rendered
PNGs are byte-identical, covering retained merged/unmerged roots, scissor,
stencil, bounded order scratch, and vector gate completion/reentry. No static
UI layout or text hierarchy was modified by this container experiment.

## Clean paired comparison

Fresh owned processes; order control/candidate/candidate/control. Each capture
has its own verified 192-entry, 17-stop warm tour followed by 30 seconds of rapid
wheel input. Every capture has zero timed cache deltas and foreground/display
guards satisfied. No UI Automation, screenshots, compiler, kernel trace, or
offline analysis ran during timed input.

| Case | Render p99 ms | Render max ms | Display max ms | Misses >9.375 ms | Dropped presents |
| --- | ---: | ---: | ---: | ---: | ---: |
| control-a1 | 2.710672 | 3.6303 | 6.6198 | 0 | 0 |
| inline-b1r | 2.668734 | 8.3627 | 18.7487 | 5 | 4 |
| inline-b2 | 2.651412 | 4.9202 | 6.4098 | 0 | 0 |
| control-a2 | 2.663678 | 6.3255 | 6.4648 | 0 | 0 |

Full per-case raw JSON, PresentMon CSVs, analyses, warm verdicts, identities, and
summaries are retained in the corresponding subdirectories. The second control
p99 overlaps the candidate values; the allocation removal does not justify a
performance promotion. PresentMon display intervals are not optical scanout or
physical-motion validation. No coverage or velocity acceptance is claimed.

## Invalid capture and storage recovery

`inline-b1` is retained as invalid: D: filled during result writing and left an
empty input JSON. It has no valid performance verdict and was not silently
excluded as a successful run. `inline-b1r` is a separately named fresh-process
repeat. Closed historical probe executables were copied to C:, SHA-256 checked,
and replaced by readable symlinks; receipts are in `closed-host-relocation.json`
and `closed-test-relocation.json`. No raw trace or user file was deleted. A
full-disk recovery receipt was written on C: before releasing the verified
archived source's storage. The ordinary running application was preserved.

## Next investigation

Do not repeat ancestor caching or claim that removing this allocation fixes
spikes. The next candidate needs evidence of repeated expensive CPU work, or a
direct explanation of a missed display deadline, rather than another isolated
sample. Preserve the distinction between on-CPU rendering, expected pacing
waits, and actual display misses. Complete cold and middle-scroll motion
acceptance before declaring the broader goal achieved.
