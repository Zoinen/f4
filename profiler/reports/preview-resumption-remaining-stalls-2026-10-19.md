# Current preview-resumption build: remaining stalls — 2026-10-19

## Scope

Fresh diagnostic host built from the deployed preview-read-resumption source.
Native preparation, QML incubation and GC phase observers were rebuilt/relinked;
the native Core archive was based on the current normal Core library. Two
4-second wheel windows each in cold and warm captures, same process 68252,
GUI thread 70948, render thread 56648, engine 15cff8, window 2194×1186.
Downloads 186 entries, density 152, DPR 1.75, primary/output 160 Hz, GC budget
5000 us. No tests, build, UI Automation or offline analysis during capture.
The normal portable worktree app was restored afterward.

This instrumented capture diagnoses individual stalls; it is not a clean
cadence comparison with the uninstrumented A/B cohort. Trace observers add work.
Native arrays were below their capacity limits. Both traces report zero lost
events/buffers. Exact raw QPC switch alignment matched 49,185 cold and 35,902
warm switches at 10 MHz, each with 900 ns offset spread. Wall-clock alignment
is not used to assign individual samples. The producer interval corresponding
to a display miss is selected by endNs == presentNs, not a later context frame.

## Warm miss

One display gap exceeded 9.375 ms. Display gap 12.4992 ms; matching producer
interval 10.728 ms, ending at QPC ns 139484352673500. GPU active 0.4918 ms.
GUI scheduled running 9.952 ms, ready 0.013 ms, waiting 0.763 ms. Render thread
running 4.681 ms, ready 0.012 ms, waiting 6.035 ms. These thread times overlap
and must not be added into a serial frame time.

GC-state union inside that producer interval is 8.6537 ms. By observed state:
MarkPersistentValues 0.4307 ms, InitMarkWeakValues 0.0004 ms, MarkWeakValues
3.4345 ms, MarkDrain 4.7881 ms. A full MarkDrain event triggered from the event
loop spans 139484348857800–139484355892500 (7.0347 ms), extending beyond this
producer interval. A preceding allocation-triggered runGC is 5.5993 ms and
includes earlier marking work; do not add this enclosing timer to its states.
Nested native updateProperties reports 4.9653 ms, including 4.838 ms GC, so it
does not establish a five-millisecond native-property implementation cost.
No incubation or nearby preparation overlaps this particular interval.

Full available stacks include weak-value traversal and allocation-triggered
string creation (RuntimeHelpers::addHelper -> convertToString -> newString ->
allocString -> runGC). Some later frames have unknown/JIT or optimized static
symbols. Phase IDs identify MarkDrain; a sampled symbol must not override that
phase attribution. The instrumented library preserves Qt's original 10,241-item
deadline-check drain batch. This explains why a nominal deadline is not a hard
upper bound, but this capture does not identify a safe replacement policy or
prove the exclusive cost of an individual marked object.

## Cold misses

Six long display intervals. Two overlap 4.4875 and 4.9438 ms of GC. Others have
zero overlapping GC, including a 15.4073 ms producer interval with 5.958 ms GUI
running and 7.3083 ms render running. Its native work union is 4.689 ms, with
2.2722 ms nearby preparation and 2.0371 ms properties; sampled render work
includes D3D driver, shader-resource snapshot destruction and scene-node visits.
Earlier misses include 0.8 ms incubation and roughly 2 ms nearby preparation,
plus image conversion, scene bookkeeping and resource binding. GPU active on
the six missed intervals remains 0.425–0.663 ms. There is no single exclusive
CPU cause proven for all cold misses.

## Next target

The current warm miss makes marking work a concrete target again. Measure
deadline overshoot per drain batch and live root/reference counts with bounded
observers, distinguishing object traversal from aggregate batch cost. Use the
already recorded negative 128-item and GC-budget experiments when designing a
new experiment; do not promote a smaller constant simply from this one trace.
Reduce avoidable live references/string allocation where a source-level cause
is verified. Keep cancellation, reference notifications, static pixel-grid
alignment and clean display cadence checks intact. Cold component/layout and
renderer resource work remains a separate target.

No additional production changes were made from this trace. The 160 Hz goal
remains active. The clean display comparison and tested notification fix are in
`preview-read-resumption-2026-10-19.md`.

AI assistance has been used to create this output.
