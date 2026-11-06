# Fresh uniform backing passes correctness checks; live validation remains blocked

A prototype substitutes a prepared fresh constant buffer when the latest frame
still reads the old backing. It avoids writing into that old resource and
increments the QRhi buffer generation so shader-resource bindings rebuild.
The old native reference is retained until a real completed GPU frame covers
the current command stream. The prototype is unpromoted: no accepted scrolling
capture or latency improvement has been established for it.

## Eligibility and lifetime

Rotation occurs only inside the uniform-binding path, with existing safe reuse
enabled, supported constant-buffer NO_OVERWRITE, a pending latest-frame proof,
lastRead equal to that frame, no unknown/external read epoch, and an available
fresh prepared buffer. Current-frame queued reads, exported native handles,
registry overflow, unknown/skipped proof, unsupported descriptors/threads and
reserve misses retain fallback behavior. The D3D11 capability-query bypass
from the preceding experiment prevents QSG's slot-count test from being
mistaken for a real native-handle export.

Native-handle exports are tracked by QRhi buffer identity until ordinary
resource destruction/recreation. At most 4096 export identities are retained;
overflow conservatively disables rotation for that context. Ownership of an
old native buffer reference transfers to retirement on substitution. Its
retirement epoch covers the next completed frame, preserving earlier queued
binding snapshots even when no draw has yet referenced them. Buffer generation
changes drive the existing binding-cache rebuild; CPU shadow size, update
length and logical QRhi size do not change.

Retirement is bounded to 256 references and 8 MiB per device. The separate fresh
reserve remains about 24.25 MiB per eligible device; these are separate bounds,
not a combined 32 MiB process bound. Collection uses real completion proofs
already obtained by mapping decisions. Idle retained resources can persist
until later mapping activity or device destruction, but remain bounded.
Retired Release calls receive their own timing label so a later benchmark can
detect stalls moved into destruction. No extra blocking GPU wait was added.

## Verification

- Seven safety checks pass, including setup/cleanup. Real old/new GPU copy
  readbacks preserve old bytes after backing substitution. Policy checks cover
  current reads, unknown proof, native exports, wrong threads, unavailable
  reserve, byte/reference limits and export-registry overflow. Bound-policy
  fixtures model occupied retirement references with balanced real COM AddRefs;
  they are not measurements of hundreds of distinct GPU allocations.
- The pending-frame binding regression performs 24 substitutions, observes
  native-pointer/generation changes, and verifies all twelve alternating color
  updates and scene transforms. Its peak retired capacity is 256 bytes. The
  previous implementation fails the required substitution assertion.
- A test-only compile seam conservatively treats an otherwise completed latest
  frame as pending. It does not fabricate completion. Separate object builds
  are used, and dumpbin confirms forcePendingForTests is absent from the
  production object and present only in the test object. These forced-path
  results prove rendering behavior, not overlap timing or performance.
- All 48 production-object native checks and 27 rendered shader checks pass
  at actual, unforced DPR 1.75 after 160 Hz was restored. Import isolation passes
  four checks and the portable Windows dependency audit passes.
- An intermediate forced-scale run failed because native scaling changed back
  to 175%, making the old multiplier produce DPR 2.45. That failed log remains;
  subsequent tests explicitly remove the multiplier. Rendered output was
  visually inspected. No UI layout was changed.

The new host is linked with the production object, not the force-pending test
object. Display/geometry/DPI-change signals are recorded by the diagnostic
host; captures reject events within measured input windows. A short Qt display
probe rejects unsuitable setups before starting timed input. Both capture arms
would keep fresh-buffer preparation and the slot-query bypass enabled, differing
only in whether backing substitution is permitted. No such pair is accepted yet.

## Storage recovery

The first test-object compilation stopped when D: had zero free bytes. Recovery
removed 17 inactive Go diagnostic launcher copies only after exact artifact
directory, inactive-path and SHA256 equality checks against the preserved normal
executable. It recovered 2,790,067,200 bytes. Native hosts, objects and traces
were retained. The partial compiler log, removal manifest and subsequent
successful receipts are preserved.

## Display recovery and rejected capture attempts

Qt initially reported the MSI MAG322UPF at DPR 1.75 / 59.997 Hz. Windows exposed
the original 3840x2160x32, 160 Hz mode and CDS_TEST accepted it. A session-only
mode change (without registry update) succeeded, and a new Qt process reported
160 Hz / DPR 1.75. Native correctness checks were run in that state.

Before the first timed attempt, the display changed to 2560x1600 / about 60 Hz;
the preflight rejected it before cache creation, launcher startup or PresentMon
input collection. No valid timing packet was produced.

A single bounded retry held ES_SYSTEM_REQUIRED|ES_DISPLAY_REQUIRED during the
capture session, applied the validated 3840x2160 / 160 Hz mode and checked it
after settling. The mode stayed at 160 Hz, but Qt reported DPR 2.25. Preflight
again rejected the run before timing began. The wrapper restored the preceding
2560x1600 / 60 Hz mode and its prior execution-state flags. No saved display or
power-plan setting was changed. The cause of the configuration changes is not
established by these observations.

[Microsoft display-mode API](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-changedisplaysettingsexw),
[execution-state API](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-setthreadexecutionstate).

The remaining required action is stable representative capture at
3840x2160, 160 Hz and unforced DPR 1.75, followed by cadence, motion, loading,
memory and fidelity comparison. The goal service reports the original goal
blocked; the full constant-160-Hz objective is unchanged and incomplete. The
existing display-restoration request remains pending. No conclusion is drawn
from synthetic refresh values or rejected captures.

The normal app remains Go PID 43936 / extracted host PID 119460. No normal-app
replacement, embedded payload rollout, commit or push occurred. Only diagnostic
artifacts and reports changed. The matching snapshot retains source, build and
symbol receipts, native/binding safety results, display-state transitions and
rejected preflight evidence, with external binary/object hashes.
