# Active-stage binding snapshot experiment, 2026-10-31

Copying only present D3D11 binding stages passes the exercised correctness
checks, but this four-run comparison does not establish a consistent p99 or
display-interval gain. Do not roll it into production as the scrolling spike fix
on this evidence. The constant-160-Hz goal remains active and unachieved.

## Candidate and comparison integrity

The candidate appends a command-owned ResourceBatches snapshot and copies only
the fields consumed by stages whose present flag is true. It preserves dynamic
offset recording and avoids pointers into mutable binding builders. A diagnostic
startup flag selects the original complete-copy path or the candidate path.
This flag and the candidate remain diagnostic only.

The retained prototype's D3D11 object, helper and source hashes were checked
against its original build receipt. Its source differs at one call site plus
the helper include. The current production Qt6Gui.lib is byte-identical to the
prototype's original QtGui archive (SHA-256
405de78cdc8c174da24b03833ea65a7bc05069ae22c408333b6d7624a0a515c3).
Relinked this one RHI object with the current production graph, including bounded
ordering scratch reuse, and the existing main-only frame/input recorder.
No other old experimental renderer, decoder or controller object was included.

All four captures use the same native binary, SHA-256
1abf2b0dacdb8cd99ce6ab16d1636b6b753e171cdac64a6f99c7ff95f210c15c,
and packaged Go bytes, SHA-256
ac68c2e7c2fd259b760f91fc7c001be26d77ec2b33c86c36d1f22893a53fe613.
Each arm starts a fresh process to select its startup flag. Order is
control/enabled/enabled/control. All restore the corrected maximized geometry
before renderer initialization and use the same owned profile.

Each capture lasts 30 seconds with Downloads, 192 entries including parent,
Masonry density 152, actual DPR 1.75, 160 Hz and foreground state verified at
every input. All first input samples start at contentY zero. No UI Automation,
screenshots or heavy analysis ran during timed input. No external workloads,
VMs or affinities were changed. This is a sequential four-run experiment,
not a randomized statistical proof or verified cold-cache/warm-tour test.

## Correctness checks rerun

Existing hash-verified native test binaries were rerun against the unchanged
QtGui/driver environment. The original snapshot test has its expected
finished-builder-retention failure (19 pass, one expected failure). The candidate
has 21 passing cases, including its throughput case, all fourteen individual
binding-stage groups, combined stage combinations, source mutation and recorded
binding independence. The throughput case reports 0.16 ms per iteration; this
is not an app-level before/after result.

Both original and candidate GPU suites pass four cases each, including graphics
pixels with texture changes/dynamic offsets and compute uniform/storage-buffer
readback. These suites do not establish complete app or physical-pixel visual
acceptance. No static UI layout was changed. The candidate has not passed a new
portable rollout or complete viewer/SVG acceptance audit and is not deployed.

## Timing results

| Capture | Arm | Wheel events | Render p50 / p99 / max, ms | Render ranges >6.25 ms | Display intervals >9.375 ms | Max display interval, ms | Dropped rows |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| active-control-a1 | original copy | 2819 | 1.05170 / 2.95202 / 7.0326 | 3 | 2 | 12.5035 | 0 |
| active-enabled-b1 | present stages | 2799 | 1.02145 / 2.93189 / 5.8527 | 0 | 2 | 12.5034 | 0 |
| active-enabled-b2 | present stages | 2770 | 1.08910 / 3.02105 / 6.3601 | 1 | 4 | 18.7497 | 2 |
| active-control-a2 | original copy | 2789 | 1.05990 / 3.01495 / 7.7230 | 2 | 3 | 18.7468 | 0 |

The enabled arms have fewer render ranges over 6.25 ms in this sample, but p99
and median overlap or reverse, and display outliers remain. Four small counts
do not establish a reliable change in tail probability. No outlier, dropped row
or failed condition is discarded. PresentMon display-change notifications are
not optical scanout proof; render ranges are not displayed intervals.

## Input-onset observation

The first wheel event arrives about 10 ms after the recording interval begins.
One counted long-interval row in each of b1, b2 and a2 has its Present entry
before that first input (3.2703, 0.9932 and 4.8555 ms respectively). These rows
remain in the table and original analyses. Their associated display interval
can span an earlier presentation; do not attribute it to wheel-handler cost
merely because it is in the recording window. Do not relabel the goal achieved
by excluding these rows. Adjacent post-input rows and the two b2 outliers at
13.67 seconds also remain. Their exact compositor/motion cause is unproven.

## Decision and remaining work

The normal app is unchanged and remains on the previous production build.
This candidate is retained for possible CPU-headroom work, not promoted as an
established spike fix. Next isolate per-frame vector-gate ancestor traversal,
another directly sampled render cost, against the current graph. Preserve the
hidden-SVG residency and painter-order checks; an optimization must not skip
their validation. Presentation outliers still need correlation with actual
motion and full frame/driver timing rather than render averages alone.

The original cold rapid-wheel, verified all-192-entry quiescent warm tour,
quarter-window-height middle motion, physical motion error, coverage holds,
all-format/native-SVG behavior and 175% resting alignment requirements remain
unverified for a complete performance acceptance claim. No commit or push was
performed. Original receipts retain one misleading inherited main-only field;
the comparison summary explicitly corrects it to main plus RHI overrides.

Compact evidence and a SHA-256 index are frozen beside this report; raw capture
JSON/CSVs, binaries and source/object references are retained externally.

> AI assistance has been used to create this output.
