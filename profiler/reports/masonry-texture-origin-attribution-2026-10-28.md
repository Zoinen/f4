# Cold texture-creation stalls occur in atlas-copy extraction

Two live captures identify the texture factory, rather than guessing from
dimensions. The render thread creates 132 and 136 destination textures through
Qt's atlas-copy extraction path. Their largest cold native creations take
6.7327 and 8.4474 ms, crossing the 6.25 ms frame budget. This establishes a
specific target, not a finished optimization. No production change is promoted.

## Observer fidelity

The host retains the initial-dynamic candidate, guarded geometry reuse,
bounded staging uploads and uniform capacity retention. It does not include
the rejected growth-headroom or atlas-aware shader candidates. An observation
patch tags the actual creation calls in Atlas::generateTexture,
Texture::removedFromAtlas and QSGPlainTexture::commitTextureOperations.

Extraction that still has a CPU QImage creates a plain texture first and may
allocate its native texture later. A render-thread-local set follows that
plain texture's lifetime so deferred creation receives the extracted-image tag;
its destructor removes the entry. Direct extraction after image release gets
a distinct atlas-copy tag. Ordinary plain-image creation and staging textures
remain separate. There is no added cross-device mutex. The set/hash/producer
instrumentation still has observer cost; it is not a performance optimization
and its cadence cannot be compared casually to earlier uninstrumented runs.

The uninstrumented plain-texture source rebuild matches all 42 executable
sections / 5656 bytes of its linked Qt object. The atlas source has the prior
matching 107 sections / 19894 bytes, with its compatibility receipt revalidated.
Both new source transformations reverse exactly. Native allocation arguments,
resource policy and image lifetime behavior remain unchanged.

For all recorded render-thread texture resources, attribution uses same-thread
containment of the creation timestamp in exactly one texture-producer bracket.
Slow native calls join by resource pointer, device and containing call interval
to exactly one resource record. No nearest-time or dimension-only association
is used. Staging creation has its own API identity. Unmatched factories remain
explicitly classified as other, not inferred to be panel images.

## Controlled validation

Three native rendered tests exercise ordinary image creation, immediate atlas
extraction while its QImage is retained, and delayed extraction after the atlas
has uploaded/released its image. Each checks the expected producer label and
rendered interior/exterior pixels, actual D3D11, actual DPR 1.75, physical origin
(14,14), and scene unit transforms. The delayed copy image was visually
inspected. The complete native suite passes 50 checks, no failures/skips.
Four import-isolation/compiled-host checks and the system-DLL audit also pass.
These validate the observer paths; they do not prove final portable rollout.

## Live captures

Downloads, 190 entries, Masonry density 152, actual 160 Hz / DPR 1.75. Two
fresh processes with separate initially empty application caches, two ten-second
rapid-wheel blocks each, standard media workers/hardware decoding. OS cache is
not flushed. Helpers retain output until their 45-second collection finishes.
Focus, input count, target PID, capacity, feature-use and zero reuse/upload-error
checks pass. No build, offline analysis or UI Automation observer runs during
the gestures. A memory sidecar is sampled after collection, not during input.

Entire recorded process, render-thread texture creation counts:

| Qt PID | Atlas page | Atlas-copy extraction | Extracted CPU-image | Ordinary plain-image | Staging | Other |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 83064 | 1 | 132 | 0 | 0 | 183 | 23 |
| 3280 | 1 | 136 | 0 | 12 | 212 | 23 |

Within cold input, atlas-copy extraction creates 94 / 110 textures; within the
next block, 38 / 26. Ordinary plain-image creation appears only in the second
capture's next block, with 12 creations; its largest recorded call is 0.2059 ms.
The other category includes a 16384x32 BGRA resource, but its owner is not
identified by these hooks and is not declared to be a glyph texture.

Largest cold atlas-copy destination creations:

| Capture | Dimensions / format | Native creation wall time |
| --- | --- | ---: |
| First | 249x249 BGRA | 6.7327 ms |
| First | 1043x787 BGRA | 4.9962 ms |
| Repeat | 544x304 BGRA | 8.4474 ms |
| Repeat | 118x257 BGRA | 3.8818 ms |

These times cover CreateTexture2D while running the extraction factory, not
GPU copy execution. Qt queues the copy after creating the destination. The
probe does not establish CPU-versus-wait attribution, GPU duration, lock
ownership, or that the earlier 17.4432 ms sample from a different executable
had this same factory. Its dimensions alone cannot retroactively prove that.

| Capture | Cold / next missed refreshes | Cold / next max display gap |
| --- | --- | --- |
| First | 13 / 7 | 18.7715 / 12.5945 ms |
| Repeat | 10 / 3 | 18.7642 / 12.6311 ms |

Misses use the existing 9.375 ms threshold. Coverage holds are zero, but all
blocks fail the complete motion requirement. This is attribution, with no
disabled-observer arm or statistical cadence claim. Presentation-derived
motion is not physical scanout measurement.

## Next experiment

The source confirms that the default atlas transient-image threshold is zero:
after upload, the atlas drops the CPU image, so later shader sampling that
cannot use the atlas takes the copy-extraction path. A controlled retention
experiment can distinguish the GPU-copy dependency from the remaining native
texture allocation. It must measure memory and image throughput as well as
cadence; retained images are not free, and this still creates a destination.

Direct atlas sampling is another way to remove that destination, but the
previous atlas-aware shader regressed preparation and recording p99 in all
eight restored-display blocks, with 65 versus 7 misses in the reverse pair.
Those results remain valid evidence. Any reconsideration under the newer
backend policies needs fresh interaction measurements, not promotion based on
the new provenance counts alone. Do not blindly reintroduce that shader.

Sources, compatibility/build receipts, native/import/DLL logs, three rendered
images, compressed capture packets, target-PID PresentMon rows, memory sidecars
and analyses are frozen in the matching experiment directory with hashes.
Diagnostic process trees were stopped; the normal portable launcher and Qt
child remain alive and unchanged. No embedded-payload replacement, commit or
push. Constant 160 Hz remains unachieved.

AI assistance has been used to create this output.
