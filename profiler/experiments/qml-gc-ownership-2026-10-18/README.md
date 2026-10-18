# QObject ownership and GC marking experiment

This is a diagnostic prototype, not a production change. It is not enabled
in the dependency recipe, normal Qt host or embedded payload.

`OwnQmlRoots.h` captures the exact ownership operation tested by the diagnostic
host: give the top-level window an engine QObject parent, and give otherwise
parentless retained panel loaders their existing visual parent as QObject
parent. Visual parentage and geometry are unchanged. The controlled workload
adopts five objects. The actual harness performs this once before settling and
scrolling, under F4_PROFILE_OWN_QML_ROOTS.

`qv4-mark-roots-observer.patch` adds counters/timing to the official Qt 6.11.1
QObject wrapper root walk. The diagnostic host instead extracts that same
function/helper into an override object linked before QtQml with
/FORCE:MULTIPLE; its duplicate-definition warning confirms the original
implementation is ignored. Logged root events establish the probe executes.
This is observation with additional overhead, excluded from clean timing.

The prototype eliminates the floating-root walk in this reproduction but
does not establish reliable display-cadence improvement. Retention of QML
properties, cache eviction, tab switching and full shutdown/lifetime behavior
are not verified. The close request exits the candidate process, but its exit
code was unavailable; that is not a full shutdown proof. Do not promote this
ownership operation without those checks and stronger performance evidence.

[Measurements and rejected rollout](../../reports/qml-gc-and-render-stalls-2026-10-18.md)
