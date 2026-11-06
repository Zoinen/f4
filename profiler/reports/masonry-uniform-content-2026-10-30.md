# Masonry native-text uniform content

## Finding

Byte-identical native-text uniform payloads recur across separate draws within
one renderer preparation pass. The two measured wheel windows queue 120405 /
120491 text updates, of which 78340 / 77298 (65.06% / 64.15%) duplicate payloads
already queued in that pass. A median pass queues 76 updates but has 27 distinct
payloads. This supports testing shared uniform storage while preserving every
draw and its order. It does not establish a whole-view speedup.

Whole capture: 259217 native-text queries and queued uploads, all eligible for
the bounded census. 166698 repeat an earlier queued payload (64.31%); 92519 are
distinct within their respective passes. Text accounts for 259217 of 931486
queued uniform updates overall. Payload deduplication alone would therefore
remove at most about 17.9% of this capture's total update calls; it does not
remove those draws, shader work or all binding work.

## Diagnostic and validation

Qt 6.11.1 diagnostic based on the uniform-material observer. A scope surrounds
`prepareRenderPass()`. After the original `updateUniformData()`, it classifies
merged native-text payloads on the queued-update path, up to 256 bytes. A fixed
512-entry table compares the full byte range after a 64-bit hash; hash collisions
cannot produce false matches. Queued and unchanged observations are counted
separately. Tables are scoped by nested preparation depth; keys never carry
across passes. The original GPU updates and shader/resource order are unchanged.
There is no per-draw timing or logging. This hashing/copying observer is
intrusive attribution, not an A/B performance comparison.

Eleven helper QtTest results pass: full-byte comparison under forced collision,
different lengths, queued-versus-unchanged observations, pass reset and generation
wrap, bounded capacity with hits retained after overflow, nested scope isolation,
and no payload read when the recorder is full, plus the existing storage tests.
All 44 native rendering results pass with actual DPR 1.75/D3D11, zero skips.
Three static import-isolation results and the Windows system-DLL audit pass.

The census records 3464 complete passes with zero key-capacity, depth, unmerged,
direct-update or oversized bypasses. Its queued total exactly matches the
independent material-class counter. Every selected measured pass runs on the
recorded render thread. Pass counts differ slightly from complete-frame counts
because their interval boundaries differ; they are not interchangeable.

## Capture and remaining gates

Case: `real-scroll-uniformcontent-a1-20261030-wheel-masonry`.
Actual Downloads, 192 entries, Masonry density 152, 3840×2160, 160 Hz/DPR 1.75,
new process and empty isolated cache, two ten-second wheel windows. Default
frame latency 2; sparse strategy disabled. The second direction is not a
verified-warm claim. Original capture-validity checks pass. PresentMon supplies
3459 target rows. All 1722 wheel events arrive active; native foreground
ownership is stable. No screenshots/accessibility polling run during gestures.

| Metric | First window | Second window |
| --- | ---: | ---: |
| Complete census passes | 1596 | 1591 |
| Queued native-text updates | 120405 | 120491 |
| Distinct queued payloads per pass, summed | 42065 | 43193 |
| Duplicate queued payloads | 78340 | 77298 |
| Display intervals above 9.375 ms | 28 | 43 |
| Maximum display interval, ms | 12.7440 | 18.7710 |
| Maximum motion error, physical px | 111.6611 | 168.6545 |

No constant-160-Hz claim follows. The finite display hold is released and the
owned diagnostic launcher tree stopped; the normal worktree application stays
alive. Diagnostic intermediate files were archived with SHA-256 verification
and their original paths preserved by a directory junction, as recorded in
`uniform-content-intermediate-relocation-20261030.json`.

## Follow-up prototype

`SharedTextUniform20261030.h` and its build/test generators implement an
unpromoted diagnostic prototype, disabled by default. It has a bounded content
book, immutable keys within the observed D3D11 frame epoch, ordinary QRhi
dynamic-buffer updates, renderer/RHI cleanup hooks and an unchanged fallback.
Using shared storage invalidates the original batch buffer so fallback must
refresh it. It does not alter glyph generation, pixels, draw order or media.
The frame epoch currently comes from existing diagnostic D3D11 frame tracking;
it is not a portable public Qt frame identifier. Native lifetime and pixel tests
and live A/B measurements are required before any rollout. The content census
does not by itself validate that implementation or prove a performance gain.

Evidence: `profiler/experiments/masonry-uniform-content-2026-10-30/`.
No production promotion, commit or push is performed.

> AI assistance has been used to create this output.
