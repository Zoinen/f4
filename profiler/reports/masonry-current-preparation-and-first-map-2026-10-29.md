# Current preparation stages and a newly adopted buffer's first map

The constant-160-Hz goal is not complete. This is a source-preserving diagnostic,
not a promoted rendering change. The raw owner memo and uniform backing rotation
are disabled. No production recipe, embedded payload, commit or push changed.

## Current-source probe and checks

The existing reversible 13-mark preparation probe was applied to the renderer
actually linked by the latest uniform-capability-query diagnostic, including
the initial-dynamic-buffer change. Reversing all substitutions restores that
exact source. An additional copy of the current SDK node updater counts calls
and ancestor steps in `isNodeBlocked`; no timestamp is taken per node.
The probe records thread identity and cumulative per-thread node counters.
Markers bracket resource-batch acquisition, render lists, cleanup, batching,
sorting, gate setup, upload/admission, opaque/alpha material preparation,
visualization, resource updates and gate completion. Public frame signals also
cover work before and after the marked body. These are wall-clock scopes,
not scheduler or GPU execution measurements.

Verification at actual DPR 1.75:

- Six probe tests pass: counter snapshots, monotonic marks, incomplete-row
  exclusion/overflow, node-walk fields, thread identity and thread isolation.
- Native renderer tests: 48 pass, zero failure/skip.
- Shader/readback tests: 27 pass, zero failure/skip.
- Import isolation and compiled-host QML loading: four pass.
- Static Windows system-DLL audit: pass.

The per-frame phase sums are checked against the enclosing public preparation
interval. Every analyzed frame has exactly one complete 13-mark renderer pass;
42,211 probe rows fit the 262,144-row capacity. The constructor mark's timing is
not used to explain a different frame, and old scheduler attributions are not
transferred onto this capture.

## Accepted cold/loading capture

`real-scroll-scene-preparation-cold-20261029-wheel-masonry`: actual 3840x2160,
160 Hz, unforced DPR 1.75, Downloads, 191 entries, Masonry density 152, isolated
empty cache `cold-gallery-cache-20261029-e`. Two ten-second boundary-bouncing
wheel blocks at a nominal 10-ms interval. 1676 inputs pass the count/focus gate;
display, source, build identity, probe use and buffer bounds also pass. No build,
offline analysis or UIA observation ran during gestures.

Both blocks are loading/reverse-scroll observations. They are not asserted to
be fully warmed: the final loading sample still has four pending requests.
Nearest-prior samples at the largest frames show 54 and 19 pending requests,
respectively. Those samples are periodic, not exact queue-depth measurements
at the frame timestamp.

| Block | Preparation median / p99 / max ms | Display misses | Maximum display interval |
| --- | --- | ---: | ---: |
| 0 | 0.8121 / 3.8589 / 12.9523 | 13 | 18.7568 ms |
| 1 | 0.6773 / 3.3963 / 10.8862 | 2 | 12.5105 ms |

Both fail the unchanged maximum-two-physical-pixel motion gate (135.65 / 91.67 px).
No coverage hold was recorded. This is not a successful 160-Hz capture.

## Exact same-frame breakdown

The 12.9523-ms block-0 preparation span contains:

- uploads/admission: 10.9744 ms;
- alpha material preparation: 1.5150 ms;
- work before the marked body: 0.2222 ms;
- gate setup: 0.1022 ms.

An actual geometry `Map(WRITE_DISCARD)` spans 8.1861 ms inside uploads (outer
backend wrapper 8.1902 ms). The frame has two geometry changes, 63 removals,
three blocked notifications and 297 ancestor-check calls / 6427 steps. Nearly
all ancestor checks occur before the marked body. The constructor/owner-lookup
cost is not the dominant delay in this particular slow frame.

Another 8.3509-ms preparation span includes 5.4947 ms uploads and a 4.5529-ms
native buffer creation wrapper. A further 8.2217-ms preparation frame includes
7.1984 ms uploads and a 6.9221-ms geometry DISCARD map.

Block 1's 10.8862-ms maximum is distributed: 3.0329 ms uploads, 2.2690 ms before
body, 2.2413 ms alpha materials and 1.5158 ms render lists. Its longest recorded
RHI call is only 0.0654 ms. A separate 7.4619-ms frame includes 2.3974 ms render
lists and 1.2587 ms batching. This capture does not contain scheduler data to
separate execution and waiting for these specific intervals; it does not prove
that every remaining delay has the same driver cause.

## Native lifetime/descriptor attribution

The 8.1861-ms DISCARD map uses native pointer `c7d9e060` on render thread 36168.
The captured resource history identifies:

- worker `buffer.prewarm.CreateBuffer`: timestamp 975186784114200 ns;
- adoption through the backend `create()` wrapper: 975186795090400 ns;
- map start: 975186795165600 ns.

Both resource records describe a 2,097,152-byte dynamic, exclusively vertex-bound
buffer. The worker creation precedes adoption by 10.9762 ms; the map begins
0.0752 ms after adoption. The backend wrapper records adoption as a create event,
so it must not be mislabeled as another native driver allocation. The true
worker record is separate. This identifies a newly adopted pool buffer rather
than an arbitrary long-lived geometry buffer.

The separate 4.5529-ms create wrapper's resource `45dbc3a0` is dynamic,
exclusively index-bound, **1092 bytes**. It is not the multi-megabyte vertex
allocation. The worker pool reports 1160 adoptions / 182 misses in this process,
with 24.25 MiB retained below its unchanged 32-MiB bound. Increasing the memory
bound without distinguishing entry count, classes, refill scheduling and driver
locking is not justified by these observations.

## Next narrowly testable hypothesis

The current completion guard applies to the whole context and does not prove
that an individual newly adopted buffer has never been exposed to queued reads.
Consequently its first full geometry update can use DISCARD even when the native
buffer has just left the fresh pool. A first-use proof could potentially avoid
that rename path without rotating any already-used backing buffer.

Before changing policy, cover lifetime registration/retirement, full-update
semantics, queued vertex/index binding and reads, resource updates/copies,
native export/external access, wrong-thread/capacity failures and pointer reuse.
Unknown or consumed first-use proof must fall back to the existing guard.
Actual D3D11 rendered/readback tests must preserve previous queued draws. Then
measure matched cold captures: the current trace does not prove that a fresh
NO_OVERWRITE map avoids lazy native allocation or driver contention. Worker
creation of initialized backing, if considered later, is a separate experiment.

Verified fully warmed comparisons also remain required. Pending-zero at one
viewport is not proof that an entire scrolling route is resident; do not freeze
loading, hide content, relax motion gates, or relabel an unfinished load as warm.

## Retained state

Two stopped launcher copies matching the preserved normal executable were
removed after exact-path, live-process and SHA256 checks, recovering 328243200
bytes. The removal manifest is retained; no host, source, trace or build evidence
was deleted. This avoided another disk-full build without removing unique work.

After collection only the exact scene-preparation launcher tree was stopped.
The normal launcher PID 43936 and extracted host PID 119460 remain alive.
The snapshot contains sources, build/test receipts and measured data; binaries
and large raw records remain external artifacts with hashes. No build rollout
or claim of cross-platform release verification is made.
