# Masonry pipeline-key construction control, 2026-10-31

## Decision

Do not promote this prototype as the scrolling fix. Moving descriptor values
reduces the isolated key-construction benchmark by about 8%, but the live
comparison does not establish a useful cold-scroll improvement. All twelve
measured 30-second windows fail the existing full performance gate. Constant
160 Hz without spikes remains unachieved.

No production source, installed Qt package, portable payload, process priority,
external application affinity, or background workload was changed by this
experiment. The normal worktree application remained running. Only separately
identified diagnostic process trees were stopped after captures.

## Prototype and correctness

`artifacts/PipelineKey20261031.h` wraps the existing Qt graphics-pipeline key
creation. Mode 0 uses the original implementation. Mode 1 moves serialized
descriptor values into the final key, retaining Qt's hashes and full value
comparisons. Mode 2 adds bounded, per-render-pass value-based hash reuse (16
entries, 64-word maximum, eight nested scopes), with fallback. It uses no raw
resource-pointer identity as a substitute for compatibility.

Mode 2 was not used for live comparisons: its benchmark overhead erased the
benefit. All live arms use the same new executable, with mode 0 for A and mode
1 for B. Typed scene registry and uniform word hash remain enabled in both;
the uniform diagnostic snapshot observer remains disabled in both.

Final checks passed in each of modes 0, 1 and 2:

- 28 helper tests, including independent allocations, full-value differences,
  COW mutation, capacity/depth fallback, replacement resources and hash fields.
- 46 native tests at 175% scale. The 25 rendered PNGs are byte-identical across
  the three modes.
- Three installed-tree-independent QML import checks and system-DLL audit.

The initial helper fixture incorrectly used a null shader manager: Qt's key
comparator dereferences its material shader. The fixture now uses real shader
instances. The first main build also failed because an early private-header
include suppressed the existing diagnostic access shim; include placement was
corrected before the successful build. Both failed attempt logs are preserved.
Neither failure is evidence of an application regression.

The benchmark includes per-pass scope overhead, uses independently allocated
descriptor lists and validates the same nonzero rolling checksum. Twelve
trials per mode yielded median 17.90735 ns for mode 0, 16.44325 ns for mode 1,
and 18.22205 ns for mode 2. These are primitive costs, not frame times.

## Live controls

Actual Downloads folder: 192 entries including parent, Masonry density 152,
3840 by 2160 desktop, 160 Hz, DPR 1.75. The ordinary right panel remains
present. Cold captures start at the normal initial viewport. Warm captures
first complete the existing verified full-folder tour. Each capture contains
two 30-second wheel windows. No screenshot, UI Automation observation, build,
or analysis was run during measured gestures. Native foreground monitoring
reported zero inactive wheel events.

The unchanged gates require display intervals at most 9.375 ms, maximum
physical motion error at most 2 px and zero coverage holds. No missing-frame
pairs were removed or thresholds relaxed. All windows have zero coverage
holds, but all fail other gates.

| Scope / arm | Misses, pass 1 / 2 | Maximum motion px, pass 1 / 2 | Preparation p99 ms, pass 1 / 2 |
| --- | --- | --- | --- |
| Cold A1 original | 102 / 61 | 137.404 / 120.997 | 3.4909 / 2.4346 |
| Cold B1 move | 102 / 123 | 131.403 / 121.627 | 3.4055 / 3.3642 |
| Cold B2 move | 96 / 117 | 166.984 / 160.303 | 3.1038 / 3.4459 |
| Cold A2 original | 143 / 177 | 124.571 / 128.560 | 3.5927 / 3.9549 |
| Warm A1 original | 3 / 0 | 85.656 / 3.278 | 1.6196 / 1.7277 |
| Warm B1 move | 1 / 0 | 3.417 / 3.326 | 1.6158 / 1.6843 |

Warm preparation medians are 0.4074/0.4099 ms for A and 0.3935/0.3926 ms
for B. Alpha-material medians are 0.2108/0.2106 versus 0.2012/0.1984 ms.
This single warm A/B pair is insufficient to establish tail reliability;
the cold A/B/B/A series has substantial variation and no consistent win.
The large first warm A motion excursion remains a failed sample, not an
excluded outlier or proof of a particular subsystem's fault.

## Evidence and next investigation

`artifacts/pipeline-key-20261031/live-summary.json` records preparation and
material statistics for all windows. Each arm directory beneath
`artifacts/pipeline-cold-20261031` and `pipeline-warm-20261031` contains native,
display, original motion-gate and foreground results. Raw packets are preserved
at the paths in `postsync-probe-20261031/<case>/raw-location.json`; only
hash-verified duplicates were retired. Build/source hashes are in
`pipeline-key-20261031/probe-build-receipt.json`.

The profiling drive became full before the remaining captures. Six completed
diagnostic build folders were relocated to the named C-drive archive, checking
every file's SHA-256 before retiring the original copy and preserving original
paths with junctions. See `pipeline-final-space-relocation-20261031.json`.
Captures resumed only after the existing free-space guard could pass.

The next useful scope is loading-time scene/material work and geometry changes,
as identified by the preceding cold CPU profile. A source inspection ruled out
the simple claim that every wheel retarget stops the native camera: the active
native path updates its destination/revision and preparation target directly.
It does not call `finishNative` for ordinary wheel retargets. Actual geometry
rebase frequency and resulting subtree work still need measurement; they are
not established causes in this report.

This is a diagnostic experiment, not a portable release validation or rollout.
