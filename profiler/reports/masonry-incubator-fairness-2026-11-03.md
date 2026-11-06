# Masonry incubator fairness, 2026-11-03

The native fixture submits two roots to a manually driven incubation
controller. Each root has a nested Loader. The stock build reported
ready order 2, 1. A diagnostic override for both Qt incubation loops
selects the oldest runnable entry, skipping parents waiting on
nested incubators. The fixture then reported ready order 1, 2, with
both nested items available. The override was linked into an isolated
diagnostic host. [Qt 6.11.1 source](https://github.com/qt/qtdeclarative/blob/v6.11.1/src/qml/qml/qqmlincubator.cpp)

Both runs used the initialization probe, 194 entries, a 160 Hz
display, DPR 1.75, and verified first viewport and native foreground.

| Metric | o179 stock | o180 oldest runnable |
|---|---:|---:|
| Median start to initialization | 9.04 ms | 20.03 ms |
| Maximum initialization exit to ready | 1,600.17 ms | 42.31 ms |
| Median total construction elapsed | 9.93 ms | 21.54 ms |
| Maximum total construction elapsed | 1,644.03 ms | 68.38 ms |
| Coverage holds | 242 | 320 |
| Maximum display interval | 37.50 ms | 12.51 ms |
| Display intervals above 9.375 ms | 3 | 3 |
| Skipped input slots | 215 | 180 |

The fairness change sharply shortened the finalization tail in this
pair, while more than doubling median construction elapsed and
increasing coverage holds. The experiment is insufficient for
adoption. A narrower follow-up can preserve the existing order for
new roots and favor an older root only after it has initialized,
provided its nested dependencies can still run.

## Prefer initialized roots

The o181 override retains the stock selection when all runnable
roots are uninitialized. The fixture submits a newer root during
the older root's initialization and forces the current slice to
expire. Stock Qt completes the newer root first. The override
completes the initialized root first, and both nested Loaders
reach ready. Submitting both roots before the first slice retains
stock order.

The live run reduced maximum post-initialization time to 19.56 ms
and kept median pre-initialization time at 9.44 ms. The longest
pre-initialization wait rose to 1,849.68 ms. Coverage holds rose
to 285 versus 242 for stock, and the largest display interval
was 43.74 ms. Both first-viewport and foreground gates passed.
The policy moved the worst wait into the uninitialized phase.
Neither strict oldest-first nor initialized-first meets the
coverage limit. A follow-up can test bounded aging that applies
to both phases while allowing nested dependencies to run.

## Age threshold

The o182 diagnostic keeps stock ordering until a runnable
incubator has been queued for 25 ms, then selects the oldest
eligible entry. The native fixture confirmed stock order for
two fresh roots and older-first order after the threshold, with
both nested Loaders ready.

In the live run, the worst pre-initialization and post-initialization
spans were 80.20 and 48.70 ms. Maximum total elapsed was 81.19 ms,
versus 1,644.03 ms in o179. Median total elapsed rose to 21.43 ms.
Coverage holds rose from 242 to 335. Four display intervals exceeded
9.375 ms, with a maximum of 100.01 ms. The first-viewport and
foreground gates passed, and the recorder skipped 189 input slots.

The age threshold contained the multi-second tail in this capture,
but did not improve scrolling. Keep this global Qt scheduler change
isolated. The result reinforces the need to reduce or decouple the
work required to publish the next visible row.
