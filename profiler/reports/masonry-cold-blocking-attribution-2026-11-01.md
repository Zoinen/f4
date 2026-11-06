# Cold scrolling: GUI execution versus renderer waits, 2026-11-01

## Finding

A zero-loss startup scheduler/CPU trace of the current row-replay candidate
localizes one early display miss to a busy GUI thread. During a 13.6063 ms
presentation interval, the GUI thread is scheduled for 12.9098 ms. The render
thread spends 7.3830 ms in a wait whose exact resume stack contains
`QWaitCondition::wait` → `QSGRenderThread::processEventsAndWaitForMore`.
Its ready-to-run delay is only 0.0064 ms. This interval does not support treating
the render thread's off-CPU time as GPU upload work or scheduler starvation.

The next measurement should split delegate creation/nearby preparation,
viewport-target commits and geometry/property publication by frame. A broad
renderer rewrite or further pixel rounding is not justified by this evidence.
The 160 Hz objective remains unachieved; no new production rollout occurred.

## Capture and integrity

`cold-w4` runs the diagnostic host with SHA-256
`ddd628de63b06da8cae40d1993045061bd723c739615f1380e0529c7cfc4410c`.
It uses actual Downloads (192 entries), Masonry density 152, the 160 Hz desktop
at DPR 1.75, and 30 seconds of first-viewport wheel input. Before QML the gallery
cache is empty; at input start eight frames are cached and 19 requests pending.
This is startup attribution, not folder entry in an initialized application,
and the OS file cache is not flushed. Observer checks before/after are clean.

The kernel session requests three seconds of Profile and CSwitch stacks, plus
process/module, scheduling, dispatcher, DPC and interrupt events. Rundown extends
the event envelope to 4.81965 seconds. Input starts 2.28778 seconds after its first
event. The trace therefore covers both early misses, not the full 30-second run.
The profiler is intrusive; this is not a clean timing-comparison arm.

Both independent native readers consume 906,329 events and report zero lost
events, lost buffers, schema errors or ProcessTrace failures. The profile reader
uses its explicitly recorded prefix fallback; xperf independently matches all
432 selected thread/IP/timestamp rows. A separate symbol-only link has identical
`.text` and `.pdata` bytes/RVAs to the captured host. It is never executed.
ICF aliases, unknown addresses and missing/ambiguous joins remain explicit.
Scheduled time includes any interrupts within the scheduled interval; CPU
samples are not exact function durations.

## Both displayed misses retained

| Presentation interval begins relative to input | Presentation interval | Display interval | GUI scheduled | GUI off CPU | Render scheduled | Render off CPU |
|---:|---:|---:|---:|---:|---:|---:|
| −1.5976 ms | 8.6133 ms | 12.4855 ms | 7.8306 ms | 0.7827 ms | 4.2081 ms | 4.4052 ms |
| +15.3609 ms | 13.6063 ms | 12.3719 ms | 12.9098 ms | 0.6965 ms | 6.1306 ms | 7.4757 ms |

The first interval crosses input start and is retained. The attribution helper
previously selected only long presentation intervals, which omitted this slow
display interval because presentation took 8.6133 ms. The new helper selects
either a presentation or display interval above 9.375 ms. Three regressions
cover short-present/slow-display, long-present/normal-display and normal cases.
Display acceptance thresholds are unchanged.

The larger render waits in both intervals resume through
`processEventsAndWaitForMore`; exact timestamp joins retain separate kernel and
user stack fragments. The GUI waits resume through
`QSGThreadedRenderLoop::polishAndSync`. CSwitch stacks are joined at the switch-in
timestamp, not guessed from a nearby sample or from the blocking timestamp.
ReadyThread events provide ready delay, but ReadyThread caller stacks are not
captured in this successful trace; the exact waking caller is not claimed.

## GUI work in the second gap

Samples include:

- Parsing/validating replayed metadata rows. The contained outer catalog-row
  operation is 0.5739 ms; its zero-pending-row installation is 0.0001 ms.
- `MasonryLayout::applyDelegateRowLayout` and `updateProperties`, reached through
  both lightweight geometry rewrap and `prepareViewportMotionTarget`.
- QML object/binding setup inside delegate creation from both `updateProperties`
  and the timer-driven `prepareNearbyDelegates`.
- Directory-preview publication, thumbnail visual-snapshot propagation and a
  popup-overlay pointer-event filter. A sample in a path does not by itself
  prove that path is redundant or individually responsible for the entire gap.

The bounded census has zero overflow. Within the second gap, eight contained
layout-data-change spans total 2.1533 ms inclusive; 17 visual-update spans total
1.1629 ms; two lightweight-flush spans total 0.7161 ms. Snapshot calls total
0.2894 ms. These overlap/nest and must not be added across kinds. They do not
fully explain the 12.9098 ms of scheduled GUI time.

Current nearby preparation limits itself to two delegates or two elapsed
milliseconds per callback, but starts its timer after retirement/release work.
A QML creation already in progress cannot be preempted by that check, and the
limit is per callback rather than a shared frame budget. The new trace justifies
measuring the complete callback and its individual creations alongside direct
viewport commits. It does not yet prove that a particular replacement budget
will improve scrolling while preserving coverage.

## Failed attempts and storage

`cold-w1` and `cold-w2` fail xperf startup with `0x80070070` before launching
the application. Kernel-session absence is verified afterward. One intervening
cleanup attempt refused a directory without all expected rebuild inputs; nothing
from that directory was deleted. A retry was made before that cleanup failure
was addressed and failed for the same storage reason. Both attempts remain.

`cold-w3` captures Profile/CSwitch/ReadyThread stacks for a requested five
seconds, reaches the exact 128 MiB cap, and fails the owned-stop check because
the kernel session has disappeared. Both readers report **75 lost buffers**.
The trace and recovered application identity are preserved; it is rejected for
timing attribution and is not relabeled as a successful capture.

The successful narrower trace keeps all scheduling events but captures only
Profile/CSwitch caller stacks. Its three harness tests pass, including matching
the declared stack configuration to the actual xperf command and refusing to
stop a foreign session. Closed data are compressed with before/after hash
checks. Explicitly superseded test executables are removed only after verifying
they are closed and their source, moc and object files remain. A duplicate stage
host becomes a verified symlink to its unchanged published bytes. Normal F4
processes and user data remain untouched.

## Remaining acceptance

The run still has two displayed intervals over 9.375 ms; dropped presents and
coverage holds are zero. Native motion analysis retains all 4,793 excluded wheel
pairs and produces no physical wheel-motion error samples. There is no optical
verification or full-goal completion claim. Further work includes bounded cold
GUI preparation, initialized-app folder entry, full-folder warm wheel/middle
scroll, physical/rendered motion verification and sustained no-spike delivery.

AI assistance has been used to create this output.
