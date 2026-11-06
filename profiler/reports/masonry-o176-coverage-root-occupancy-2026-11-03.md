# Masonry o176 coverage root occupancy, 2026-11-03

All 217 holds matched a readiness snapshot. The 1,354 queued blocker
occurrences reduce to 154 distinct held snapshots with a queued blocker
when deduplicated by hold timestamp and layout owner. Both construction
slots were occupied in each snapshot.

| Slots occupied by rows missing from the held viewport | Held snapshots |
|---:|---:|
| 2 | 117 |
| 1 | 15 |
| 0 | 22 |

Every occupied slot for a missing row was still constructing. Every
occupied slot for another row held a ready root. Those 59 observations
correspond to 29 distinct roots, each eventually taken. The median
per-root maximum observed ready age was 11.112 ms and the maximum was
17.653 ms. Capacity had opened by render time in 24 of the 154 snapshots.

This separates the remaining coverage problem into construction latency
and ready-root delivery. Most queued holds already had both slots working
on needed rows. Ready-root delivery alone can address only a subset.
Construction age includes suspended incubation and does not measure CPU
time. An earlier frame-boundary delivery experiment on the 192-row
workload shortened ready-to-take latency but did not improve aggregate
hold counts. Any retest should use the current 194-row workload and
measure both the occupancy breakdown and total holds.

## Request lead time

Deduplicating the o176 blockers by row and request serial, 88 were
queued at their first hold and 29 were constructing. Nine distinct
rows lacked a matching request. The 88 queued blockers had already
been requested a median of 84.09 ms before their first hold. Of
those, 77 eventually started, a median of 29.69 ms after the hold.
The 76 that reached ready spent a median of 9.26 ms from start to
ready, including suspended incubation. The 29 already constructing
had started a median of 9.36 ms before the first hold.

For this run, increasing the request window alone would leave most
blocking requests in the queue. The next diagnostic should record
priority positions when each construction slot opens and distinguish
busy slots from free slots awaiting the next admission callback.

Reconstructing continuous occupancy from the recorded lifetimes
shows that the 77 eventually admitted blockers waited a median of
29.75 ms after their first queued readiness snapshot. During those
waits, the median time with a spare slot was 5.69 ms. A median of
three other roots started first, with up to 26 in the longest cases.
The analyzer distinguishes layout owners and retains ready roots
until take or cancellation. Two synthetic tests passed. This proves
that other admissions contributed to the wait, but the trace still
lacks the priority order needed to decide whether those starts were
correct at the time.
