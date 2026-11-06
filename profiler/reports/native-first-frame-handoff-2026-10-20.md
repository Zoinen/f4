# Native scroll startup handoff clock — 2026-10-20

## Finding and implementation

Repeated idle starts exposed one unused native animation frame. The job's first
render tick initialized its curve at time zero although GUI acceptance and render
handoff had already consumed time. Every baseline job first committed its old
pose; median first motion commit was about11.3ms after wheel input.

GalleryScrollAnimationState now records steady-clock request time. On the first
render command only, the job offsets its start by elapsed handoff milliseconds,
bounded to the duration. Subsequent retargeting preserves existing curve behavior.
A newer command received before first rendering refreshes the request timestamp
under the existing mutex. No per-frame wall-clock query or layout work is added.

Regression: accept a400ms linear scroll, delay GUI synchronization50ms, then read
the first synchronized frame's actual viewport QSG matrix. Before:0logical pixels
on both backends. After:51pixels on both, with final contentY400. This is a timing
change with no new visual hierarchy. Existing native rest-origin/unit-transform
and rendered-image checks at DPR1.75 continue to pass.

## Checks and operational corrections

12 scrolling checks per backend passed, zero failed/skipped: new startup,
three interruptions, retargeting, two thumb mappings, shrink, metadata rewrap,
stop and fallback. The initial build caught mixed qBound integer types; explicit
qint64 arguments fixed compilation. A diagnostic link then hit the still-running
old diagnostic host; only its exact launcher tree was terminated and the build
rerun successfully. The normal application stayed alive during these failures.

Portable import isolation, Windows system-DLL audit, embedded-host extraction
tests and packaging passed. Parent/submodule diff checks pass with existing
line-ending warnings. System Go cache retained. An old scheduler CSV was gzip
compressed and round-trip SHA256 verified before removing its original copy.

## Idle workload and measurement

Diagnostic-only wheel pattern:200ms burst each second,30ms repeat, then time to
finish the150ms animation and idle. Each run has20cold+20warm starts over40seconds.
Downloads186,Masonry152,screen160Hz,DPR1.75,defaultGC5000us,16MiB geometry pools,
hidden-ready admission fix. Same bounded material probes as preceding captures.
No build/test/analysis/UIA/scheduler trace ran during input. Capacities remain
unexhausted and actual-link/source hashes are recorded in capture identities.

The analyzer joins the first actual moving camera commit to its render bracket
and PresentMon call. It records intentional inter-burst idle separately; only
adjacent displayed frames within an active job contribute to animation pacing.
Movement threshold is one physical pixel. It does not equate producer and display
intervals or count idle as dropped frames.

| Metric | Before cold | Before warm | Verified after cold | Verified after warm |
| --- | ---: | ---: | ---: | ---: |
| First motion commit median ms |11.3109|11.2585|5.9822|5.1391|
| First motion commit maximum ms |17.9689|19.1661|8.6299|7.7440|
| First motion displayed median ms |29.5564|29.0268|18.2542|16.6206|
| First motion display samples |20|20|20|19|

The first post-fix run had first-commit median5.4374ms cold; its first warm
motion frame preceded the first captured PresentMon call. Added a one-second
consumer lead-in and repeated; the verified run still misses the first warm
presentation bracket. All40 commit timings remain measured, but only39 of40
first-display timings are established. This limitation is explicit in each job
and summary, not discarded silently. A consumer lead-in alone is insufficient
for an idle window; explicitly priming a rendered frame before input is the next
capture-infrastructure correction. Do not claim every startup display was covered.

The39 covered after jobs have zero displayed intervals>9.375ms and zero coverage
stalls. That threshold detects a missed refresh, not an exact6.25ms upper bound.
These limited samples demonstrate improved startup latency, not universal160Hz.
The earlier cold displayed miss remains unresolved evidence. Native motion is
still disabled by default; broader interaction and rollout verification remain.

## Deployment

Direct worktree --gui=qt launcher, no override, extracted host hash matches:
9788d8f537d8ef1daa7b97c6bd9db9d91e760f649746cfd8c384ff339a64c100.
Launcher SHA256:
120cefc39cb36841c9a85ea513255d7edc19dc6b58bfbc0cfbe0129c29958657.
Responsive Qt window verified. Canonical checkout untouched. No commit/push.
The constant160Hz without spikes goal remains active and incomplete.

AI assistance has been used to create this output.
