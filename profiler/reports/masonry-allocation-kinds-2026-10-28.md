# Masonry buffer creation: conversions and growth are separate causes

Two fresh-process captures classify the remaining buffer creations without
changing resource allocation policy. Hundreds of cold-scroll creations convert
DEFAULT buffers to DYNAMIC at unchanged native capacity. The recurrent
2,054,448-byte vertex buffer instead grows with unchanged usage. Removing
conversions would not remove that growth allocation. Neither capture reproduced
the preceding combined experiment's 13.6489 ms allocation stall, and neither
meets the complete no-spike/motion requirement. No production change is promoted.

## Probe and verification

The observer derives from the verified combined geometry-reuse, staging-upload
and uniform-capacity executable. Immediately before QD3D11Buffer::create destroys
an existing native buffer, it obtains that buffer's descriptor. The actual
CreateBuffer call receives a producer bracket classified by old/new usage,
capacity and binding. Initial creation, promotion with unchanged capacity,
promotion with growth and same-usage growth are distinguished. There is no
preallocation, type substitution, pool or capacity change.

The source transformation reverses exactly to the combined source. The shared
recorder's structures are unchanged. Slow outer CreateBuffer records join by
same-thread containment to exactly one producer span and by pointer, device and
containing time interval to exactly one resource descriptor. Resource publication
happens after the inner producer scope; nearest-time attribution is not used.
All render-thread buffer creations also contribute producer counts, including
those below the 50 us slow-call threshold. Explicit destruction before create
would appear as initial creation; this is native-buffer provenance, not a full
logical QRhi object lifecycle history.

The relinked native suite passes 46 checks with zero failures/skips at actual
DPR 1.75, including rendered pixels and scene origins/unit transforms. Four
compiled-host/import checks and the system-DLL audit pass. The combined
component's earlier 5 geometry-safety and 56 staging checks are inherited
evidence, not newly rerun suites in this experiment.

## Live measurements

Actual Downloads, 190 entries, Masonry density 152, 160 Hz / DPR 1.75. Each
capture uses a separate initially empty application cache and two ten-second
rapid-wheel blocks. Default media workers and decoding remain enabled. Capture
output is retained until the 45-second helper exits; no builds, analysis or UI
Automation observer run during input. Focus, input count, target PID, recorder
capacity and combined-feature-use checks pass. OS cache is not flushed.

| Capture / block | Initial | Same-capacity promotion | Same-usage growth | Promotion + growth |
| --- | ---: | ---: | ---: | ---: |
| PID 62768, cold | 277 | 444 | 372 | 9 |
| PID 62768, next | 64 | 47 | 35 | 0 |
| PID 112432, cold | 313 | 344 | 348 | 1 |
| PID 112432, next | 32 | 25 | 9 | 0 |

Largest buffer creation wholly inside cold measurement windows is 0.4095 ms
in the first capture and 0.5215 ms in the second, both same-usage growth.
Observed 2,054,448-byte vertex growth takes 0.2637 ms and 0.3106 ms respectively;
the first capture's next block contains another at 0.0979 ms. These are separate
executions from the earlier 13.6489 ms sample: they establish the recurring
allocation's provenance, not retroactive proof of the older sample's lifecycle.

| Capture | Cold / next gaps >9.375 ms | Cold / next maximum display gap |
| --- | --- | --- |
| PID 62768 | 3 / 1 | 12.5076 / 12.5017 ms |
| PID 112432 | 2 / 0 | 12.4949 / 6.4423 ms |

Coverage holds are zero. The second next block has zero missed refreshes but a
maximum presentation-derived motion-step error of 2.5546 physical pixels, above
the two-pixel gate. Other blocks also fail. These repeats have no disabled arm
and different loading progress; they are attribution, not a comparative claim
of improved cadence or eliminated tails. Presentation-derived motion is not a
physical scanout measurement.

## Consequence

Same-capacity conversion is a concrete avoidable-allocation candidate. It needs
a controlled test against the combined candidate, with CPU shadow lifetime,
dynamic-map safety and memory effects checked. Growth requires a separate
retired-buffer selection/capacity investigation; blindly making everything
dynamic or widening every buffer would not establish a solution. Earlier
large-static-buffer and buffer-exchange experiments were rejected and must not
be silently revived. Small texture creation and unchanged constant-buffer maps
remain independent targets.

The matching experiment snapshot retains sources, build identity, verification
logs, compressed capture packets and target-PID PresentMon rows with hashes.
Diagnostic processes are stopped. The normal portable launcher and its Qt host
remain alive and unchanged. No commit, push or embedded-payload replacement.

AI assistance has been used to create this output.
