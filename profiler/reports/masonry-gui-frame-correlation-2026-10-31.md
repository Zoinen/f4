# GUI event and full-frame correlation, 2026-10-31

The current evidence shifts the next investigation toward GUI scroll-position
commits, scene/component work, and frame/presentation scheduling. It does not
establish steady 160 Hz or a single cause for every long display interval.
No production change is deployed in this investigation.

## Existing production-graph captures correlated

Used the corrected maximized-startup production-sized-clean-c1/c2 captures and
maximized-wheel-wait-d1. These retain the current production renderer/gallery
objects; the last also has intrusive scheduler tracing. All input guards retain
Downloads, 192 entries including parent, Masonry density 152, actual DPR 1.75,
160 Hz and foreground state. No outlier or dropped row is removed.

There are respectively five/zero/three long display intervals, with two/zero/one
dropped present rows. In c1, an 18.7718-ms display interval belongs to a Present
entry 7.5598 ms after recording starts, before the first wheel event. Its complete
native frame takes 1.4035 ms. A later c1 long display interval has a 5.9018-ms
native frame and a 0.7859-ms render range, with no GUI synchronization signals.
In d1, the late long interval has a 7.4653-ms whole frame and 1.5661-ms render
range. The correlation retains each containing frame, its neighbors, phase
durations, nearby input handlers and boundary gaps. Missing/partial frame
boundaries remain explicit rather than guessed.

All three captures contain one swapchain each, in Composed: Flip mode; these
are not occasional rows from another application swapchain. Some GPU completion
timestamps precede the prior nominal refresh by more than 12 ms. That is not
proof that DWM should have selected that frame: queue position and compositor
eligibility remain unmeasured. PresentMon notifications are not optical scanout.

Qt's local 6.11.1 render-loop source emits beforeFrameBegin before calling
QRhi::beginFrame, and performs synchronization afterward if requested. Therefore
beforeFrameBegin-to-beforeSynchronizing includes frame pacing/beginFrame work;
it must not be labeled GUI CPU cost. When synchronization is absent, the
begin-to-beforeRendering range still includes pacing. Whole-frame, render-only,
GUI callback wall time and display intervals are distinct measurements.

## Main-only GUI attribution probe

The diagnostic subclasses QGuiApplication to record GUI-thread notify spans and
adds the existing frame/input recorder. Renderer and gallery objects are
unchanged. Class names are copied before dispatch, with pointer-reuse/name
checks; no receiver is dereferenced after dispatch. Receiver addresses are
opaque identities. Raw spans preserve nesting depth, class, event type and
timestamps. Native event enum values were checked in the current Qt header.

The completed v2 observer retains every top-level event and every nested event
lasting at least 0.05 ms. Other nested events are counted explicitly. It records
108320 spans from 280701 notifies, with 172381 fast nested spans omitted, zero
buffer overflow and zero null receivers/events. Top-level spans do not overlap;
count accounting and timestamp ordering pass. Nested inclusive durations are
not added together as CPU time. A bounded overflow would mark the capture
incomplete rather than terminate the app.

Host SHA-256:
988676f6c5f369c9e11ea35f3b009a345f04388044d47f02ace8bb14f854427f.
Go bytes match the normal packaged app, SHA-256:
ac68c2e7c2fd259b760f91fc7c001be26d77ec2b33c86c36d1f22893a53fe613.
Startup uses the owned maximized profile. GUI notify instrumentation adds
overhead; this is attribution evidence, not a clean timing comparison.

## Observations from the completed GUI capture

| Top-level event group | Count | Inclusive wall total, ms | Median, ms | p99, ms | Maximum, ms |
| --- | ---: | ---: | ---: | ---: | ---: |
| F4HostWindow queued MetaCall | 2460 | 17517.5851 | 6.7067 | 11.9024 | 14.3467 |
| GalleryScrollAnimation queued MetaCall | 2546 | 4238.6377 | 1.24485 | 5.78632 | 14.6565 |
| QWindowsGuiEventDispatcher timer | 4319 | 1452.6573 | 0.2287 | 2.72454 | 9.8801 |
| QQmlApplicationEngine queued MetaCall | 31 | 133.7630 | 5.2833 | 6.66346 | 6.7672 |

Window MetaCalls wrap nested UpdateRequest events with nearly the same duration;
these include polish/synchronization and waiting, not necessarily equivalent CPU
execution. The scroll-animation class's queued callback reaches 14.6565 ms at
89.45 ms into input. The source posts syncNativePose to that object, whose wheel
path calls MasonryLayout::setContentY and commits viewport/delegate state.
Class/type timing alone does not identify every MetaCall's exact method or
prove how much of the duration is on CPU.

The run also records 62981 top-level QObject DeferredDelete events and 797
GalleryThumbnailShaderLayer deletions, plus 771 GallerySceneItem deletions.
GalleryComponentHost source constructs a new incubator for a new component and
deletes the previous ready item after exchange. These facts make component
creation/retirement a concrete hypothesis; they do not prove that every engine
MetaCall is incubation or that retaining hidden scene graphs would help.
Earlier retention experiments must not be repeated without measuring their
extra binding/render-tree cost.

The input recorder's enclosing timer was matched for every one of 2764 wheel
events. Time after its measured handler, including property sampling and JSON
append, has median 0.0162 ms, p99 0.0325 ms and maximum 0.1618 ms. Thus this
specific bookkeeping segment is not the observed 10–15-ms GUI span. This does
not prove that all observer effects are negligible.

The GUI probe has three long display intervals and one dropped row. A late miss
at Present+25125.10 ms follows engine callbacks lasting 2.9065 and 5.5066 ms and
a window MetaCall lasting 9.3056 ms. This is temporal correlation, not exact
compositor causality. All spans and outliers remain in the results, including
the initial Present before the first wheel event.

## Failed probe retained

The first all-notify observer terminated with Windows exception 0xc0000409 at
offset 0x469728, around 28.36 seconds into input. Both diagnostic processes were
confirmed terminal; raw PresentMon data and the failed binary/source/receipts
remain. That probe had a fatal 262144-record capacity check. The completed v2
run sees 280701 notifies in 30 seconds, consistent with capacity exhaustion,
but no stack or fatal message establishes the exact first failure cause.
The failed run is not used for performance acceptance. V2 was built separately;
it neither overwrites the failed evidence nor restarts based on a timeout.

## Next action and completion limits

Measure work inside syncNativePose's viewport/delegate commit and identify the
engine callbacks before implementing a fast path or retention change. A likely
useful boundary is unchanged visible/delegate coverage versus a real metadata,
layout or identity change, but the existing revision, selection, thumbnail,
native-SVG, motion and pixel-grid rules must be audited and regression-tested.
Do not skip GUI state commits merely because rendering runs on another thread.

Original acceptance remains incomplete: cold rapid wheel, a verified
all-192-entry quiescent warm tour, quarter-height middle motion, physical motion
error, coverage holds, all-format/native-SVG behavior and resting alignment at
175% have not been jointly proven on the current production build. No complete
160-Hz claim, commit or push is made. The normal app is unchanged and alive.
Only owned diagnostic trees are stopped. Compact evidence and SHA-256 references
to raw data and binaries are frozen beside this report.

> AI assistance has been used to create this output.
