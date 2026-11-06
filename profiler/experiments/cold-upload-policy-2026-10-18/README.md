# Cold-upload policy experiments (not shipped)

Both renderer patches apply independently to Qt 6.11.1 after the integrated
stable-interval/batch-retention patch, using ordinary `git apply` from the
qtdeclarative directory. They do not compose with each other for the measurements
reported here.

- `large-dynamic.patch` changes only initial vertex/index allocations of at
  least 64 KiB on D3D11 to Dynamic. It is rejected because two clean pairs do
  not establish a scrolling improvement.
- `single-unmerged.patch` retains unchanged single-element, permanently
  unmergeable batches when drawing-order numbering changes, including a full
  list rebuild. It does not transfer buffers between batches. Its native tests
  pass, but two clean pairs do not establish a cold-scroll improvement, so it
  is not integrated.
- `native-regression.patch` applies to parent `87258b8af`. It includes the
  integrated retained-renderer fixture and the new single-element order-change
  regressions. Do not apply it on top of an earlier native fixture patch.
  The new rows fail against the normal stable-interval renderer and pass against
  `single-unmerged.patch`. It deliberately gives the two overlapping unmerged
  rectangles different material colors so Qt keeps them in separate batches.

The normal recipe, imported Qt library, embedded payload, application binary,
and native test source keep the previous integrated renderer after this trial.
See [the report](../../reports/cold-upload-policy-experiments-2026-10-18.md)
for attribution, all displayed-frame pairs, and the remaining mechanism needed.
