# Masonry o171 residual stalls, 2026-11-03

The first-viewport-qualified o171 capture showed one 25.006 ms display
interval. Relative to the fixed input start, the previous Qt frame finished
at 166.023 ms. Qt recorded no next frame begin until 195.095 ms, leaving a
29.072 ms gap outside the measured render stages. The next frame's
synchronization took 0.370 ms and its render-signal interval took 1.445 ms.
PresentMon measured 0.097 ms in Present and 0.662 ms to render completion
for the late present. The last input before the gap took 1.684 ms and the
next delivered input arrived at 204.641 ms. A scheduler trace is needed to
distinguish an unscheduled thread from a wait or uninstrumented work.

All 215 coverage holds joined to a render coverage record. Their last
occurrence was at 2,220.773 ms. The largest requested-to-visible displacement
was 488.692 physical pixels. The queue had both construction slots occupied
at synchronization for all 1,616 queued blocker occurrences. At the held
render, 460 of those occurrences had spare capacity and 1,156 still had both
slots occupied. These counts are row occurrences, so a row may recur across
frames. They distinguish pressure on the construction slots from the delay
between a slot becoming available and the blocking row being admitted.

These are separate failures. The display gap occurs before a Qt frame begins.
The coverage holds occur when a frame renders but the next rows are not ready.
The queue trace does not establish CPU residency or prove a priority defect.
Keep both cases in the acceptance run.
