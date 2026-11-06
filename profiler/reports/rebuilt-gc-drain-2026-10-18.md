# Rebuilding the real Qt GC deadline loop

## Result and scope

The preceding turn corrected input-mode and clock attribution and proved that a
standalone symbol override did not reach the live collector. That was progress.
This turn rebuilds the complete matching Qt memory-manager translation unit,
verifies its internal code and live engagement, and measures actual wheel input
at two GC time budgets. The smaller marking chunk is not promoted: default-budget
results remain mixed and the full constant-160-Hz objective remains unfinished.

This is a diagnostic experiment. Production sources, SDK packages, normal Qt
libraries, compressed payload and portable launcher are unchanged. No commit
or push is made.

## Matching source and actual internal callers

The official qtdeclarative v6.11.1 archive supplies the exact memory source and
143 matching masm/header files. Its qv4mm.cpp matches the previously fetched
6.11.1 source. Archive SHA256:
3d98c58a8652f7188288832d8bea5529a1bc3906915ad1c9a2974955c8cd5656.
Memory-source SHA256:
7734908613887fb6988f2a351534d48d2bd9b220893663cafc9df6c8a6f4dbf2.
The existing 6.10.1 Conan source tree is not used.

The original SDK QtQml compile command and installed 6.11.1 headers are used.
The first attempt encounters a generated moc include pointing to a removed
Conan source directory; a local copy of that generated file redirects only the
include to the matching installed header. The SDK and original generated files
are not modified. The library's single qv4mm object is removed and replaced
with the rebuilt object, without /FORCE:MULTIPLE.

The rebuilt control's internal markDrain is 231 bytes and is byte-for-byte
identical to the installed SDK, SHA256
e8e593a683802c108522980bb410035fff5f20a7af467933d902f0395e765e8c.
The candidate changes the inlined loop compare/boundary branch. Its internal
caller hash is 6aaa0de25ce119f26462f259524db15af36ec308b7d152263cae59e91ddbc42f.
The clean source delta is only i<=1024*10 becoming i<128 in timed MarkStack::drain.
Untimed draining, object ownership, GC states and sweeping are unchanged.

A separately compiled observer proves actual engagement: during 268 wheel events,
replacement-call counts rise 29->41 downward and 41->54 upward at a verified
1000 us budget. Counters are inside the real translation unit and its inlined
callers. Clean candidate/control builds have no counter or phase/object observer.

The focused native regression queues a warmed valid heap object 4096 times in a
private MarkStack with an already-expired deadline. The rebuilt control drains
4096 and returns Complete (expected regression failure); the candidate yields
Ongoing after 128 and passes. Both retain strings through a separate real
50000-object JS allocation workload and two collections. This verifies yield
behavior, not the entire Qt GC/lifetime suite. Full QML/release gates are not
claimed; no production rollout is attempted.

## Clean comparisons with actual wheel events

Both sides use the same experimental bounded-order QtQuick renderer to isolate
GC from order-exhaustion rebuilding. This renderer itself remains unpromoted.
Workload: Downloads 184 entries, density 152, extent 4161.142857, actual DPR 1.75,
active 160 Hz, default D3D11, wheel delta 120 every 30 ms for four seconds per
direction. Folder state is restored between fresh processes. Actual metadata
verifies 267-268 wheel events and stable order ranges near 223600 in every run.
Caption measurements remain zero. No build, tests, logger, sampler, ETW recording
or scene observer runs during clean comparisons.

PresentMon displayed intervals are the headline, with both endpoints inside
the same direction observation window. All observed presents use DXGI,
Composed: Flip, SyncInterval 1 and one swapchain per process. Inconsistent timing
is retained without an unsupported environmental explanation.

### Equal experimental 1000 us budgets

| Run | Down p99 ms | Down max ms | Down >9.375 ms | Up p99 ms | Up max ms | Up >9.375 ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| A1 | 18.7304 | 25.0602 | 30 | 6.5313 | 12.5322 | 6 |
| B1 | 6.3437 | 25.0025 | 3 | 6.2991 | 12.4974 | 1 |
| B2 | 6.3673 | 12.4951 | 2 | 6.2765 | 12.4875 | 1 |
| A2 | 6.3838 | 12.5061 | 5 | 6.3171 | 12.5016 | 1 |

The candidate has fewer long intervals in both pairs, but A1 is substantially
slower in both GUI/native and display measurements. It is not discarded; nor
is the full difference attributed to GC without supporting phase evidence.
Candidate B1 still reaches a 25 ms displayed interval. This does not establish
a constant-160-Hz solution or a rollout under the normal time budget.

### Equal normal 5000 us budgets

| Run | Down p99 ms | Down max ms | Down >9.375 ms | Up p99 ms | Up max ms | Up >9.375 ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| A1 | 6.3112 | 18.7536 | 3 | 8.3294 | 12.5020 | 2 |
| B1 | 9.3590 | 12.5039 | 5 | 9.3639 | 12.4241 | 2 |
| B2 | 6.2978 | 12.5060 | 2 | 7.3237 | 12.5319 | 2 |
| A2 | 9.4945 | 13.6507 | 36 | 9.4691 | 13.6773 | 32 |

B1 worsens p99 and the downward long count in the first pair, while B2 improves
against the much noisier A2. The overall result is mixed; neither unfavorable
candidate results nor slow controls are removed. No unverified display-mode,
background-process or scheduling theory is used to explain away A2. The evidence
does not justify changing the default chunk/budget as a reliable scrolling fix.
Stems: mm-grain-{a1,b1,b2,a2}-wheel-masonry and
mm-default-{a1,b1,b2,a2}-wheel-masonry. Compact summaries preserve every run.

## Deadline limits and individual objects

Separate Qt phase-logging captures use actual wheel events and 1000 us budgets.
They are attribution only, excluded from clean cadence results. The control has
17 MarkDrain steps, max 2495 us and total 24636 us; its two DoSweep steps peak
at 3536 us. The candidate has 17 MarkDrain steps, max 17450 us and total
34030 us; its two sweep steps peak at 7375 us. The long candidate mark step is
about 429 ms into the downward observation window, not an idle/startup interval.
There is no simultaneous scheduler capture for that phase outlier; its wall time
must not all be asserted CPU marking or attributed causally to the chunk change.

A further full-source observer measures individual timed-drain markObjects calls
and records only calls over 500 us. The actual wheel/1000 us run has four interior
calls: QQuickItemWrapper 698.9 us, QObjectWrapper 572.4 us, QQuickItemWrapper
953.8 us and QQuickItemWrapper 583.7 us. Both endpoints are inside a scrolling
direction window. Matching raw-QPC ETW confirms the calls are almost entirely
scheduled execution: respectively 688.9, 572.4, 953.8 and 583.7 us; the first
also has 10 us ready time. The 17.45 ms phase outlier is not reproduced here
and is not explained by these shorter object calls.

The ETW trace has zero lost events/buffers. Raw alignment matches 51061 switches
with 900 ns offset spread. Per-object timers, phase logging and sampling perturb
performance; their frame cadence is not compared with clean runs. This confirms
that object traversal itself is indivisible work below the marking-loop deadline
check. Sweeping also remains atomic in the unchanged Qt implementation. Smaller
chunks cannot alone bound all GC work to the requested frame budget.

## Saved evidence, restored app and next action

profiler/experiments/rebuilt-gc-drain-2026-10-18 contains the source delta, local
builders, source manifest, code proof, engagement/native results, phase/object
summaries and both complete clean comparisons. Raw traces, SDK source archive,
headers, objects, libraries, PDBs, binaries and logs stay untracked in artifacts.
The tracked source tree remains clean.

The panel state is restored. WPR reports not recording. Only this worktree's
process tree is replaced, and the normal portable launcher starts directly
with --gui=qt after clearing experimental host, GC, renderer, scale and logging
overrides. The extracted host matches the normal host build, title identifies
zoin_branch/cached-pixel-grid, and startup stderr is empty. Canonical
D:/Code/f4/f4.exe is untouched. Evidence: artifacts/mm-grain-normal-identity.json.

- Host SHA256: 549f063f2f8e3252f5784f0b017d6500b719102ff799fb49b279964e44383bed
- Launcher SHA256: b9c479436f6fbc954f0eabccf67594844a1e35fd962a3c7eab40eb6e4e7f9d58

The next target should be allocation/retirement churn in nearby delegate
preparation and the object graph being retained/marked, alongside the independently
verified cold geometry upload/copy cost. Measure those producers and their GC
debt without expanding caches unboundedly or changing ownership without lifetime
regressions. Repeating time-budget/chunk constant trials is not the next action.
Constant 160 Hz without spikes remains active and unachieved.
