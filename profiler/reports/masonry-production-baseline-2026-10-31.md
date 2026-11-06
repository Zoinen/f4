# Masonry: production-graph baseline, 2026-10-31

The 160 Hz goal remains incomplete. A diagnostic executable now links the
actual production CMake objects and libraries, replacing only main.cpp with an
otherwise unchanged copy that attaches a frame recorder and scripted wheel
input. Earlier experimental renderer, controller, image decoder and model
object overrides are absent. The normal portable worktree application was
neither replaced nor stopped during these measurements.

## Recorder and scope

The recorder adds direct QQuickWindow frame-signal timestamps to a bounded
100,000-row buffer. It reads contentY only after synchronization, while the GUI
thread is blocked. It writes its frame data after input stops, with no UI
Automation, screenshots or command-file polling during the timed interval.
Wheel-event receipts are collected on the GUI thread. PresentMon 2.6 records
presentation separately. Signal ranges include time spent descheduled; they
are not exclusive CPU measurements or GPU durations.

The measured left panel contains the actual Downloads folder's 192 entries,
including the parent entry. The private test profile uses the normal Go session
setting GalleryDensityMasonry=152. The window is maximized on the local 3840 by
2160, 160 Hz display at DPR 1.75; its client size is 2194 by 1186 logical pixels.
Both test panels contain Downloads. Interface icons and native SVG content are
preserved. The normal app retains its different right-hand photo directory.

The accepted setup samples record density 152, DPR 1.75, 160 Hz, active focus,
Masonry mode and 192 entries on every wheel input. These runs are initial and
repeat wheel baselines, not a verified cold cache, all-entry warm tour, or
quarter-height middle-button acceptance run. They do not establish physical
motion error, zero coverage holds or pixel-grid correctness. Those original
gates remain required.

The timer requests wheel input every 10 ms but actual delivery is slower:
median intervals are 11.70/11.67 ms and p99 26.51/24.24 ms. This delivery
limitation is retained rather than described as a true 100 Hz input stream.

## Setup regression found and corrected

Setting density only in QML did not update the Go core's state. A later normal
semantic update restored 150. The earlier a1/a3 traces have no per-input
density samples and are excluded from density-152 conclusions. Their analyses
now explicitly mark maintained density as unverified. The failed a2 run ended
before usable data; its cause was not captured. The new guard rejected a4 with
active=true, density=150, DPR=1.75 and refresh=160. All these attempts remain.

Only the owned private diagnostic processes were stopped. Their session.ini
was backed up, then configured through the ordinary GalleryDensityMasonry key
in the saved panel/workspace sections. The recorder now samples density and
display state on every input, and a scope rejection saves a receipt instead of
terminating the diagnostic host. No product persistence rule was bypassed.

## Measured results

Both a5 and a6 use the same diagnostic process and host build. Each input
window lasts 30 seconds. Every displayed outlier and dropped-present row is
retained in the raw CSV; display statistics cover displayed rows only.

| Measurement | First pass a5 | Immediate repeat a6 |
| --- | ---: | ---: |
| Accepted, foreground wheel inputs | 2668 | 2746 |
| Present rows inside input window | 4784 | 4798 |
| Dropped present rows | 5 | 1 |
| Display interval p99 | 6.364 ms | 6.316 ms |
| Maximum display interval | 25.003 ms | 12.474 ms |
| Display intervals above 9.375 ms | 16 | 1 |
| Synchronization p99 / maximum | 0.905 / 6.280 ms | 0.797 / 1.037 ms |
| Rendering signal range p99 / maximum | 3.595 / 18.050 ms | 2.812 / 4.828 ms |
| Input handler median / p99 | 0.234 / 3.052 ms | 0.235 / 2.987 ms |
| Whole frame signal range maximum | 22.719 ms | 10.081 ms |

All sixteen first-pass display misses occur in the first five seconds.
At 1086.9 ms the longest frame contains an 18.0499 ms rendering range and
lasts 22.719 ms overall. The next long frame includes 11.666 ms between
afterRendering and afterFrameEnd, while PresentMon's maximum Present API time
in this run is only 0.3067 ms. That tail cannot be attributed solely to Present.
One early synchronization range also reaches 6.2796 ms.

The repeat's one display miss occurs near 25.9 seconds. Its containing native
frame lasts 7.5053 ms, with 2.6478 ms in the rendering signal range. PresentMon
reports 3.3619 ms until render completion and 21.7949 ms until display. These
observations do not establish an exact DWM/driver cause or physical scanout
timing; that needs a corresponding driver/scheduler trace.

## Implication and next measurements

The initial/repeat difference points toward first-encounter content work as
the next profiling target. It does not prove which renderer, decoder, shader,
allocation, upload or scheduling call is responsible. Capture CPU stacks and
driver/scheduler events for the production graph's first scrolling seconds,
then isolate the expensive calls before porting any experimental optimization.
Separately trace the repeat's residual presentation miss.

Add the original verified all-192-entry warm tour and cold-cache setup to this
small recorder, followed by quarter-height middle motion and native/pixel
motion checks. Keep the display threshold 9.375 ms, physical motion limit 2 px,
zero coverage holds and middle velocity tolerance unchanged. A passing p99 or
better repeat average cannot replace any of those gates.

## Evidence and live application

Working evidence is in artifacts/production-recorder-20261031. Its
build-receipt.json identifies the single main-object substitution and hashes
the current host, original main, generated main, recorder header and link RSP.
The host SHA256 is
cf6b10c1bdf2575c54531c36373cf1755be746995dc7a507a5ab9961b720bdb4.
The original portable host remains
cd01b63ff07e92951d2ffcebe65e04a7ecb946f9c539d54c9195a4f0478eb63b.

Raw frame JSON, complete and target PresentMon CSVs, analyses, containing-frame
attribution, scope failures and sources are retained. No raw trace, outlier or
failed attempt was deleted. Earlier intermediate diagnostic executables were
rebuilt in place; earlier tool outputs identify historical hashes, but their
binary bytes are not preserved. The final measured a5/a6 executable is retained.

The owned diagnostic process tree 69760/86280 was stopped after a6. The normal
worktree app remains Go PID 103648 and Qt PID 17748, with unchanged hashes,
restored foreground focus, Downloads left and its existing photo directory
right. No external workload, VM, OS affinity or desktop setting was changed.
No commit or push was performed.

AI assistance has been used to create this output.
