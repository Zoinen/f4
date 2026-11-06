# Per-buffer proof enables constant-buffer reuse; scene cadence remains incomplete

A bounded per-buffer read tracker makes 99.56% of observed main-context updates
eligible for no-overwrite mapping, compared with 0.015% under the whole-context
guard. The guarded prototype passes safety/rendering tests and performs over
660,000 reuse maps per enabled process without failure. Admitted maps remain
below 0.32 ms in both captures. Unproven cases retain DISCARD and still reach
2.7 ms. Whole-scene results are mixed, so this remains unpromoted and constant
160 Hz is not achieved.

## Read/lifetime model

The observer starts from precompiled direct-atlas sampling and the current
diagnostic backend policies. The rejected DEFAULT constant-buffer upload
experiment is disabled. It records queued QRhi draws/indexed draws/dispatches
using their current shader-resource bindings, before native execution. Tracking
only native execution would miss earlier queued reads and could allow an unsafe
overwrite. Uniform binding references are resolved to their native buffer.

Native constant-buffer creation registers a fresh lifetime; destruction removes
it before release. The per-device registry retains at most 4096 dynamic,
constant-only buffers and owns no extra COM references. Unknown/untracked or
overflowed resources are not eligible. Context-owner thread checks reject
cross-thread tracking. Device records are bounded at 32; untracked devices
fall back as well. Copies mark both tracked endpoints conservatively, and
beginExternal records an unknown-use epoch that blocks all reuse until a later
completion proof covers it. No static UI layout or image fidelity changes.

The proof requires queried driver support, the known owning context/thread,
an active tracked buffer, successful nonflushing completion of the existing
latest event marker, and both the buffer's last queued-read epoch and the
unknown-use epoch at or before that completed marker. Intervening work is not
ignored generally: only a proof that it has not referenced this buffer makes
the difference. This assumes native interop follows QRhi's external-access
contract; arbitrary unannounced native-context use is not proven safe.

An observation-only build still maps every buffer with DISCARD. Its source
transformation reverses exactly to the previous RHI source. A compiler pointer
typo was corrected before any accepted build/capture; that error log is retained.
The shared RHI recorder layouts remain unchanged.

## Observation and tracking cost

Fresh PID 30400, main context 2b44e118, entire recorded snapshot:

| Counter | Value |
| --- | ---: |
| Map decisions | 663043 |
| Eligible | 660157 (99.565%) |
| Completion unproven | 2886 |
| Same-frame read / unknown / untracked | 0 / 0 / 0 |
| Queued draw/dispatch references | 868158 |
| Created / retired / peak live constants | 389 / 82 / 307 |
| Registry overflow / wrong-thread use | 0 / 0 |

Cold input contains 21 map samples above 50 us, 20 eligible. The largest,
5.5721 ms, has last-read epoch 377 and completed marker 377, despite 245
intervening operations unrelated to that buffer. The next 3.5817 ms sample
also qualifies. All 19 slow samples in the next block qualify. The three
offscreen video contexts remain unproven and receive no eligibility.

Recorded observer wall time is 293.7982 ms for map decisions and 38.3436 ms
for queued-reference hooks, about 0.1024 ms per recorded main-context frame.
Maximum individual observed hooks are 0.1662 / 0.0940 ms. These are instrumented
wall-time aggregates, not a disabled-observer comparison, CPU-cycle proof or
universal overhead bound. Counts include startup/inter-block activity, not
only measured input. No external access is encountered in the live scene;
fallback behavior is exercised in tests rather than inferred from that absence.

## Guarded reuse prototype and tests

A separate wrapper selects WRITE_NO_OVERWRITE only when the proof succeeds
and the diagnostic flag is enabled. Every rejected case keeps WRITE_DISCARD.
A failed admitted map retries DISCARD. The same executable disables admission
for the control while retaining identical tracking. Direct-update methods and
non-uniform buffers retain their previous paths. This is not a replacement of
Qt's public API or a new setting.

Validation, zero failures/skips:

- Six tracking checks: real completion queries, queued reads of A excluding A
  while leaving B eligible, copies, external access, lifetime removal,
  non-uniform rejection, wrong-thread rejection and the 4096-entry bound.
- Four safety checks in each enabled/control process: real byte writes and GPU
  copies preserve A's old contents while B is reused; same-frame A falls back;
  external, unsupported, untracked and unproven cases fall back.
- 47 native rendering checks at actual DPR 1.75 and 27 shader pixel checks
  with maximum channel difference zero. The fixtures were strengthened and
  rerun to explicitly assert reuse occurred and no reuse map failed. The
  shader fixture records 25 admitted and 75 DISCARD maps. The native fixture
  asserts positive admission, although its installed message handler suppresses
  the informational count line.
- Four compiled-host/import checks with admission disabled, four enabled,
  and the system-DLL audit.

Test totals include setup/cleanup and are not that many independent scenarios.
The rare native-failure retry is implemented but not forced through a synthetic
driver error; all tested/recorded admitted maps succeed. Correctness checks do
not substitute for a complete review of every native client or portable rollout.
Adding explicit render-fixture counters after captures relinks only test
executables; the captured host/source/policy is unchanged.

## Controlled live results

Actual Downloads, 190 entries, Masonry density 152, actual 160 Hz / DPR 1.75.
Enabled/control/control/enabled; separate fresh processes with initially empty
application caches, two ten-second rapid-wheel blocks per arm. Standard media
workers/hardware decoding stay enabled. OS cache is not flushed. Quiet
45-second helpers retain output until completion. Focus, input count, target
PID, recorder capacities, feature-use and zero reuse/upload-error checks pass.
There are no extraction producers or recorded runtime HLSL compilations. No
builds, offline analysis or UI Automation observer run during gestures. Memory
is sampled afterward, not polled during input.

| Arm / Qt PID | Admitted / DISCARD, whole snapshot | Cold / next missed refreshes | Cold / next max display gap | Last cache entries |
| --- | --- | --- | --- | ---: |
| Enabled / 56316 | 674133 / 2710 | 3 / 1 | 14.0021 / 12.5031 ms | 214 |
| Disabled / 106668 | 0 / 676936 | 5 / 1 | 18.7519 / 12.4762 ms | 198 |
| Disabled repeat / 82684 | 0 / 677676 | 2 / 1 | 24.9575 / 12.5024 ms | 194 |
| Enabled repeat / 126432 | 663003 / 2762 | 10 / 8 | 12.5636 / 18.7729 ms | 213 |

Misses use >9.375 ms; coverage holds are zero. Every capture still fails the
complete no-spike/motion requirement, with maximum presentation-derived errors
from 10.4259 to 253.5566 physical pixels. These metrics are not physical scanout
proof. Loading progress differs; cache counts are stored-entry snapshots, not
unique-file throughput or latency. Two ordered pairs do not establish general
cadence improvement. In particular, the repeated enabled run is worse by miss
count and must not be hidden by selecting the first pair.

| Enabled inner API | Cold max, first / repeat | Next max, first / repeat |
| --- | --- | --- |
| Admitted constant NO_OVERWRITE | 0.1605 / 0.2863 ms | 0.1138 / 0.3115 ms |
| Rejected constant DISCARD | 1.8982 / 2.6689 ms | 0.0840 / 2.6946 ms |

These are the actual inner map types. The old outer wrapper's historical API
name/map-type field is not used to infer admission. Control cold DISCARD maxima
are 0.4019 / 0.8374 ms in these runs; the preceding observation's 5.6 ms value
is not substituted for a matched control. Buffer creation separately reaches
7.1354 ms and Present 6.8068 ms in the enabled repeat. Safe cases are fast,
but other delays remain and the admission guard is not relaxed to hide them.

End private memory is 1189.5 / 1146.7 / 1172.4 / 1178.9 MiB; peak pagefile
is 2081.6 / 1881.2 / 1920.1 / 2128.3 MiB. Whole-process samples do not isolate
tracker, buffer or GPU-memory cost. The tracker is bounded, but that does not
bound all active scene resources or driver allocations.

## Remaining attribution

PresentMon's reported GPU-active p99 is around 6 ms in all arms. Completion
and display tails also remain. These are presentation/timeline fields, not
isolated shader execution measurements; the next investigation should validate
them against native GPU timing or scheduling before assigning the residual
misses to a shader, other device work or the compositor. CPU map timing alone
does not explain the full outcome. Unproven buffers, allocation lifetime,
submission, cold/warm wheel and quarter-height middle scrolling remain in
scope. The normal application still uses its previous validated build.

Sources, bounded-tracking/safety tests, build identity, test/import/DLL logs,
the observation plus four comparison packets, target PresentMon rows,
memory/method sidecars and analyses are frozen in the matching snapshot with
hashes. Diagnostic process trees are stopped. Normal launcher PID 43936 and
Qt PID 119460 remain alive and unchanged. No commit, push or embedded-payload
replacement. The goal remains active.

AI assistance has been used to create this output.
