# Proven first-use geometry maps: implemented and tested, not promoted

The 160-Hz objective remains incomplete. This isolated prototype changes only
the first full update of a provably unused dynamic vertex/index buffer. It does
not replace the current completion policy for used or uncertain buffers. No
production source/payload, normal running build, commit or push changed.

## Native contract and proof

The preceding stage capture found an 8.1861-ms DISCARD map on a newly adopted
2-MiB vertex buffer. The prototype tests whether that particular first-use path
can avoid DISCARD. Microsoft's [D3D11 map contract](https://learn.microsoft.com/en-us/windows/win32/api/d3d11/ne-d3d11-d3d11_map)
requires NO_OVERWRITE writes to avoid data used by the input assembler. The
[functional specification](https://microsoft.github.io/DirectX-Specs/d3d/archive/D3D11_3_FunctionalSpec.htm)
distinguishes immediate from deferred-context first-map requirements. This
prototype admits immediate contexts only and establishes unused native lifetime
provenance; it does not infer completion from a CPU timer.

`FirstGeometryMap.h` registers exclusively vertex- or index-bound dynamic,
CPU-write buffers after successful QD3D11Buffer native creation/adoption. A
128-entry fixed table per context holds outstanding first-use proofs without
COM ownership. The first full-update map consumes the proof once and requests
NO_OVERWRITE. Failure falls back to the existing geometry map function; no
second use receives this exemption. Native destruction retires the identity.
Unknown, wrong-thread, unsupported and capacity-exhausted cases fail closed.

Every queued vertex/shader binding, resource update/copy path, draw, indexed
draw, dispatch, external command boundary or native buffer export conservatively
invalidates all outstanding proofs for that context. This avoids per-draw read
epochs. It deliberately gives up opportunities instead of weakening the promise.
Existing geometry/uniform completion guards, admission, shaders, layout and
worker-pool bounds remain unchanged.

## Native export survives recreation

Review found a real edge case before live capture. Qt's native buffer handle
exposes the address of the QD3D11Buffer `buffer` member. An earlier export can
therefore expose a replacement native buffer after `destroy()` / `create()` on
the same QRhi object. Merely invalidating its current native lifetime is unsafe.

The new regression reproduced that error: expected zero fresh proofs after
recreation, actual one (11 passed / one failed). The fixed prototype records
exported **QRhi owner identities** until the actual object destructor, not the
native `destroy()` method. Recreated backing for that owner cannot acquire a
first-use proof. A bounded 4096-entry export table fails closed on overflow;
actual owner destruction removes the export identity and permits address reuse
by a different object. Wrong-thread metadata access permanently poisons that
context's optimization. These lifetime rules do not change Qt's private layout.

## Regression and build evidence

- Disabled first-use baseline reproduces both vertex/index first-map regressions.
- Fourteen focused checks pass, including actual D3D11 staging readback, first
  writes, preservation of a prior queued read during later DISCARD updates,
  actual QRhi full updates, native export, resource-update invalidation, exports
  across recreation, object destruction, unsupported descriptors, capacity and
  wrong-thread rejection. The capacity tests use metadata identities; they are
  not represented as thousands of GPU allocations.
- Current rendered native suite: 48 pass at actual DPR 1.75, zero failure/skip.
- Shader/readback suite: 27 pass, zero failure/skip.
- Embedded import isolation/compiled-host loading: four pass.
- Static Windows dependency audit: pass.

Backend changes reverse exactly to the pinned source. Host and backend receipts
pin actual objects and source hashes. A receipt-generator self-reference was
corrected by excluding the receipt from its own input list; the previous invalid
receipt was retained, tests rerun and host input hashes revalidated. The initial
test compile's const-handle cast error and failing export-lifetime regression
logs are preserved separately. They are not counted as passing evidence.

## Reversed-order live comparison

Same executable with first-use flag on/off, uniform rotation and raw owner memo
disabled. The current 13-mark preparation/node-walk probes remain in both arms.
Actual 3840x2160 / 160 Hz / DPR 1.75, Downloads 191 entries, Masonry density 152,
isolated empty cache per run. Two ten-second boundary-bouncing wheel blocks,
nominal 10-ms interval. No build, offline analysis or UIA observer during input.
All four capture gates pass, including source/build/display/focus/input count,
bounded records and proof feature use. No proof overflow, poisoned context,
wrong-thread access or failed first-use map occurred.

Prefix `real-scroll-first-geometry-map-`, suffix `-20261029-wheel-masonry`:

| Arm | Inputs | Display misses 0 / 1 | Preparation p99 ms 0 / 1 | Preparation max ms 0 / 1 |
| --- | ---: | ---: | ---: | ---: |
| on | 1706 | 4 / 2 | 3.3304 / 3.7029 | 5.3230 / 8.6699 |
| off | 1727 | 7 / 6 | 3.2432 / 3.4292 | 7.9889 / 5.9571 |
| off-r2 | 1722 | 6 / 6 | 3.3902 / 3.7645 | 6.0059 / 10.7280 |
| on-r2 | 1696 | 6 / 4 | 3.3020 / 3.5609 | 15.5873 / 8.4017 |

Enabled runs consume 893 and 942 first-use proofs process-wide; peak outstanding
proof count is one, and no fresh/native-export identity remains at JSON snapshot time.
No `geometry-first-use.MapNoOverwrite` call reaches the 50-us slow-call threshold
inside the gesture windows. That means no recorded first-use map at or above
the threshold, not zero mapping time or an exact per-gesture count of all first
maps. Native-resource activity and the process totals are retained separately.

The controls do not reproduce the preceding 8-ms first-map event. Recorded
geometry DISCARD maxima by block pair/process are 0.3770 and 2.5100 ms in controls;
enabled runs still have used-buffer DISCARD tails up to 0.4640 and 3.8614 ms.
The new exemption intentionally does not bypass those used-buffer guards.

Display-miss counts are lower in the enabled arms, but preparation p99 and maxima
are mixed. Every block fails the unchanged two-physical-pixel maximum motion
gate (enabled maxima 91.64 / 87.37 and 104.79 / 45.93 px). The prototype therefore
does not establish constant 160 Hz and is not promoted from these runs.

Initial captured extents agree within each pair (3480.5714, then 3497.5714 logical
pixels), but source-loading histories are not identical. Final pending requests
are 0 / 6 / 3 / 39. These are cold/loading-plus-reverse-scroll observations,
not verified fully warmed comparisons. No claim of an exact same-residency
causal speedup follows from the miss-count differences.

## Remaining large spans

The enabled reverse-order cold maximum is 15.5873 ms preparation, including
14.4369 ms uploads/admission, with no individually recorded RHI API span reaching
50 us inside that preparation interval. It adds 147 nodes and changes two
geometries. Another enabled reverse block reaches 8.4017 ms spread across alpha
materials (3.0237), uploads (2.2442), before-body work (1.4087) and render lists
(0.8871). The control has a 5.6404-ms texture creation elsewhere in preparation.

The next attribution must separate merged geometry generation/copy, per-batch
upload work and scheduler waits within the **same** large upload span. Absence
of a long instrumented RHI call does not prove the complete 14.4369 ms is CPU
copying. Do not substitute aggregate isNodeBlocked samples or the earlier map
stall for this frame's cause. Preserve atomic vector readiness, camera motion,
source priority, loading behavior and pixel fidelity while testing any change.
Verified warm and quarter-height middle-scroll acceptance remains outstanding.

After capture only the exact first-geometry diagnostic launcher tree was stopped.
Normal launcher PID 43936 and extracted host PID 119460 remain alive. Sources,
tests, build receipts and measured results are snapshotted; executable/object
outputs and large raw traces remain hashed external artifacts. No claim of
portable production rollout or untested platform verification is made.
