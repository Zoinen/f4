# Current renderer resource attribution and reservation experiment

## Input-mode correction (2026-10-18)

A later metadata audit found that the resource/reservation captures in this
report used continuous linear animation: wheelIntervalMs is 0 and wheelEvents
is empty. The -wheel- filename suffix was misleading. Their timing tables and
native regression results remain as recorded, but these captures must not be
cited as wheel-event measurements. The backend comparison is separately
verified at 30 ms with 267-268 actual wheel events and is unaffected.
The runner now rejects wheel-named captures with a zero wheel interval.

## State and method

The preceding turn saved the implementation checkpoint, changing authoritative
Git state; it was progress. Parent b2318c4b5 and gallery 23d0a5f are the current
checkpoint. This investigation leaves production sources, Qt defaults and the
embedded payload unchanged. Constant 160 Hz without spikes remains unproven.

The resource probe starts from the currently integrated Qt 6.11.1 renderer,
including stable intervals, buffer-fit selection and the native-child stacking
guard. It distinguishes new buffers, capacity growth, immutable-to-dynamic
promotion, combined growth/promotion, direct dynamic copies and static copies.
It does not change buffer selection, pool limits or upload policy. The broad
probe includes frequent material/map/preparation scopes; the reduced probe
removes those. Cause v2 additionally records the exact reservation-exhaustion
fallback. No expensive ancestor/owner mapping is enabled. Four diagnostic
captures have zero event-array overflow. These captures are attribution evidence,
not clean cadence comparisons: even the reduced probe adds timing and JSON work.
Individual events below 50 us are omitted; aggregate counts include them.

All captures use Downloads, 184 entries, density 152, extent 4161.142857, actual
DPR 1.75, active 160 Hz window and continuous linear animation for about four
seconds per direction. Caption measurement counts are zero, actual GC budget
is 5000 us, QObject ownership adoption is zero and the RHI is D3D11 (API 4).
Folder state is restored before each fresh process. The probe build initially
failed to link over its running executable; linking to a distinct v2 filename
succeeded. The normal working app was preserved until the candidate existed.

## What remains expensive after buffer-fit correction

The broad capture's worst render frame spends 13.4662 ms rendering. It uploads
398 batches, with 10,050,252 logical vertex bytes and 10,981,172 combined
vertex/index bytes processed by unmap. Its new allocations individually do not
explain the full spike. Across that capture, the slowest recorded growth create
is 0.2486 ms, direct dynamic copy 0.3574 ms and static copy 0.5497 ms. These
are measured wall times, not pure CPU or a universal driver bound.

In the final reduced v2 capture, two exact reservation-exhaustion events both
lead to full render-list reconstruction. The worst frame has a 24.3643 ms native
period and 15.095 ms render wall time, including 12.5961 ms preparation and
8.9476 ms nested batch uploads. It uploads 397 batches, processes 10,981,080
combined vertex/index bytes, creates 353 buffers, grows 66, promotes 8 and both
grows/promotes 4. Nested times must not be summed as independent costs.

Across the complete v2 scrolling capture:

| Buffer operation | Calls | Total wall ms | Largest recorded event ms |
| --- | ---: | ---: | ---: |
| New vertex/index buffer | 495 | 2.0113 | 0.0677 |
| Growth | 276 | 5.4321 | 0.2219 |
| Promotion | 304 | 3.3117 | 0.0530 |
| Growth and promotion | 7 | 0.2926 | 0.1145 |
| Direct dynamic copy | 18151 | 35.9401 | 0.2888 |
| Static copy | 2075 | 1.1706 | 0.3133 |

This does not reproduce the earlier isolated multi-millisecond create call.
Changing every buffer to Dynamic is therefore not justified by this evidence.
The verified remaining burst is broad invalidation and aggregate upload work
when drawing-order reservations run out. Tagged-root updates and smaller
uploads also consume time; avoiding full reconstruction alone is insufficient.
Local stems are current-fit-{profile,light,light-v2}-wheel-masonry. The first two
lack the exhaustion probe: their empty exhaustion lists are not a zero count.

## Controlled reservation trial

A uses the normal four-order minimum; B changes only Qt's existing
QSG_BATCHRENDERER_MINIMUM_ORDER_PADDING environment setting to 2048. Both use
the identical uninstrumented current buffer-fit diagnostic host. This revisits
an earlier trial after stable-interval, stacking and buffer-fit fixes; it does
not assume the older results transfer unchanged. No build, tests, sampler,
ETW recording, debug logging or resource observer runs during clean A/B/B/A.
The larger setting is verified through actual scene orders: A ranges 1399-2175;
B stays at 223640 and 223606 respectively. It is not merely an unverified env var.

Headline intervals are actual PresentMon display changes. Both display endpoints
must lie within the same scrolling direction window, as explained below.

| Run | Down p99 ms | Down max ms | Down >9.375 ms | Up p99 ms | Up max ms | Up >9.375 ms | Max render wall ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| A1 | 6.3161 | 18.7261 | 3 | 6.3297 | 12.4978 | 2 | 11.6255 |
| B1 | 6.3283 | 12.5232 | 4 | 6.3073 | 12.5029 | 1 | 6.8008 |
| B2 | 6.3223 | 18.7069 | 2 | 6.3011 | 12.4737 | 1 | 9.0431 |
| A2 | 6.3209 | 18.7498 | 5 | 6.3148 | 12.4998 | 2 | 11.9676 |

A separate B probe confirms zero full-list rebuilds and zero reservation
exhaustion, versus two of each in default v2. Its vertex/index create calls
fall to 573 from 1082. That comparison demonstrates the mechanism, not a clean
percentage speedup: instrumentation and asynchronous source readiness vary.
B's largest render wall remains 9.147 ms under observation. The clean display
runs retain 12.5-18.7 ms intervals. Neither a 160 Hz guarantee nor elimination of
all spikes follows from the smaller counts.

The global setting is not promoted. It inflates drawing-order space over 100x
for this scene; a production policy needs explicit total/depth-range bounds,
nested-root lifecycle and overlapping-alpha tests, and must avoid consuming
large reservations for every empty root. The next experiment should bound
reservations around cold vector branches, measuring source-to-presentation
latency as well as frame tails, rather than blindly enlarging all roots or
repeating unsupported driver-allocation policies.

## Backend comparison and display-window correction

The preceding clean same-host D3D11/OpenGL A/B/B/A uses the current buffer-fit
renderer. RHI metadata verifies API 4 versus 3. Results with the same endpoint
rule applied to both backends:

| Run | Down p99 ms | Down max ms | Down >9.375 ms | Up p99 ms | Up max ms | Up >9.375 ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| D3D A1 | 6.3235 | 12.5010 | 5 | 6.2801 | 12.4768 | 2 |
| GL B1 | 6.2923 | 37.5179 | 2 | 6.2855 | 12.4942 | 1 |
| GL B2 | 6.3398 | 18.7306 | 5 | 6.3147 | 6.3951 | 0 |
| D3D A2 | 6.3178 | 18.7631 | 6 | 6.2819 | 12.4885 | 1 |

No default backend change is justified. OpenGL's render wall includes backend
blocking and cannot be compared as pure CPU against D3D11's shorter render wall.
An initially reported GL B1 interval of 393.7442 ms straddled startup/idle:
first scrolling frame QPC 66812.4379092 s, present 66812.4387507 s,
msUntilDisplayed 5.3092, so its previous display endpoint is 387.5935 ms before
the observation begins. The valid 37.5179 ms interval occurs inside scrolling
and is retained. This is endpoint accounting, not removal of an unfavorable run.

The optional --strict-display-window analyzer requires current display
QPCTime + msUntilDisplayed/1000 and preceding display minus
msBetweenDisplayChange/1000 both inside the same direction window. Missing
display times are excluded from display statistics. Other metrics preserve the
original present selection. The option writes a separate -present-window-summary
file, leaving old summaries and raw captures intact. No old report is silently
rewritten. A synthetic regression passes for both direction windows: it excludes
idle-spanning and trailing-boundary intervals, retains an interior 37.5 ms
stall, preserves non-display metric selection and leaves the old summary
unchanged. The archived renderer probe patch applies cleanly to the current
integrated base. Stems: backend-{d3d-a1,gl-b1,gl-b2,d3d-a2}-wheel-masonry.

## Saved evidence and restored application

Compact attribution, backend and reservation summaries, exact probe source
delta, diagnostic headers and analyzer snapshot are in
profiler/experiments/current-buffer-attribution-2026-10-18. Raw traces, generated
binaries and logs stay untracked under artifacts. No new production behavior or
UI layout is introduced, so no new pixel-grid layout regression is asserted.
Existing test results from the checkpoint are not presented as fresh runs here.

The panel state is restored. WPR reports not recording. Only this worktree's
process tree is replaced; its normal portable launcher starts directly with
--gui=qt after clearing diagnostic host/backend/GC/reservation overrides.
The extracted host hash matches the normal build, startup stderr is empty, and
the title identifies zoin_branch/cached-pixel-grid. Canonical D:/Code/f4/f4.exe
is untouched. Evidence: artifacts/current-resource-normal-identity.json.

- Host SHA256: 549f063f2f8e3252f5784f0b017d6500b719102ff799fb49b279964e44383bed
- Launcher SHA256: b9c479436f6fbc954f0eabccf67594844a1e35fd962a3c7eab40eb6e4e7f9d58

No commit or push is made in this continuation. The full 160 Hz objective remains
active and unfinished. The evidence changes the next action from resource-type
policy to bounded drawing-order reservation around cold scene generation.
