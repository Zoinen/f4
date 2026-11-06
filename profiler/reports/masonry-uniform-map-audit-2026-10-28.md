# Uniform-map audit narrows the remaining pending-frame problem

The eight saved 160 Hz / DPR 1.75 captures contain 129 recorded slow rejected
uniform maps inside gesture windows. Of these, 125 last referenced exactly
the latest submitted frame while its completion query was pending. Only two
refer to never-read resources and two to resources older than that frame.
This does not support treating an older completion watermark as the main fix
for these measured map tails. The largest latest-frame rejection is 3.2019 ms.

These are records at or above the 50 us threshold from the prior range/full
geometry experiments, not a census of all map decisions and not new captures
of the constant-buffer candidate. Analysis filters the original owner render
thread and complete gesture windows. The 125 latest-frame cases all have
reason 3, query status 2, lastRead == taggedFrame; none of them has a newer
queued-read epoch. No guard was weakened on this evidence.

## Source checks and a capability-query candidate

Qt's batch renderer calls nativeBuffer().slotCount twice in its uniform-update
paths, testing whether a direct pointer update is needed. The local D3D11
implementation of nativeBuffer() always returns one native slot; it also calls
executeBufferHostWrites for dynamic buffers. Thus the D3D11 capability test
does not need to export a native handle or potentially flush pending writes.

An isolated opt-in renderer candidate bypasses only that D3D11 capability
query. Other backends keep the old test. A counter in the D3D11 nativeBuffer
implementation observes actual uniform-handle exports. The regression changes
a colored rectangle over twelve updates with frame grabs, verifies pixels,
scene origin (14,14) physical pixels, unit transforms, and saves rendered output.

| Regression | Native uniform exports | Completed frames | Uniform map decisions | Result |
| --- | ---: | ---: | ---: | --- |
| Original renderer | 25 | 25 | 25 | Expected no-export assertion fails |
| Candidate renderer | 0 | 25 | 25 | Pass |

The full native suite passes 48 checks, zero failures/skips. The image was
visually inspected. This proves redundant handle-query removal in the fixture;
it does not prove fewer maps, lower latency, or 160 Hz improvement. Map count
is unchanged in this workload. The getter change is not promoted or integrated
into a portable host; only test executables and source artifacts were built.

The implementation and counter are in artifacts/uniform-native-query-20261028,
with a base receipt referencing the current constant-buffer preparation
candidate. The new tests do not request its async-reserve feature. The normal
application and its embedded host remain unchanged.

## Avoided incorrect fix

The buffer-readback path was checked before adding extra read epochs. Dynamic
buffer readbacks copy the CPU shadow immediately; the queued GPU CopySubRes
branch applies to non-dynamic buffers. The uniform read tracker only registers
dynamic constant buffers. Consequently, a supposed missing queued GPU readback
touch for these tracked buffers is not a reproduced bug, and no such fix was
added. Native/external access contracts still require care in any future
backing-resource substitution.

## Display and next verification

Windows still reports 59 Hz and unforced Qt tests reported DPR 1.25 in the
preceding turn. This suite uses QT_SCALE_FACTOR=1.4 to obtain a QWindow DPR of
1.75 for controlled native rendering tests. It is not evidence of a restored
160 Hz / 175% desktop. The pending request to restore that measurement setup
was not repeated; no display settings were changed. No new cadence capture
or performance claim is made from the forced-DPR check.

Fresh native backing for a still-in-flight constant buffer is a remaining
candidate direction. Before implementation, it must account for queued binding
snapshots, generation-driven resource-binding rebuilds, old native resource
lifetime, actual native-handle exports, and bounded retirement/fallback. The
capability-query result does not itself implement or validate that mechanism.
The original constant-160-Hz goal remains active and incomplete.

The matching snapshot preserves the eight-case rejection analysis and exact
source/test receipts, while referencing the already retained original capture
snapshots. No commits, pushes, payload replacement or normal-app restart.
