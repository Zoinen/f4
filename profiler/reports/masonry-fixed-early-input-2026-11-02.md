# Fixed early input for the Masonry comparison

## Current result

The next control/candidate comparison is built and prepared, with a common
fixed-input recorder. Scheduler/validation tests and native portable-host checks
pass. **No live capture has been launched with it.** Windows currently exposes
only the Microsoft Remote Display Adapter at 1512x949 and 32 Hz, with that as its
only enumerated mode. The physical 3840x2160/160-Hz lease fails its availability
gate before changing display mode. These builds therefore supply no new live
cadence, coverage-hold or physical-motion result.

The previous turn was progress: the tested frame-boundary handoff prototype,
four live captures and a new occupancy analysis. This turn addresses a concrete
comparison weakness from those captures: navigation-to-input timing depended on
content readiness, and wheel direction depended on observed scroll position.
The full constant-160-Hz objective remains active and unachieved.

## Workload specification

The initialized empty-folder baseline and normal `panel.navigatePath` action are
retained. Both panels enter Downloads. The recorder attaches to the empty panel
before navigation; it does not wait for 192 entries, thumbnails or surfaces.

Input has 3,000 planned slots, starting **200 ms after navigation**, separated by
10 ms, over 30 seconds. The direction reverses every 150 slots (1.5 seconds),
independently of content position. This preserves the previous rapid-wheel input
rate while fixing its intended timeline. It does not replace the separate warm
tour, middle-button, or other scrolling verification still required.

Each delivered event records its intended slot, deadline, actual timestamp,
lateness, acceptance, route, dataset count, mode, density, native foreground,
window DPR and refresh rate. Late callbacks dispatch once at the current due
slot and explicitly record any skipped range. There is no catch-up burst and
no silent reindexing of late events. The stop callback accounts for any remaining
slots. Skipped input disqualifies a complete delivered sequence and remains a
failure to deliver the full intended workload; it must not be used to claim a
performance pass from a reduced workload.

The planned start remains fixed even if arming or GUI dispatch overruns its
deadline. The receipt distinguishes planned start from actual first dispatch.
Events while the folder is loading are retained, with actual entry counts and
paths, rather than filtered to a later fully populated viewport. The initial
viewport, actual final 192-entry dataset and cache identity are verified.

## Tests and analysis

`profiler/FixedWheelSchedule.h` is a small C++ schedule used by the compiled
recorder. `FixedWheelScheduleTest.cpp` tests exact slot boundaries, early waits,
direction changes, late/skipped slots, end-of-window accounting and timer
rounding. Runtime conservation verifies that a callback every third slot yields
1,000 dispatches plus 2,000 recorded skips, totaling the 3,000 planned inputs.
It is compiled with static MSVC runtime; its link also uses the mandatory
response-file wrapper.

`profiler/fixed_early_input.py` validates the actual capture schema. Five Python
tests cover complete delivery, explicit missing slots, retained early-loading
events, readiness/time/direction changes, incorrect routes, native-focus loss,
false slot accounting and truncated captures. Capture-scope admission never
claims a full performance pass. A complete slot sequence does not imply
identical actual dispatch timestamps; lateness remains a measured outcome.

New analysis wrappers use this fixed-input admission instead of the earlier
ready-viewport admission. They retain actual entry-count observations, all
display intervals, unmatched events and physical-motion exclusions. Display
intervals can include stationary boundary periods; an outlier is not
automatically a missed animated refresh. The existing frozen analyzers and
earlier experiment manifests are unchanged. The wheel physical-error gap is
still unresolved and must not be hidden by this new admission.

## Build identities and checks

Both hosts use the same newly compiled recorder/main object. The control retains
the earlier grid-deferral/two-root baseline; the candidate adds the previously
tested frame-boundary handoff. No new incubation limit, layout, renderer or
production frontend change is introduced here.

Control SHA-256:
`a013ebd0a71f82eff59fad2cabe5d28d5b8af9f2d1e5194d201e8ab432a66446`.
Candidate SHA-256:
`911326ec02f6cac189909b15098e3b11d049bfea48c10c187977fe5095b7b489`.

Each passes embedded-only QML import isolation (3 passes), the Windows GUI
subsystem test (3 passes), and the system-DLL audit. These checks ran in the
currently available desktop session and are not evidence of 160-Hz performance.
No new payload was embedded or tested through default-cache extraction; no
production rollout is claimed. Native warnings are retained in the logs.

`cold-n151` is **prepared but not launched**: its host publication and controller
script exist, but no raw capture directory or process identity exists. Do not
rerun its preparer or invent a completed capture. Once the physical display is
available, verify its build receipt and launch the existing controller for that
case. A timeout after launch must be inspected through the same owned handles;
it does not authorize restarting the case.

The controller also has an early native display preflight. An invocation under
the current remote display was verified to reject before creating the raw case
directory or launching the application. That expected rejection is recorded
separately and is not a live capture or a restarted case.

## Remaining work

Revalidate the actual display modes before starting live work. Run the prepared
fixed-schedule control/candidate comparison at actual 160 Hz and effective DPR
1.75. Preserve skipped slots and late starts as outcomes, and require the full
intended workload before a successful-delivery claim. Then continue removing
coverage holds, resolve physical wheel-motion measurement, and verify the full
cold/warm/middle-button workload and portable production rollout.

The normal application is preserved. No capture permission was requested, no
remote session was disconnected, and no commit or push was made. Live
verification is limited by the current display environment; independent
preparation and tests are complete for this harness, while the broader goal
remains active.
