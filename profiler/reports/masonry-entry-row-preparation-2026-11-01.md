# Masonry: cold row-preparation bursts

Status: attribution progress, not a new scheduling implementation. Constant 160 Hz remains unachieved.

The previous goal turn progressed by measuring complete model events and ruling out a long publication burst in cold-k13. This analysis reuses four existing zero-overflow native captures; no new timing run, host or production rollout is needed.

## What the slow preparation phase contains

The native phase-probe commit at +29.2620 ms in cold-k13 lasts 7.0685 ms, including 6.5495 ms preparation. Its ten bounded row-preparation scopes total 6.4601 ms; the largest is 0.7623 ms. They cover consecutive catalog rows 58–67. There are no nested nearby-preparation callbacks in this commit. Its ten model visual-snapshot spans total only 0.0945 ms. The source starts these row scopes after `ensureBrickAt`; that lookup is not included. The scope is enabled for a missing delegate or nearby preparation. With no nested nearby callback, these are missing-delegate row preparations. The factory/pool boundary itself is not instrumented, so these timings must not be described as ten proven heap allocations.

This points to accumulation of ordinary row costs, not a single 6-ms outlier in snapshot lookup. Existing QML creation stacks corroborate component creation as a relevant path, but do not assign every microsecond of each preparation scope to it.

| Case / commit start | Preparation phase | Missing-row scopes | Row-scope sum | Largest row scope | Snapshot sum |
| --- | ---: | ---: | ---: | ---: | ---: |
| cold-j12 / +150.9468 ms | 6.2214 ms | 7 | 6.1047 ms | 1.0526 ms | 0.0884 ms |
| cold-j13 / +32.9090 ms | 5.6576 ms | 8 | 5.5405 ms | 0.7677 ms | 0.0748 ms |
| cold-k13 / +29.2620 ms | 6.5495 ms | 10 | 6.4601 ms | 0.7623 ms | 0.0945 ms |

The script also analyzes cold-k12 and retains every qualifying first-250-ms commit in each case's `entry-row-preparation.json`. Different cache states and instrumentation mean this is attribution across captures, not a controlled performance comparison. Aggregates are inclusive elapsed times. The parser validates complete phase groups using `VisualPhaseCensus20261101.correlate`; nested same-owner phase records are not silently assigned to an outer commit.

## Constraints on the next candidate

`MasonryLayout::prepareDelegateRow` acquires missing delegates through `GalleryDelegatePool::acquire`; its factory calls `QQmlComponent::createWithInitialProperties` synchronously. Existing delegates remain available for reuse, with retained recent rows and bounded free spares. This is not evidence that enlarging the pool alone helps: additional retained bindings/scenes can add work.

The current materialization plan extends delegates toward the native motion target while retaining the metadata-window budget. This protects real render-thread camera coverage. `GalleryLayoutEngineTest::motionTargetReusesTrailingBudgetForFastScroll` records a prior Downloads failure: the GUI snapshot was at 609.7595 while the first missing row started at 1916, already needed by the next camera pose. Removing look-ahead to make a shorter benchmark pass would reintroduce a known problem.

The next candidate should prepare reusable delegates before this synchronous burst, with bounded allocation and cancellation/revision behavior. Its evaluation must distinguish factory creation from pool reuse and verify initial coverage, rapid reversal, distant jumps, SVG scenes, folder previews and actual 175% visual leaves. It must also measure first-preview latency and binding/render cost; moving work earlier or retaining more objects is not automatically a win. Warm GC/binding spikes remain a separate unresolved part of the full goal.
