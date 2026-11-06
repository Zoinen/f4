# Masonry: verified warm wheel and middle scrolling

## Outcome

The full 160 Hz goal remains unachieved. A complete warm-up route now succeeds
with all 192 Downloads entries. Four warm wheel captures contain one missed
refresh in approximately 12,800 display intervals, but every wheel window still
fails the unchanged maximum two-physical-pixel motion gate. A separate warmed
quarter-height middle-scroll capture passes both ten-second windows, including
the 0.002-pixel native velocity gate. This is one passing middle run, not proof
of repeatability or cold-loading performance.

The previous goal turn was progress: it preserved and verified the dependency-set
experiment. This turn adds missing real-content warm coverage and distinguishes
curve execution from displayed timing. No production source, Qt package, portable
payload, setting, quality level, commit or push was changed.

## Protocol and verification

The diagnostic host is the same executable used for the preceding cold-cache
dependency experiment, SHA256
`631b87d7976664b5f55b3400d0ef23cc48576a58b85d7aa788000b8f4c608a11`.
All existing experimental flags remain matched. A uses the original QSet graph;
B uses the optional compact dependency sets. Order is A1/B1/B2/A2. The compact
representation remains off by default. These are diagnostic configurations,
not performance claims about an unchanged production renderer.

Every capture uses the actual C:\Users\xs\Downloads folder, Masonry density 152,
192 entries including the parent, 3840×2160 output, actual DPR 1.75 and 160 Hz.
Each process starts with a fresh isolated gallery cache, then visits overlapping
viewport positions from top to bottom and back through the normal scrollbar
method. Every one of 19 waypoints requires zero pending requests, all expected
previews ready or definitively failed, stable identities/counters for 500 ms,
and a subsequent frame swap. All indices 0–191 must be visited. Waypoint focus,
DPR, refresh rate and quiet-time evidence are checked again offline. The four
wheel tours take 15.45–15.67 seconds; the middle tour takes 15.57 seconds.
This verifies preparation along this route, not permanent residency of every
offscreen scene or a cold operating-system file cache.

The cold-skip variable is removed from the environment: setting it to zero
would still skip the route because the gate checks variable presence. The warm
observer stops before input. Native foreground callbacks verify ownership
throughout the timed windows. There are no inactive inputs, recorder overflows,
or coverage holds. No UI Automation, screenshot capture, builds or offline
analysis run during gestures. Initial and final screenshots are outside timing.
PresentMon retains display tracking and uses the same no-GPU-duration protocol.

An initial preparation-script error invoked the old namespace's wrapper, whose
helper-name assertion rejected it before launching an application or submitting
input. The corrected attempt uses fresh `warmdepsr2` case/cache/observer names.
It is not a rejected timing run replaced by a favorable result.

## Wheel results

| Arm/window | Preparation p99 / max, ms | Display max, ms | Intervals >9.375 ms | Motion error max, physical px |
| --- | ---: | ---: | ---: | ---: |
| A1/0 | 2.1080 / 2.9970 | 6.4074 | 0 | 2.6663 |
| A1/1 | 2.1661 / 3.0404 | 6.3983 | 0 | 2.7964 |
| B1/0 | 1.9794 / 2.6624 | 12.5073 | 1 | 87.8813 |
| B1/1 | 1.8541 / 2.3362 | 6.4403 | 0 | 2.1990 |
| B2/0 | 1.8059 / 2.5445 | 6.4510 | 0 | 2.9818 |
| B2/1 | 1.8088 / 2.4409 | 6.5683 | 0 | 5.8994 |
| A2/0 | 1.9999 / 2.7784 | 6.3998 | 0 | 2.6334 |
| A2/1 | 1.9959 / 2.6272 | 6.4124 | 0 | 2.8562 |

PresentMon target rows are 3642, 3629, 3618 and 3603. Delivered wheel counts
are 1835, 1823, 1775 and 1776, with zero inactive events. Input policy and duration
match; delivered counts and paths are not identical. Native foreground and
bounded-buffer checks pass for all four captures. None passes all performance
gates. Do not use the zero-miss subset or pooled p99 as a completion claim.

The preceding cold captures had 130 control misses and 114 candidate misses;
these warmed controls have zero and warmed candidates have one. This separates
the workloads but is not an interleaved cold/warm causal experiment: run times,
loading and membership churn differ. Cold preparation still reaches 7.61 ms
in the candidate, including a 3.98 ms upload bracket in that frame. Improving
that loading tail remains necessary even though warm preparation fits the budget.

## Curve versus presentation timing

For unchanged job, command revision, curve and geometry mapping, the analyzer
recomputes the declared curve at the recorded animation clock and compares its
displacement with the actual render-node translation. Maximum native curve error
is 0.001313 physical pixels across the four captures. The larger displayed errors
therefore are not explained by a failure to apply the recorded curve.

For example, A1's worst second-window pair advances its animation by 6.25 ms,
while the display interval is 6.3983 ms; its displayed displacement error is
2.7964 px and native curve error is 0.000281 px. B2's worst second-window pair
has a 5.9327 ms display interval against the same 6.25 ms animation advance:
5.8994 px displayed error, 0.000087 px native error. B1's missed-refresh pair
advances 6.25 ms but is displayed 12.5073 ms later, producing 87.8813 px error.

This identifies the clock/display relationship to investigate; it does not
identify whether the remaining hitch originates in Qt pacing, GPU scheduling,
composition, a driver, or the measurement's display assignment. The cold-frame
phase/present join also shows rendering completing well before its displayed
timestamp. Consequently the CPU cost of the frame named by a displayed interval
alone does not establish causality. A selected-output presentation timeline with
matching scheduling evidence is needed; neither earlier nor later neighboring
frames should be blamed solely from their CSV index.

## Middle scrolling

After the same full warm tour, the pointer displacement is exactly one quarter
of the logical window height: 296.5 of 1186. There are 1014 active checks, zero
inactive checks and endpoint reversals. Original QSet storage is used here.

| Window | Display max, ms | Misses | Display velocity error max, px | Native velocity error max, px | Coverage holds |
| --- | ---: | ---: | ---: | ---: | ---: |
| 0 | 6.4230 | 0 | 0.52544 | 0.000729 | 0 |
| 1 | 6.5331 | 0 | 0.81498 | 0.000981 | 0 |

The strict middle analyzer exits zero with every original gate intact. Both
windows have over 1590 compared motion pairs, zero excluded geometry pairs and
only reversal boundaries excluded. This twenty-second result needs repetition
and cold/loading coverage; the candidate middle configuration is not tested here.

## Preserved state and next work

The diagnostic process trees are stopped by their verified paths/parent chains.
Both finite display holds are released and terminal. The normal portable
worktree launcher PID 23488 and extracted Qt PID 84780 remain alive.

The next targets are bounded cold scene/upload work and the rare warm
presentation hiccup. Capture scheduling and selected-output composition timing
on the same failing run before changing presentation policy. Retain the existing
precise native animation clock, content fidelity, two-pixel motion limit,
0.002-pixel middle native-velocity limit, and zero-coverage-hold requirement.
Do not promote the small-set cache as the full solution.

Evidence is frozen in
`profiler/experiments/masonry-verified-warm-cadence-2026-10-31/`; manifests pin
compact sources/results and external raw packets, CSVs, binaries and images.

> AI assistance has been used to create this output.
