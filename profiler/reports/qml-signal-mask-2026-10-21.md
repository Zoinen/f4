# Wider QML signal mask removes unnecessary render-thread notifications

This report records the artifact-only comparison before deployment. The fix was
subsequently integrated and deployed as described in
[the production verification report](qml-signal-mask-production-2026-10-21.md).
Nothing has been committed or pushed. Constant 160 Hz remains unverified.

## Cause and rejected first idea

The sampled wait path reaches QQmlData::signalEmitted, which forwards a
cross-thread signal through a temporary proxy and a posted event. Its actual
QML endpoint is looked up later on the owning thread. QtCore already calls the
thread-safe signal-connection guard before entering that path. Adding the same
guard inside signalEmitted is redundant and was rejected after the baseline
test showed ordinary unobserved signals were already filtered.

The connection guard is a 64-bit conservative mask: signal indices separated by
64 can collide. The root window has enough signals and property listeners to
trigger false positives for render-stage signals with no actual QML endpoint.
Live counters confirm those endpoints are absent, while tens of thousands of
proxy events are still delivered in the control runs.

Sources: [QtCore activation](https://github.com/qt/qtbase/blob/v6.11.1/src/corelib/kernel/qobject.cpp#L4259),
[Qt QML forwarding](https://github.com/qt/qtdeclarative/blob/v6.11.1/src/qml/qml/qqmlengine.cpp#L291),
[connection-mask implementation](https://github.com/qt/qtdeclarative/blob/v6.11.1/src/qml/qml/qqmldata_p.h).

## Isolated implementation

A replacement Qt QML implementation object uses a 512-bit conservative mask.
The existing NotifyList prefix and 64-bit mask remain intact for SDK-built
callers. A private derived allocation adds eight atomic words, maintained by the
same translation unit that owns list allocation, deletion and reset. Matching
typed deletion is updated too. SDK headers and libraries are not modified.
Relaxed cross-thread connection-order semantics remain the same as Qt's
documented internal contract. Collisions 512 indices apart remain conservative.

The prototype adds 64 bytes per notify-list allocation. Its per-process flag
F4_PROFILE_DISABLE_QML_SIGNAL_MASK selects the original check for same-exe
comparisons; both arms use the same allocation shape. This is a diagnostic
control, not a new user setting. Source replacement is checked reversibly and
build receipts pin the Qt 6.11.1 source, SDK inputs, object and linked host.

## Functional checks

The meaningful baseline regression fails because a 64-position collision queues
two proxies instead of the one real signal. Ordinary unobserved signals, copied
arguments, property binding updates and C++ receivers continue to work. A first
test counter incorrectly relied on the proxy's metaobject name; it was corrected
to inspect the actual generic-QObject receiver and source before accepting the
baseline regression.

The expanded candidate tests pass: 8 passed, zero failures or skips. They cover
all 513 generated signal indices, connected arguments and properties, C++ queued
receivers, a 512-position collision, and 50 repeated engine/notifier lifecycles.
The same executable with the original mask selected reports 7 passes and the
one expected collision regression. Import isolation and compiled-host QML
loading pass (4 tests); the Windows system-DLL dependency audit passes.

A test rebuild regenerated the pinned Qt object. The only difference was its
four-byte COFF timestamp. The pinned bytes were restored by matching the original
SHA-256; the recompiled copy and timestamp-restoration record are retained.
Every input in the captured host receipt verifies afterward. Further test-only
rebuilds should not recompile the pinned Qt implementation object.

## Live comparisons

Both control/enabled and reversed enabled/control pairs use the same executable
and receipt, 187 entries, density 152, window 2194x1186, DPR 1.75 and 160 Hz.
Corresponding cold/warm extents match. Material micro-probes and detailed D3D
pass probes are off; the outer-pass CPU counter and common native probes remain.
Both rejected texture candidates and scheduler tracing are off. Existing vector
upload prototypes are held fixed in this diagnostic base; they are not promoted
by this experiment.

Cumulative unwanted proxy deliveries for nine unobserved render-stage signals:

| Pair | Cold snapshot control / enabled | Warm cumulative control / enabled |
| --- | ---: | ---: |
| First | 27,340 / 0 | 57,349 / 0 |
| Reversed | 26,169 / 0 | 52,443 / 0 |

Warm snapshots include cold and intervening activity; do not sum these columns
or treat them as exact per-input rates.

Outer begin-pass p99, control -> enabled, milliseconds:

| Pair | Cold 0 | Cold 1 | Warm 0 | Warm 1 |
| --- | ---: | ---: | ---: | ---: |
| First | .0284 -> .0031 | .0260 -> .0031 | .0500 -> .0031 | .0311 -> .0032 |
| Reversed | .0368 -> .0032 | .0238 -> .0032 | .0347 -> .0034 | .0529 -> .0030 |

The affected path improves in all eight blocks. Render-to-frame-end p99 is also
lower in all eight, although one difference is only 0.0001 ms and is not a
meaningful isolated benefit. Preparation and recording p99 do not each improve
uniformly. This removes identified work; it does not explain every frame tail.

Enabled blocks have zero counted >9.375 ms displayed intervals, but first-block
displayed-step maxima remain roughly 6.46-7.31 physical pixels, above the
unchanged 2 px limit. The strict goal gate is not passed or relaxed. Broad Qt/UI
regressions, portable integration and production-payload verification remain
before promotion. CPU shader-binding copies and the remaining cadence tails are
the next targets.

Evidence: artifacts/analyze-qml-mask-pairs.py,
artifacts/qml-mask-pairs-analysis.json, case-specific phase/motion snapshots,
artifacts/QmlSignalEndpointTest.cpp,
artifacts/build-qml-signal-endpoints.py,
artifacts/qml-mask-build-receipt.json and per-case files with
real-scroll-qml-mask- prefix under artifacts/modes-profile.
