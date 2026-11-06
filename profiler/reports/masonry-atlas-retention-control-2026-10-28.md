# Retaining source images removes atlas copies but not allocation stalls

Keeping Qt's atlas source images eliminates the observed GPU-copy extraction
path, but cold scrolling still spikes. Destination texture creation reaches
9.3012 ms even with no atlas-copy extraction. Two retained-image runs have
more cold missed refreshes than both controls. This configuration is not
promoted; constant 160 Hz remains incomplete.

## Comparison and validity

All four arms use the identical texture-origin diagnostic host. No native
binary is rebuilt. Guarded geometry reuse, initial dynamic geometry, bounded
staging uploads, uniform capacity retention and the origin observer remain
enabled in every arm. Only QSG_ATLAS_TRANSIENT_IMAGE_THRESHOLD changes:
2147483647 for retained images, explicitly zero for the control. The host
receipt is validated before each launch and the exact environment is saved in
each build-identity sidecar. The new launcher has its own worktree artifact
path and still points to the same verified host.

Source inspection established that Qt's default threshold is zero; an atlas
normally releases the QImage after upload. The retained arm keeps that image,
so extraction creates and uploads a plain texture instead of allocating a
destination and queuing a GPU copy from the atlas. Destination allocation is
still present. This is a controlled dependency experiment, not direct atlas
sampling and not a product setting change.

Each capture asserts more than 50 expected render-thread extraction producers
and zero opposite-path producers. Both retained runs pass with no atlas-copy
producers; both controls pass with no extracted-CPU-image producers. This
verifies actual path selection, not just requested environment variables.
The host's previous 50 native checks cover all three image-factory paths,
pixels and actual 175% scene geometry; four import checks and the DLL audit are
inherited verified evidence, not newly rerun test counts. No UI layout or
renderer policy changes in this experiment.

Actual C:/Users/xs/Downloads, 190 entries, Masonry density 152, actual
160 Hz / DPR 1.75. Retained/control/control/retained, fresh processes with
separate initially empty application caches, two ten-second rapid-wheel
blocks per arm. Standard media workers and hardware decoding remain enabled;
OS cache is not flushed. Capture helpers retain stdout until 45-second
collection completes. Focus, input count, target PID, recorder capacity,
feature-use and zero reuse/upload-error checks pass. No builds, offline
analysis or UI Automation observer run during input. Memory is sampled after
collection, with no process-memory polling during gestures.

## Results

| Arm / Qt PID | Cold / next missed refreshes | Cold / next largest display gap | Last cache entries |
| --- | --- | --- | ---: |
| Retained / 67928 | 12 / 0 | 25.0139 / 6.4621 ms | 208 |
| Default / 30912 | 4 / 4 | 12.5656 / 12.5460 ms | 204 |
| Default repeat / 56128 | 5 / 1 | 13.6385 / 18.7541 ms | 210 |
| Retained repeat / 33828 | 10 / 0 | 12.5225 / 7.7175 ms | 209 |

Misses use display gaps above 9.375 ms. Coverage holds are zero. Both retained
next blocks have zero missed refreshes but maximum presentation-derived step
errors of 2.5927 and 25.1742 physical pixels, above the two-pixel limit. All
four captures fail the complete no-spike/motion requirement. Cache counts are
stored-entry snapshots, not unique-file throughput or per-file latency.
Two ordered pairs and variable loading progress are not a statistical cadence
claim. This instrumentation does not prove zero observer effect or physical
scanout motion.

Entire recorded process, render-thread factory counts:

| Arm | Atlas-copy extraction | Retained-image extraction | Ordinary plain image | Staging texture |
| --- | ---: | ---: | ---: | ---: |
| Retained | 0 | 126 | 13 | 209 |
| Default | 136 | 0 | 16 | 185 |
| Default repeat | 138 | 0 | 1 | 203 |
| Retained repeat | 0 | 124 | 15 | 211 |

There is still one atlas page and 23 other texture creations in each recorded
process. Retention switches the extraction factory; it does not eliminate its
native allocation count. The source uses implicit QImage sharing, but this
experiment does not isolate image-sharing memory or establish a product bound.

The first retained cold block's largest native destination creation is
9.3012 ms for a 153x273 BGRA image, with further 5.4905 and 3.0112 ms
creations. This directly rules out the queued atlas-copy dependency as the sole
cause of native allocation tails. It does not prove that copies have no cost.
The repeat's largest retained-image creation is 2.8348 ms; an ordinary plain
image creation takes 3.7654 ms. Geometry DISCARD mapping separately takes
4.0521 ms while completion is unproven. Controls still record atlas-copy
destination creation at 7.4620 ms and an unchanged constant-buffer Map at
6.9419 ms. Wall-time observations do not attribute CPU, GPU or lock ownership.

## Memory and decision

| Arm | End private memory, MiB | Process peak pagefile, MiB |
| --- | ---: | ---: |
| Retained | 1212.6 | 1958.6 |
| Default | 1201.0 | 2047.3 |
| Default repeat | 1220.0 | 1907.7 |
| Retained repeat | 1168.6 | 2117.6 |

These whole-process samples are mixed and do not establish either reduced
memory or no retention cost. Images remain alive with their atlas textures;
the configuration does not supply a new aggregate retention bound. The second
retained peak is larger than either default peak in these runs. GPU memory and
image throughput are not isolated by these samples.

Leave the normal threshold unchanged. The next meaningful target is avoiding
the duplicate destination allocation itself, rather than merely replacing a
GPU copy with an image upload. The previously rejected atlas-aware shader is
not automatically rehabilitated: any reconsideration must explain/test its
preparation/recording regressions under the newer backend policies and retain
pixel, memory, loading-throughput and whole-scene motion requirements. Other
independent stalls, including constant-buffer maps, remain unresolved.

Inputs, same-host identity, all four capture packets, target-PID PresentMon
rows, memory/method sidecars and analyses are frozen in the matching experiment
snapshot with hashes. The diagnostic process trees are stopped. The normal
portable launcher and Qt child remain alive and unchanged. No embedded payload
replacement, commit or push.

AI assistance has been used to create this output.
