# Initial dynamic geometry removes conversions, but cold scrolling still spikes

The diagnostic starts newly allocated D3D11 batch vertex/index buffers as
Dynamic instead of Immutable. It eliminates observed immutable-to-dynamic
conversions and reduces buffer creation substantially. Two enabled runs have
zero missed refreshes in their second block, versus one and eleven for the
same-executable controls. Cold blocks still miss refreshes and all four runs
fail the complete motion/no-spike requirement. This candidate remains isolated;
it is not a finished optimization or a portable rollout.

## Exact scope

The starting renderer is the uniform-capacity candidate based on the actual
current production batch renderer. The only added renderer policy changes
the initial newBuffer type for batch geometry on D3D11. No capacity is widened,
pool budget changed, upload budget increased, media worker reduced, or geometry
discarded. Other backends retain the original policy. Existing recycled buffers
continue through the original capacity and promotion logic. A diagnostic flag
selects the change in the same executable, cached once per process.

Both arms retain the combined guarded geometry reuse, bounded nonblocking
staging uploads and uniform capacity retention. The same allocation classifier
tags native buffer creation in both. The profile records the new flag explicitly;
receipt hashes cover the host, patched source and compiled objects. The source
change reverses exactly to its base. These are artifact-only native diagnostics,
using the normal portable Go launcher's explicit diagnostic-host path.

## Regression and packaging checks

A native regression inspects the very first rendered batch's QRhi vertex buffer
and native pointer, then changes rectangle geometry twelve times without
changing allocation size. It verifies type, logical/native identity and
vertex-buffer creation count, interior/exterior/changed-edge pixels, actual
D3D11, actual DPR 1.75, scene origin at physical (14,14), and scene unit vectors.
It saves the enlarged rendered rectangle; its edges were visually inspected.

The unchanged renderer fails the initial-type assertion (Immutable 0 versus
Dynamic 2). The candidate passes all 47 native checks with zero failures/skips.
An initial fixture mistake sampled physical (80,80) as background, although the
enlarged rectangle ends at physical 84. That failed test/source is retained.
The corrected sample is (100,100); no runtime policy or timeout changed.
The current baseline regression was rerun and still fails as intended.

Four compiled-host/import-isolation checks and the Windows system-DLL audit
pass. Earlier combined geometry-safety and staging suites are inherited
evidence, not newly rerun counts. No embedded payload or normal executable was
replaced; these checks do not constitute a finished portable-release gate.

## Capture discipline

Actual C:/Users/xs/Downloads, 190 entries, Masonry density 152, actual
160 Hz / DPR 1.75. Enabled/control/control/enabled, separate fresh processes
and initially empty application caches, each with two ten-second rapid-wheel
blocks. Standard media workers and hardware decoding remain enabled. The OS
cache is not flushed. Each 45-second helper retains stdout until completion;
no build, offline analysis or UI Automation observer runs during gestures.
Input count, focus, PID, capacity, feature-use and zero reuse/upload-error checks
pass. The same observers are retained in both arms; zero observer effect is
not asserted.

## Results

| Arm / Qt PID | Cold / next missed refreshes | Cold / next largest display gap | Last stored cache entries |
| --- | --- | --- | ---: |
| Enabled / 71780 | 6 / 0 | 12.7291 / 6.7266 ms | 201 |
| Disabled / 109140 | 6 / 1 | 24.9791 / 12.4937 ms | 204 |
| Disabled repeat / 92412 | 5 / 11 | 18.7702 / 19.1580 ms | 191 |
| Enabled repeat / 111288 | 5 / 0 | 12.5405 / 6.4395 ms | 201 |

Misses use display gaps above 9.375 ms. Coverage holds are zero. Enabled next
blocks still have maximum presentation-derived motion errors of 6.0776 and
2.6398 physical pixels, above the two-pixel limit. This is not physical scanout
measurement. Two ordered pairs and different loading progress do not establish
statistical significance or a general guarantee of better cadence.

| Arm | Cold buffer creations | Cold same-capacity promotions | Cold growth without usage change | Next buffer creations |
| --- | ---: | ---: | ---: | ---: |
| Enabled | 679 | 0 | 382 | 54 |
| Disabled | 1134 | 469 | 370 | 134 |
| Disabled repeat | 973 | 372 | 338 | 194 |
| Enabled repeat | 701 | 0 | 395 | 45 |

Counts include all native buffer creations on the render thread wholly within
the measurement windows, including uniform buffers. Promotion producer records
are absent throughout both enabled processes. Capacity growth is not reduced:
the candidate's cold counts are actually higher in these runs.

The enabled repeat records an 8.8188 ms growth allocation of a 72,468-byte
dynamic vertex buffer, plus 9.8417 ms texture creation and a 7.3035 ms outer
upload containing 7.2007 ms staging-texture creation. The reverse control
records a 12.9831 ms growth allocation of a 159,324-byte dynamic vertex buffer.
These are neither first allocations nor pure type conversions. Wall time does
not by itself attribute CPU execution, lock ownership or GPU dependence.

## Memory limits

After the input and PresentMon collection end, one process-memory sample is
taken using psutil; no process-memory polling runs during scrolling.

| Arm | End private bytes, MiB | Process peak pagefile bytes, MiB |
| --- | ---: | ---: |
| Enabled | 1180.7 | 1879.9 |
| Disabled | 1199.2 | 1932.7 |
| Disabled repeat | 1182.6 | 1923.9 |
| Enabled repeat | 1188.5 | 1760.7 |

These samples show no obvious process-memory regression in this small set;
they do not isolate geometry/shadow memory, prove bounded GPU memory, or prove
that starting every geometry buffer as dynamic has no memory cost. Dynamic
buffers add a CPU shadow where immutable ones did not need it, and driver
allocation behavior differs. Additional lifetime accounting is required before
promotion. The existing pool limit alone is not a bound on all active buffers.

## Next action

Keep this candidate for controlled combinations, but do not promote it on these
results. Investigate capacity growth/retired-buffer selection and small texture
allocation separately. Native allocation stalls survive even after conversions
are gone, and media progress must remain intact. Cold/warm wheel and
quarter-height middle scrolling, strict motion gates, reduced-observer checks
and the complete embedded portable rollout remain outstanding.

The matching experiment snapshot preserves source/build identity, tests,
corrected and failed fixture evidence, rendered output, compressed capture data,
target-PID PresentMon rows, memory sidecars and analyses with hashes. Diagnostic
process trees were stopped. The normal portable launcher and its Qt child remain
alive; the launcher hash is unchanged. No commit or push.

AI assistance has been used to create this output.
