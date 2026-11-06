# Masonry driver records and actual image displacement

## Result

Constant 160 Hz without spikes remains unachieved. This capture adds evidence
that four apparent refresh-counter jumps from timestamp association are not
missing F4 image IDs. It does not replace the original display or motion gates.
No application source, animation clock, normal binary or portable payload changed.

The preceding turn made progress by calibrating the unchanged displacement
estimator against known analytic fractional translations. This turn applied it
to actual gallery pixels while recording native frame markers, submission-only
PresentMon records and driver refresh events together.

## Workload and retained failures

Case `real-scroll-postsync-presentationdriverpixels-b1-20261031-wheel-masonry`:
the current marked diagnostic host, normal window, the actual Downloads folder,
all 192 entries, Masonry density 152, 3840×2160, 160 Hz and actual DPR 1.75.
The normal all-entry warm tour completed before both 30-second rapid-wheel
windows. All 5,380 wheel events report the window active. Existing host hashes
were checked before launch; no source graph or rendering flag was changed.

The bounded Desktop Duplication observer additionally copied a 16×384 physical
pixel strip at client position (640,360). Its 345,317,376 bytes stay below the
existing 384 MiB limit. It wrote after capture, not during scrolling. This
observer remains intrusive: copy/map medians were 1.4841 / 1.4967 ms and maxima
2.6188 / 2.6897 ms. A submission-only PresentMon session ran beside the original
display-tracked session; GPU duration tracking was disabled.

Two failures remain explicit:

- The original display-tracked PresentMon sample guard failed, with zero target
  rows in both measured windows. Submission records are a diagnostic source,
  not a substituted display pass.
- The separate sequential DxgKrnl trace reached its 256 MiB cap and its logger
  stopped. The original wrapper then failed its finally-block ownership assert
  because the logger was already terminal. The decoder reports 46 lost buffers
  and exits 3. It has no property, metadata or ProcessTrace errors, but it is
  incomplete and is not accepted as a lossless full capture. The first window
  lies within the available trace; the second is only partly covered.

Recovery independently checked that the named logger was absent and stopped
only the owned diagnostic application tree. The original failed wrapper and
data remain. Closed ETL, packet and pixel files were transparently compressed
with SHA256 equality before/after; no trace data was pruned.

## Image-motion evidence

| Quantity | Window 0 | Window 1 |
| --- | ---: | ---: |
| Produced marker frames | 4,800 | 4,801 |
| Unique marker/native-frame submission bracket joins | 4,800 | 4,801 |
| Captured desktop image rows | 4,800 | 4,800 |
| Identifiable image-displacement pairs | 2,827 | 2,895 |
| Image displacement vs recorded renderer displacement, median px | .09514 | .09999 |
| p99 px | .79324 | .80063 |
| Maximum px | 1.17201 | 1.09162 |

The remaining pairs are explicitly unmeasurable: window 0 has 1,701 low-texture,
178 ambiguous, 64 weak, 27 carried-pose and two no-motion-sample pairs; window 1
has 1,641 low-texture, 176 ambiguous, 56 weak and 31 carried-pose pairs. They
are not zero-error samples or passing motion checks.

Every measured capture row has a valid marker CRC, successful readback, the
correct foreground PID and AccumulatedFrames=1. Window 0 retains one repeated
ID and one step of two; window 1 retains one repeat. These are not removed from
the dataset. Consecutive produced markers join unique native render frames and
their exact Present submission brackets.

The unchanged matcher was separately calibrated for the actual 16-pixel strip
width against 600 analytic fractional translations: 525 matched, median error
.005909 px, p99 .051634 px and maximum .051994 px. The 75 high-frequency weak
matches remain rejected. This bounds the tested synthetic examples only, not
arbitrary gallery-image error.

Sampled strips were rendered and inspected: they visibly contain changing
gallery image and caption content. The desktop-global masked-content flag is
still one on every row; a visible sample strip does not establish that every
application pixel is unmasked. No capture protection was changed. Neither the
strip nor markers prove optical scanout.

## Driver association and the next decision

The display query identifies one active MSI MAG322UPF path, DISPLAY1, source 0,
target 200965, 160/1 Hz. All available VSyncDPC records selected by source and
target have one adapter pointer. Adapter-LUID rundown is absent from this
truncated trace; it is not claimed to provide the fuller mapping established
by the earlier independent Oct 29 trace.

The first window has 4,800 available driver-reported refresh records with 4,799
consecutive FrameNumber steps. Associating each desktop ImageQpc with the
preceding driver FrameQPCTime produces two steps of two and two steps of zero.
All four corresponding captured marker-ID steps are one. Thus this temporal
association alone is insufficient to identify the refresh containing a given
image; those four apparent jumps cannot be labeled F4 image skips from this
association. The available portion of window 1 has 1,214 driver records with
consecutive ordinals; the remainder is missing.

This makes the next action more specific: retain the original clean display
checks, and avoid fitting the animation clock to Desktop Duplication or DPC
notification timestamps. A lossless combined repeat needs a trace capacity
above the observed event volume and verified full-window coverage. Even then,
temporal association must not be promoted to physical image identity. Cold
loading, middle scrolling and product rollout remain separate outstanding work;
this warm intrusive diagnostic does not stand in for them.

## Evidence and final state

Sources and results are under `artifacts/presentation-driver-pixels-20261031`.
The complete packet, capture CSVs and pixel strip retain their original names
under `artifacts/modes-profile`. The recipe, recovery receipt, decoder metadata,
all temporal anomalies, calibration rows and final audit are retained.

The final audit confirms all owned capture processes and named trace sessions
are terminal. Normal Go PID 23488 and Qt PID 84780 remain alive. No external
applications, affinity policies, installed Qt SDK, commit or push were changed.

AI assistance has been used to create this output.
