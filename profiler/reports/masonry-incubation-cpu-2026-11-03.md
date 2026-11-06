# Masonry incubation CPU, 2026-11-03

The o174 verified CPU sample stream and row lifetimes were joined on
the native clock. The post-entry window from 250 to 2,500 ms contains
738 GUI CPU samples, 180 of which have a uniquely matched stack
containing `QQmlIncubatorPrivate::incubate`.

| Inclusive frame on those 180 stacks | Samples |
|---|---:|
| `QQmlObjectCreator::finalize` | 107 |
| `QQmlObjectCreator::createInstance` | 70 |
| `QQmlObjectCreator::setupBindings` | 62 |
| `QQuickLoader::componentComplete` | 39 |

Counts overlap. They are sampled CPU evidence, distinct from elapsed
construction lifetimes. There were 172 incubator samples concurrent
with at least one active requested-row lifetime. The overlap does
not prove which incubator or delegate produced a given sample.
Before 250 ms, the corresponding count was 11 of 259 GUI samples,
consistent with publication work also contributing to the earliest
gaps.

The row construction experiment should target creation and binding
setup. The entry root currently instantiates two layout-debug loader
shells even with diagnostics off, a shared preview, and separate
mode and folder hosts. The named Loader stack does not identify a
specific QML object. A targeted QML construction comparison is
needed before attributing those samples to any one of these items.

The 194-row captures expose a much longer tail than the roughly
10 ms median. In o176, row 15 occupied one of its layout's two
slots for 852.31 ms from start to ready. During that lifetime six
other roots on the same layout and 73 on other layouts reached
ready. The capture recorded 129 coverage holds in the interval.
The o178 direct-caption diagnostic had a similar 914.95 ms
lifetime for row 35, with eight same-layout and 66 other-layout
completions and 87 coverage holds during it. Those holds are
temporal overlap, not attribution to the slow row.

The queue and incubation engine continued to make progress while
each outlier occupied a slot. Add timestamps for
`QQmlIncubator::setInitialState` and `Loading → Ready` so the
next trace can distinguish delay before root initialization from
delay after initialization. Neither the lifetime trace nor the
sampled stacks identify the responsible component or its CPU
residency for these individual outliers.

## Initialization boundary probe

The o179 probe recorded a single in-memory scope at
`QQmlIncubator::setInitialState`, matched by owner and request serial.
The first-viewport and foreground checks passed at 160 Hz and DPR 1.75.
The analyzer joined 211 successful constructions started during input.

| Phase | Median | p95 | Maximum |
|---|---:|---:|---:|
| Start to initialization entry | 9.044 ms | 13.990 ms | 145.166 ms |
| Initialization callback | 0.003 ms | 0.005 ms | 0.014 ms |
| Initialization exit to ready | 0.834 ms | 36.680 ms | 1,600.175 ms |

The longest row took 1,644.030 ms. It spent 43.853 ms before
initialization and 1,600.175 ms after it. The next longest input
row spent 620.313 of 629.549 ms after initialization. These are
elapsed spans and do not show continuous CPU use. The evidence
points to the finalization/dependency phase for the worst outliers.
Trace the nested incubator or Loader responsible for readiness
before changing the admission limit or using synchronous completion.

## Engine scheduling order

The Qt 6.11.1 incubator controller drives the first entry of its
intrusive list in both bounded incubation loops. The installed
`qintrusivelist_p.h` inserts new entries at the head. This means
newer incubators can take successive time slices before an older
root resumes. Qt also waits for nested dependencies when the
parent has completed its own object creation. A fairness
experiment must allow those dependencies to run. [Qt 6.11.1 incubator source](https://github.com/qt/qtdeclarative/blob/v6.11.1/src/qml/qml/qqmlincubator.cpp)

After o179's row 4 exited initialization, 19 newer roots on the
same layout and 107 on other layouts both started and reached
ready before it. For the next longest row, 40 newer roots on other
layouts did so. This shows that the engine continued to service
new work while older roots waited. It does not isolate engine
ordering from nested dependencies in the specific lifetimes.
Test engine order directly with two roots and a nested dependency,
then evaluate an oldest-runnable scheduler in an isolated host.

## Content classification

The o184 diagnostic added snapshot flags at the initialization
boundary. Its first-viewport and foreground checks passed.
The worst post-initialization span was 1,561.61 ms on a folder
whose snapshot classified its preview as empty. The 15 folder
requests with a positive preview snapshot had a maximum of
12.62 ms. The largest non-image file and image waits were
137.29 and 100.38 ms respectively.

This run does not support removing the folder preview Loader as
the first explanation for the tail. The snapshot records what
the row knew at initialization. It does not report later
property changes or which nested incubators actually started.
The first probe binary used the wrong directory key and was
never run. These results use the corrected `isFolder` probe.
