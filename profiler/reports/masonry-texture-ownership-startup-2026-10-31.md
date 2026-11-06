# Texture ownership and startup geometry, 2026-10-31

The diagnostic startup geometry changed Qt's texture allocation behavior. Earlier
small-window-then-maximize traces remain real measurements of those diagnostic
runs, but their standalone-texture stalls are not established as representative
of the normal maximized app. The constant-160-Hz goal remains unachieved.

## Method and correction

The normal app restores maximized geometry before its first visible frame.
The earlier recorder used an initially small window, then maximized it after
renderer initialization. Qt chooses its atlas dimensions from the initial surface
size. The ownership probe measured a 2048x1024 atlas with that earlier setup.

Copied only the normal MainWindowGeometry settings group into the owned
diagnostic profile, preserving the normal settings file. With the same probe
binary, startup then created a 4096x4096 atlas. No atlas environment override,
resource policy change, Qt SDK edit, production code change, or normal app restart
was made in this investigation. This observation establishes a startup mismatch;
it does not establish atlas size as the cause of every historical spike.

All current captures used Downloads, 192 entries including parent, Masonry
density 152, actual DPR 1.75 and 160-Hz display. Per-input guards retained these
conditions. No foreground screenshots or UI Automation polling ran during input.
No external processes, VMs, affinities, or workloads were changed.

## Ownership evidence

The diagnostic binary observes plain-texture creation/destruction, shared-atlas
creation and atlas extraction. It uses the current decoded pixmap's source URL,
not a possibly pending requested URL. Existing image keys associate allocations
with source metadata. It reads existing texture-provider and default-factory
fields without invoking textureProvider(), decoding, or requesting another image.
The event buffer and ownership traversal have explicit limits and fail on
exhaustion. Both completed v2 captures have zero pending event records and no
capacity exhaustion.

| v2 capture | Startup atlas | Plain allocations during input | Atlas extraction allocations | Unique source identities |
| --- | --- | ---: | ---: | ---: |
| retained-source-a2 | 2048x1024 | 69 | 18 | 87/87 |
| production-sized-b1 | 4096x4096 | 0 | 77 | 77/77 |

In the smaller-startup run, 78 creations belonged to managed thumbnails and nine
to qrc sources. In production-sized-b1 all 77 extraction allocations belonged to
managed thumbnails; the longest measured QRhi creation was 0.2895 ms. That is the
creation-call duration, not the entire copy/upload cost. It does not reproduce or
explain the historical 9.3774-ms standalone creation stall. No claim is made that
such a stall can never occur in a normal app or another folder.

Ownership collection traverses items and constructs metadata after synchronization;
it is intrusive attribution instrumentation, not performance acceptance evidence.
The first v1 capture completed, but its wrapper failed on cp1252 decoding of UTF-8
metadata. Its data and failure are retained; UTF-8 postprocessing recovered it
without repeating input. V2 also corrects requested-versus-retained source
attribution. Prefer v2 for ownership conclusions.

## Main-observer-only timing repeats

Removed the ownership probe and used the existing production-object recorder
(main observer only) with the corrected maximized startup and current packaged Go
binary. Both runs were 30 seconds. These are sequential first/repeat captures,
not randomized A/B proof or a verified cold-cache/warm-tour comparison.

| Capture | Wheel events | Input-window present rows | Dropped rows | Display intervals >9.375 ms | Maximum display interval | Render range p99 / max |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| production-sized-clean-c1 | 2671 | 4798 | 2 | 5 | 18.7718 ms | 2.83094 / 7.4937 ms |
| production-sized-clean-c2 | 2716 | 4798 | 0 | 0 | 6.5218 ms | 2.78218 / 4.1950 ms |

All dropped rows and display outliers remain in the results. Display intervals
refer to PresentMon's display-change notifications, not optical scanout proof.
Render signal ranges and whole-frame ranges are separate measurements; the latter
includes pacing/waits and must not be summed with render timings.

Neither capture verifies the original all-192-entry quiescent warm tour, cold
cache, physical motion error, coverage holds, or quarter-window middle-button
motion. A clean repeat with zero missed display intervals does not satisfy the
full goal. The ownership-probe production-sized-b1 capture itself had four long
display intervals and is retained separately, not substituted for clean timing.

## Identities, preservation, and next investigation

Normal app left alive: Go PID 9252 and Qt PID 106980, verified against
artifacts/normal-qt-current-identity.json at finalization. Normal Go SHA-256:
ac68c2e7c2fd259b760f91fc7c001be26d77ec2b33c86c36d1f22893a53fe613.
Normal Qt SHA-256:
653cbd8a9e6b416e1d2af88f88e34e353edcb4a18e7a3fb9d7ce32d6152a949b.

V2 ownership host SHA-256:
cd7d189440a3535931bdd47ce3d05400a75dd366cd1ffce0a1d914b384911932.
Clean recorder host SHA-256:
36544272ede9acef2333d20f61f18a50fc6fec1937acebcd81a41929b8a6e1c1.
Both use current packaged Go bytes. The ownership host replaces main, plain and
atlas objects for observation; its inherited original receipt label claiming
only a main replacement is corrected in a separate receipt addendum.

Compact scripts, receipts, scope guards and analyses are frozen beside this
report, with SHA-256 references to raw capture JSON/CSVs and binaries. Original
files and failed runs are preserved. Only owned diagnostic process trees were
terminated. No commit or push was performed.

Next, repeat scheduler/resource attribution with maximized startup. Previously
observed dynamic-buffer updates, resource-binding copies and vector-gate work
remain candidates, but none is established as the cause of the current remaining
display misses. Atlas-aware sampling may remove redundant copies; these corrected
measurements do not justify presenting it as the main spike fix yet.
