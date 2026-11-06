# Fixed-input scrolling exposes catalog reset churn

The physical 3840×2160, 160 Hz display returned after the existing session was
transferred from RDP to the unused console. Two fixed-input cases ran with
sampled native foreground and DPR 1.75, both panels entering Downloads and a
192-entry final catalog. Input deadlines were anchored at navigation +200 ms.
Both cases skipped input slots and therefore fail full-workload acceptance.

| Metric | Control `cold-o161` | Handoff candidate `cold-o162` |
| --- | ---: | ---: |
| First input after navigation | 203.001 ms | 242.171 ms |
| Skipped slots out of 3,000 | 161 | 133 |
| Coverage holds | 172 | 127 |
| Last hold relative to planned start | 2,270.027 ms | 1,023.665 ms |
| Maximum recorded display interval | 12.519 ms | 62.505 ms |
| Admitted wheel model pairs | 4,100 | 4,108 |
| Maximum camera-versus-curve error | 307.591 px | 287.708 px |

The control delivered three events before the full catalog was available,
with 48 entries visible to the model. The candidate's first event was delayed
by 42.171 ms from the planned start. Its two long display intervals ended at
presents 43.530 and 81.734 ms after the planned start; the longest interval
includes time before the planned start. They are retained as entry/early-input
gaps, without assuming all such intervals occurred during animated motion.
The candidate's measured render signal range was at most 4.495 ms. This does
not explain the gaps or prove which thread was on CPU in them.

The curve probe joined every recorded motion sample. On the admitted pairs,
the requested trajectory matches the captured curve model; the camera falls
behind when coverage constrains it. With pairs that have an after-sample hold
removed for diagnosis only, display-clock model p99 is 0.328 / 0.463 px and
maximum is 2.951 / 2.153 px. Holds are still included in acceptance. Retargets,
job changes and segment changes remain excluded and unverified; this is scene
matrix evidence, not an optical measurement.

## Catalog application and canceled row work

The new `catalog_reset_attribution.py` joins row lifetimes to enclosing
`ExternalCatalogModel::applyCatalog` spans by their captured timestamps. It
preserves unmatched or ambiguous events and rejects incomplete traces. Three
tests cover queue state, ambiguity and overflow.

All 258 control and 226 candidate `cancel-all` events in the first 500 ms of
planned input join to catalog applications, with none unmatched. Respectively,
255 and 222 were queued and had not started; only 3 and 4 had started. Catalog
application shapes include 48 incoming / 48 prior logical entries, 48 / 192,
and 192 / 192. The 48 / 192 applications account for 92 and 93 canceled
requests respectively. The trace confirms repeated catalog-driven cancellation;
it does not identify catalog revision/identity equality or prove those
applications are unnecessary. Avoid treating cancellation counts as elapsed
construction or CPU time.

Next, distinguish same-revision page-shaped updates from genuinely new
catalogs. The existing same-revision fast path compares storage shape as well
as revision and row count. A regression should prove retained identity,
metadata and cursor behavior before changing that rule or retaining any queued
root across a reset.

## Interrupted and changed-scope cases

- `cold-o163` never released foreground or navigated. RDP reattached, its
  controller timed out, and a later process check found both owned processes
  gone. It was preserved and never restarted. The old display lease process
  exited when its original display disappeared; it did not verify restoration.
  The physical display was subsequently observed at 160 Hz again. No new mode
  change was made for `cold-o164` or `cold-o165`.
- `cold-o164` completed at sampled 160 Hz/DPR 1.75, but Downloads had changed
  to 194 panel entries. The 192-entry verifier rejected it. Its raw files and
  rejection are retained, and its exact diagnostic process tree was closed.
  No Downloads content was modified. A metadata-only fingerprint of the new
  corpus was recorded for a separate comparison.
- `cold-o165` is a separate 194-entry, verbose catalog diagnostic. It reached
  initialized setup, but Far reclaimed foreground after activation. Input
  was not released. Its process and controller state must be checked before
  attempting recovery; never restart this case solely because observation
  timed out. The verbose trace is for catalog identity attribution, not cadence
  comparison with uninstrumented cases.

The control/candidate comparison remains incomplete, and neither recorded
run meets the goal. The original cold/warm/middle-button and physical-motion
requirements remain pending. No production renderer change, commit or push
was made.
