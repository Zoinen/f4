# Uniform capacity retention removes reallocations, but not scrolling spikes

A D3D11-only diagnostic change retains uniform-buffer capacity that the backend
already allocates. A native regression fails before and passes after the change.
Two ordered live pairs show substantially fewer uniform creations, but no
consistent presentation benefit. The change remains experimental; production
and its embedded payload are unchanged. Constant 160 Hz remains unachieved.

## Regression before implementation

The fixture renders a real red item, then changes its material's NoBatching
flag to switch merged/unmerged batching. It checks the same logical QRhi object,
same underlying native buffer, unchanged count of actual D3D constant-buffer
creations, and identical rendered images. It checks actual D3D11, DPR 1.75,
the leaf's physical origin (14,14), unit transforms and rendered red pixels.
A control starts unmerged and already has sufficient capacity.

Before the patch: three passes / one failure including setup/cleanup. The
transition fails with logical size **80 -> 256** and native identity
**4b79b4a0 -> 4b7af320**. The already-unmerged control passes. The original
failure was reproduced before writing the renderer capacity change.

After: all **46 native checks pass**, zero failures/skips, including both new
rows and the existing 44 checks. The transition retains its resource and no
new constant buffer is created. Import-isolation/compiled-host checks report
four passes; Windows system-DLL audit passes. These are diagnostic checks,
not proof of a completed portable rollout or every other backend/platform.

## Intervention

Only batch-renderer uniform allocation/growth changes. When enabled on D3D11,
requested capacity is rounded to the 256 bytes already used by the backend's
native allocation. Shader update ranges and binding ranges keep their original
sizes. The rule runs only when allocating/growing a uniform buffer; it adds
no per-frame pixel alignment or UI layout work. Other backends retain the old
policy. Source replacements restore exactly to the current production renderer.

Keeping logical/shadow capacity consistent matters: changing RHI's create
function to skip reallocation alone could overflow its smaller CPU shadow.
This candidate allocates the shadow at the retained size from the start.
Each reservation adds less than 256 bytes of CPU shadow padding compared with
the exact requested logical size. Per-request native GPU capacity is unchanged
relative to D3D's existing rounding. This is not a measured peak-memory claim;
there is no new separate pool, retirement budget or unbounded resource list.

One executable contains enabled/disabled paths. It uses default four decode
workers, hardware video decoding, all media types, original atlas behavior and
the existing RHI call/resource/producer probe. It does not include the previous
video-lane or video-cache experiments, nor the high-frequency batch observer.

## Live on/off/off/on sequence

Actual Downloads, 190 entries, Masonry density 152, actual 160 Hz / DPR 1.75.
Each fresh process gets its own initially empty application cache and two
ten-second rapid-wheel blocks. All four arms pass unchanged focus, input-count,
capacity and target-PID presentation checks. OS file cache and Go metadata are
not cleared. No builds, tests, offline analysis or UI Automation observer run
during gestures. The normal portable app remains alive.

| Arm / Qt PID | Cold uniform creations | Cold total renderer creations | Last stored entries | Cold / next missed refreshes | Cold / next max display gap |
| --- | ---: | ---: | ---: | --- | --- |
| On / 91120 | 177 | 1,335 | 193 | 7 / 2 | 12.5282 / 12.5522 ms |
| Off / 32056 | 456 | 1,517 | 194 | 2 / 0 | 12.5046 / 6.4253 ms |
| Off repeat / 62116 | 416 | 1,382 | 198 | 5 / 2 | 12.5240 / 12.5313 ms |
| On repeat / 89888 | 138 | 1,195 | 199 | 6 / 2 | 18.7842 / 12.4899 ms |

Missed-refresh threshold is a display gap >9.375 ms. All blocks have zero
coverage holds; large individual motion errors still fail the requested end
state. Total renderer creations include observed buffers/textures, not every
driver-internal allocation, shader or sampler. Stored entries are cache
snapshots, not unique-file counts or per-file latency. Progress differs slightly
and these are ordered pairs, not randomized statistical proof of regression.

The cold uniform count falls about 61% in the first comparison and 67% in the
reverse-order comparison. That supports the allocation mechanism; it does not
support a frame-time improvement. Next-block uniform counts also decrease,
but media arrivals and total creations differ between runs. Summed allocation
bytes in the structured analysis describe turnover, not peak/live memory.

## Remaining stalls and decision

The enabled runs still show geometry Map calls of 8.4754/8.7915 ms and a texture
upload of 10.7566 ms. The repeated control has a 10.4329 ms upload and 7.4624 ms
Map. API wall time alone does not establish blocked time or GPU execution;
no scheduler trace was collected for these four arms.

Do not promote this candidate as a scrolling optimization based only on fewer
allocations and passing tests. It can remain available for later combined
experiments, but the next intervention needs to target dynamic-buffer mapping
and texture-upload waits rather than another reduction in small explicit
constant-buffer creation. The earlier scheduler stacks remain the evidence
for allocation-lock/driver wait paths; this capture does not identify a new
lock owner or prove that every stall has the same cause.

The snapshot contains the candidate, failing/passing regression, rendered
fixture captures, build receipts, all four live packets and analysis, with
external hashes for large binaries. Diagnostic processes were stopped. The
normal launcher hash remains unchanged. No production change, commit or push.

AI assistance has been used to create this output.
