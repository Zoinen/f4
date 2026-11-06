# Direct atlas sampling exposes a 22 ms runtime shader-compilation stall

Direct atlas sampling eliminates duplicate extraction allocations, but its
source-HLSL package introduces a measured cold compilation stall. Baking DXBC
ahead of time removes that call in two controlled runs and reduces the maximum
cold render envelope from about 26.7 ms to 10.5–15.9 ms. Other stalls survive;
no shader or backend candidate is promoted and constant 160 Hz remains unmet.

## 1. Repeat the atlas-aware comparison under current backend policies

The previously rejected shader is tested without editing its source. Shader
and resource hashes still match the earlier fixture. Both arms now retain the
newer guarded geometry reuse, bounded staging uploads, uniform capacity
retention and initial-dynamic geometry. Growth headroom and source-image
retention are disabled. The texture-factory observer remains enabled.

One executable selects the shader package. A relinked fixture uses the host's
actual Quick, Gui and Qml libraries, the same overridden renderer/RHI/texture
objects, and the existing pixel-test source. All 27 checks pass with zero pixel
differences, including plain/atlas, alpha/checkerboard, rounded edges,
up/downsampling, neighbor bleed, source replacement and actual 175% scene
origin/unit transforms. Rendered output was inspected. Four import checks pass
in each mode and the system-DLL audit passes. Duplicate static plugin-init
objects produce LNK4042 warnings; these are not runtime failures and were not
hidden by dropping required initializer objects.

Downloads, 190 entries, Masonry density 152, actual 160 Hz / DPR 1.75. Separate
fresh processes and initially empty application caches, enabled/control/
control/enabled, two ten-second rapid-wheel blocks each. Standard media workers
and hardware decoding remain active. OS cache is not flushed. Quiet 45-second
helpers retain output until completion; no builds, offline analysis or UI
Automation observer run during gestures. Focus, input count, target PID,
recorder capacity, feature-use and zero reuse/upload-error checks pass.

| Shader / Qt PID | Cold / next missed refreshes | Cold / next largest display gap | Last cache entries |
| --- | --- | --- | ---: |
| Atlas-aware / 89604 | 5 / 0 | 31.2590 / 6.5301 ms | 195 |
| Original / 37996 | 1 / 1 | 12.5363 / 12.4818 ms | 188 |
| Original repeat / 110160 | 10 / 4 | 12.5471 / 12.5256 ms | 197 |
| Atlas-aware repeat / 50512 | 10 / 2 | 25.0214 / 18.7728 ms | 201 |

Atlas-aware runs have zero extraction producers, versus 120 and 114 atlas-copy
creations in the controls. The candidate's largest individual cold texture
creation is below 0.5 ms, yet preparation reaches 25.3178 ms in the first run.
Its largest recorded RHI call is only 4.3422 ms. This discrepancy motivates
timing an unobserved operation rather than declaring allocation removal a win.

Preparation / recording p99, cold then next block, milliseconds:

| Shader | Preparation p99 | Recording p99 |
| --- | --- | --- |
| Atlas-aware | 3.1038 / 1.9833 | 1.3335 / 0.7747 |
| Original | 2.7561 / 2.6353 | 1.2938 / 0.9796 |
| Original repeat | 4.3465 / 2.4090 | 1.3528 / 0.9384 |
| Atlas-aware repeat | 3.6221 / 2.6019 | 1.9430 / 1.0873 |

Unlike the earlier eight-block result, these values are mixed. This uses rapid
wheel input, not that earlier quarter-height middle-button scenario. It neither
reproduces every prior condition nor invalidates the prior warm regressions.
The phase names are wall-time envelopes; preparation is not necessarily pure
CPU batch construction.

## 2. Measure the missing operation

Qt's D3D11 backend prefers DXBC when present, otherwise obtains HLSL from the
QShader package and calls D3DCompile. The COM-call observer did not cover that
plain function. An observation-only patch brackets the actual call by stage,
preserving every argument/result and reversing exactly to the prior source.
The same 27 pixel checks, four import checks and DLL audit pass.

One fresh atlas-aware capture, PID 126600, records exactly one compiler call
above the 50 us recording threshold: fragment compilation, 22.3854 ms. Exact
same-thread containment places the call inside a 25.2130 ms preparation span
and 26.4153 ms render envelope during cold input. This is direct attribution
for that frame, not a guess from the shader's first use. It does not explain
all earlier misses or repeated warm regressions.

The checked existing application shader package also contains HLSL 50 rather
than DXBC. Its compilation/cache timing was not separately measured here;
the 22 ms finding belongs to the diagnostic atlas-aware package and must not
be represented as a measured production stall of the same size.

## 3. Precompile the same shader, then isolate that change

Qt 6.11.1's installed qsb supports --fxc. With the installed Windows SDK fxc,
the identical atlas-aware fragment source is baked with the same GLSL 150,
GLSL 300 es and MSL 12 targets, replacing its HLSL entry with DXBC 50. Its dump
confirms all five target entries and reflection metadata. No shader arithmetic,
geometry or sampling semantics are edited. The other five embedded resources
retain their verified bytes. The embedded override verifies all six hashes
before use. fxc is a build-time tool, not a shipped runtime dependency.

The pixel fixture is rebuilt to compare the original shader against the DXBC
package using the current backend. All 27 checks pass with maximum channel
difference zero; rendered output is inspected. Four import checks pass with
precompiled selection disabled and four with it enabled; the DLL audit passes.
This is an artifact-only resource package, not an embedded portable rollout.

One executable now compares source-HLSL atlas sampling with DXBC atlas
sampling. Both retain the same backend/observer policies. The capture asserts
zero extraction producers in both. Source runs record runtime compilation;
DXBC runs record none at the 50 us threshold. The DXBC-only package and Qt's
verified selection branch additionally establish that this fragment does not
fall back to HLSL compilation; sub-threshold calls elsewhere are not counted.

| Package / Qt PID | Cold fragment compile | Cold max render envelope | Cold / next misses | Cold / next max display gap |
| --- | ---: | ---: | --- | --- |
| DXBC / 59700 | No recorded call | 10.4777 ms | 12 / 0 | 18.7673 / 7.0661 ms |
| HLSL / 114652 | 23.2839 ms | 26.7621 ms | 4 / 3 | 24.9985 / 12.5030 ms |
| HLSL repeat / 86252 | 22.6550 ms | 26.6430 ms | 3 / 0 | 24.9950 / 6.4956 ms |
| DXBC repeat / 62776 | No recorded call | 15.9014 ms | 9 / 1 | 18.7225 / 12.4935 ms |

Both HLSL calls lie wholly inside cold preparation (25.9693 and 25.5101 ms).
The isolated compilation stall is removed; whole-scene cadence is not better
by every metric. Both DXBC cold runs have more misses than these HLSL runs.
The strict motion requirement fails in every capture; even DXBC's next block
with zero missed refreshes reaches a 14.8230-physical-pixel step error. Zero
coverage holds does not make a failed motion gate pass. Display misses use
the existing >9.375 ms threshold, not a relaxed new success definition.

Last cache entries are 202 / 191 / 190 / 201. These are stored cache snapshots,
not unique-file throughput or latency. End private memory is 1200.2 / 1147.6 /
1124.5 / 1124.8 MiB, and process peak pagefile is 2024.1 / 1855.0 / 1851.5 /
2079.5 MiB. These mixed whole-process values do not isolate shader memory or
prove no memory cost. Samples are taken after input, not polled during it.
Two ordered pairs and variable loading progress are not statistical proof of
general frame-rate improvement. Presentation-derived motion is not scanout.

## Remaining work

The precompiled runs still show constant-buffer host-write Map at 5.8417 and
4.5348 ms, and geometry DISCARD Map at 5.4060 and 4.1770 ms when completion is
unproven. The safety guard must not be relaxed just to remove those samples.
These are independent of the now-absent extraction allocations and compiler
call. Scheduling, submission and scene work also remain to be attributed.

Keep precompilation as a validated way to remove the measured compilation
stall, but do not promote the atlas shader or combined backend solely on this
result. A product build change needs normal-source packaging, portable gates,
default-workload validation and full cold/warm wheel/middle-button checks.
The earlier atlas-aware warm regression remains unresolved. Constant 160 Hz
has not been achieved.

The matching snapshot contains all three experiment sources, build/compatibility
identity, corrected fixture link libraries, shaders/resources, native/import/DLL
logs, rendered output, nine capture packets, target PresentMon rows, memory/method
sidecars and analyses with hashes. Diagnostic process trees are stopped. The
normal portable launcher and its Qt child remain alive and unchanged. No
embedded-payload replacement, commit or push.

AI assistance has been used to create this output.
