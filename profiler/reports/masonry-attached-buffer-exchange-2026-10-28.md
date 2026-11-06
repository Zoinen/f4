# Attached-buffer exchange works in regression but is unused in Downloads

A candidate checks the retired geometry-buffer pool before growing an attached
buffer. A best-fit dynamic buffer can replace the undersized attached buffer;
the smaller buffer goes back to the same pool. Pool cost is reduced by the
capacity difference, with no extra allocation, headroom, draw-length change or
pool-budget increase. Existing upload and GPU-completion rules remain intact.
The candidate is D3D11-only and opt-in in the diagnostic executable.

This mechanism is not promoted: the live capture has 487 growth checks but
zero fitting opportunities and zero exchanges. The feature-use assertion
correctly rejects the capture. There is no enabled/control cadence comparison
and no claim that this improves scrolling. Subsequent post-assertion checks
in that helper did not run; the rejected packet is not a fully accepted run.

## Regression and build verification

A real native fixture grows an attached 80,000-byte vertex buffer with a
100,000-byte retired buffer available, and separately grows a 24,000-byte index
buffer with 30,000 bytes available. The original renderer fails both expected
pointer-selection assertions. The candidate selects the existing larger buffer,
returns the smaller buffer to the pool, verifies pool cost/count, and retains
the selected capacity on the following growth. Pixel reads and saved output
show the same red geometry on black at actual DPR 1.75. The scene origin maps
to physical (14,14), with unit scene transforms. No UI layout changed.

The complete native candidate suite passes 49 checks with zero failures or
skips. Embedded import isolation passes four checks, and the Windows dependency
audit passes. Before testing, the inherited test link was repaired to use the
newly compiled regression fixture instead of the older counter fixture; the
repair script and receipts are retained. No results from the wrong fixture
are used.

## Growth sizes explain the lack of pooled capacity

A separate bounded observer records old/requested capacities and pool count,
leaving exchange disabled. Atomic ready flags publish completed records to
the snapshot reader; storage is bounded to 8192 records. It builds from the
same candidate and does not change allocation sizes or map policy.

The accepted capture uses real Downloads (190 entries), Masonry density 152,
actual 160 Hz / DPR 1.75, fresh application cache bh, and two ten-second rapid
wheel blocks. Import isolation and dependency audit pass for this observer.
Its complete-process growth sample contains 366 records, all below capacity:

| Requested capacity / old capacity | Growth requests |
| --- | ---: |
| At most 1.25 | 91 |
| Above 1.25, at most 2 | 101 |
| Above 2 | 174 |

306 requests are at most 8192 bytes; five are at least 1 MiB. The largest
request jumps from 1680 to 2054448 bytes. Other large jumps include 25600 to
1344420, 6480 to 1344420, and 4160 to 1136916 bytes. Only ten growth checks
see an empty pool, yet none of all 366 checks finds a qualifying fitting dynamic
buffer. Pool count alone is not proof of suitable capacity. These counts cover
the whole process, including startup, not just gesture windows. The 258 distinct
QRhi pointer values do not prove 258 unique lifetimes because addresses can
be reused. The renderer default pool budget is already 16 MiB per vertex/index
pool; these data do not justify raising it.

This supports avoiding another modest headroom experiment. It does not prove
the ratio or cost distribution of all application buffers, and size alone does
not identify the SVG or ordinary batch that owns an allocation. Preparing
large allocations before their first visible render frame remains a candidate
direction; it has not been implemented or verified by this experiment.

All diagnostic trees were stopped by exact worktree launcher path. The normal
app remains unchanged, PID 43936 and embedded host PID 119460. No portable
payload replacement, commit or push occurred. Constant 160 Hz remains incomplete;
the cold creation/unfinished-marker DISCARD tails and strict motion gates remain.

The matching experiment snapshot retains source, test evidence, rendered
fixtures, the rejected feature-use run, the accepted growth-size run, and hashes
of external executable/object artifacts.
