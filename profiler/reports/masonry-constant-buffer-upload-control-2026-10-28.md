# Constant buffers: whole-context reuse is rarely eligible; DEFAULT uploads still stall

The device supports no-overwrite mapping of dynamic constant buffers, but the
current whole-context proof would allow only 103 of 681,657 main-context updates
(0.015%). Replacing maps with synchronized uploads to DEFAULT native constant
buffers passes correctness tests but moves tails into UpdateSubresource, up to
7.7257 ms. Neither approach is promoted. Constant 160 Hz remains incomplete.

## Eligibility observation, without changing maps

The source-only observer queries D3D11_FEATURE_D3D11_OPTIONS once per device.
All four recorded devices return success and support both constant-buffer
offsetting and MapNoOverwriteOnDynamicConstantBuffer. This is a queried driver
capability, not an inference from the Windows or Direct3D version. Microsoft
documents the required capability and the caller's data-lifetime obligation in
[the feature structure](https://learn.microsoft.com/en-us/windows/win32/api/d3d11/ns-d3d11-d3d11_feature_data_d3d11_options)
and [dynamic-resource guidance](https://learn.microsoft.com/en-us/windows/win32/direct3d11/how-to--use-dynamic-resources).

The host retains precompiled direct-atlas sampling and the previously tested
backend policies. Every observed host-write map still uses WRITE_DISCARD.
Before constant-buffer mapping, the observer polls the existing event query
without flushing, records prior-frame completion and intervening work, then
times the original map. It does not relax the geometry guard or infer which
resource the intervening work accesses. Shared recorder layouts are unchanged
and the source transformation reverses exactly.

Fresh capture PID 55408, main context 2dcc0fc8, cumulative recorded snapshot:

| Counter | Updates |
| --- | ---: |
| Host-write maps, all constant buffers | 681657 |
| Prior frame proven complete | 678151 |
| Intervening work since that marker | 681554 |
| Complete prior frame and no intervening work | 103 |
| Prior-frame completion unproven | 3506 |

The counts overlap and are not confined to the input windows. A complete prior
frame alone does not prove a constant buffer is unused by newer work. Cold
input has 52 maps above the 50 us recording threshold, only one strictly
eligible; the next block has 11 and zero eligible. The largest observed map in
this particular run is 0.7364 ms, with prior completion but 176 intervening
operations. It does not reproduce the preceding runs' 4–6 ms tail. Extra
feature/proof polling has observer cost; this is eligibility evidence, not a
performance improvement or a disabled-observer comparison.

The observation-only host passes the existing 27 shader pixel checks, four
import checks and the DLL audit. Those are not tests of a no-overwrite policy,
because no such policy is enabled for constants here.

## Alternative tested: native DEFAULT buffers plus full uploads

A separate same-executable experiment preserves QRhi's Dynamic UniformBuffer
API but creates native DEFAULT constant buffers without CPU map flags. Deferred
host writes use full UpdateSubresource with a null destination box. Direct
begin/end updates write the CPU shadow and mark it pending for the same flush
path. Vertex/index mapping and other native buffer types retain their policy.
The diagnostic flag selects the old path in the same executable for controls.

The CPU shadow allocates and initializes the entire rounded native capacity,
not just the logical request, because full constant-buffer uploads read all
native bytes. Logical update/binding lengths remain unchanged. This handles
non-aligned requests such as 68 and 1028 bytes without an out-of-bounds source
read. It also avoids depending on the no-overwrite feature or completion proof.
The runtime upload API provides synchronization; its cost may be higher under
contention, as documented in
[UpdateSubresource](https://learn.microsoft.com/en-us/windows/win32/api/d3d11/nf-d3d11-id3d11devicecontext-updatesubresource).

Tests against the current backend, zero failures/skips:

- 11 buffer checks with the candidate enabled, and 11 with it disabled:
  logical/native usage, flags, 68/256/1028/65536-byte buffers, batch/direct
  updates, queued GPU copies preserving the earlier contents, initialized
  native padding, partial-update preservation and unchanged dynamic geometry.
- 47 native rendering checks at actual DPR 1.75, including buffer identities,
  scene origins/unit transforms and rendered pixels.
- 27 shader pixel checks with maximum channel difference zero.
- Four import checks in the original mode, four with the combined candidate
  selected, plus the Windows system-DLL audit.

Setup/cleanup contribute to test totals. This does not verify every native
interop consumer or constitute a portable rollout. The normal binary and its
embedded payload are unchanged.

## Four live captures

Actual Downloads, 190 entries, Masonry density 152, actual 160 Hz / DPR 1.75.
Enabled/control/control/enabled, separate fresh processes and initially empty
application caches, two ten-second rapid-wheel blocks each. Precompiled atlas
sampling and all other backend policies/observers match. Standard media workers
and hardware decoding stay enabled; OS cache is not flushed. Quiet 45-second
helpers retain output until collection ends. Focus, input count, target PID,
recorder capacity, feature-use and zero reuse/upload-error checks pass. There
are no extraction producers or recorded runtime shader compilations. Builds,
offline analysis and UI Automation are absent during input. Memory sampling
occurs after collection.

| Arm / Qt PID | Cold / next missed refreshes | Cold / next max display gap | Last cache entries |
| --- | --- | --- | ---: |
| DEFAULT upload / 109688 | 7 / 1 | 13.3958 / 18.7080 ms | 203 |
| Dynamic map / 112668 | 4 / 1 | 12.5060 / 12.5049 ms | 189 |
| Dynamic map repeat / 92500 | 4 / 1 | 18.7579 / 12.4981 ms | 206 |
| DEFAULT upload repeat / 109868 | 4 / 2 | 12.5348 / 12.4693 ms | 209 |

Misses use >9.375 ms. Coverage holds are zero, but every capture fails the
complete no-spike/motion requirement. The first enabled next block has maximum
step error below two pixels but still misses a refresh; that is not success.
Other maximum step errors reach 138.8882 physical pixels. Presentation-derived
motion is not physical scanout. Loading progress differs; cache counts are
stored-entry snapshots, not unique-file throughput or per-file latency.

All 443 / 462 recorded native constant-buffer creations in enabled processes
have DEFAULT usage; all 441 / 455 in controls have DYNAMIC usage. Enabled
snapshots contain 696,021 / 670,374 full uploads and no legacy constant-buffer
Map records; controls record zero new-path uploads. The direct API path is
exercised by unit tests, not by these scene snapshots (direct counters are zero).

| Operation | First candidate cold max | Repeat candidate cold max |
| --- | ---: | ---: |
| DEFAULT constant-buffer upload | 5.3335 ms | 7.7257 ms |
| Legacy constant-buffer Map | No records | No records |

Corresponding control Map maxima are 1.6743 and 1.1517 ms in these runs. A
slower map from an older capture is not substituted for these controls. Buffer
creation separately reaches 8.4374 / 12.5406 / 9.5683 ms in the first three
arms; Present reaches 13.3229 ms in the reverse control. Removing one API from
the sample list does not remove the driver/resource stall.

End private memory is 1029.8 / 1060.6 / 1177.3 / 1204.2 MiB; process peak
pagefile is 1790.4 / 1736.0 / 1843.9 / 2145.1 MiB. These mixed whole-process
samples do not isolate constant-buffer memory or prove a memory benefit. Two
ordered pairs are not a statistical improvement claim. Keep the dynamic-map
policy unchanged.

## Next action and workspace state

Do not drop the intervening-work check. Useful no-overwrite reuse would need
proof about each buffer's actual reads, copies, lifetime and unknown external
access, plus bounded tracking overhead and GPU-ordering tests. The queried
hardware support makes that an available direction; it does not supply the
missing synchronization proof. Whole-scene scheduling/submission and other
independent allocations remain in scope, as do cold/warm wheel, quarter-height
middle-button validation and eventual portable rollout.

To reclaim diagnostic space, 14 inactive Go launchers were removed only after
their full path was verified under artifacts, no running process used them,
and their SHA256 exactly matched the unchanged normal launcher. They were
duplicates recoverable by copying that launcher; native hosts, sources,
measurements and snapshots were retained. The cleanup manifest records all
paths/hashes and 2,297,702,400 bytes reclaimed.

The matching experiment snapshot retains both probes, sources, tests and logs,
build identities, five capture packets, target PresentMon rows, memory/method
sidecars and analyses with hashes. Diagnostic process trees are stopped. The
normal launcher PID 43936 and Qt PID 119460 remain alive; launcher hash stays
5023071d267153cc2bb1ee1a7ac7119f47a6849fd586c9de93a968eac97bdc31.
No commit, push or embedded-payload replacement. The goal remains active.

AI assistance has been used to create this output.
