# Shared native-text uniform prototype

## State

Implemented and built an unpromoted diagnostic prototype after the content
census found approximately 65% duplicate queued native-text payloads within
each preparation pass. It preserves every draw and the existing overlap
ordering. It is disabled by default; explicit diagnostic flag
`F4_PROFILE_SHARED_TEXT_UNIFORMS=1` enables it.

There is no live A/B performance result yet and no claim of constant 160 Hz.
The preceding census run still fails the original display and motion gates.
The regular worktree app remains alive. No embedded payload, Qt SDK, production
renderer, commit or push is changed by this prototype.

## Mechanism and bounds

Each renderer owns a bounded pool of at most 128 QRhi dynamic uniform buffers,
each 256 bytes. A full-byte content book maps payloads to distinct pool slots
within an observed D3D11 frame epoch. It never overwrites a slot with different
bytes within that epoch, including across multiple preparation passes. The
normal queued QRhi update uploads each distinct payload once; later identical
draws bind the same buffer with their original stages and binding size. Glyph
sampler binding remains with the original shader path. Existing draw order,
glyph generation and GPU synchronization semantics are preserved.

Only merged native-text batches on the queued path at uniform offset zero are
eligible. Unsupported backend/epoch, size, capacity, allocation failure and
test bypass use the original batch buffer. A shared draw leaves the original
batch buffer invalid, so returning to that buffer forces its contents to be
refreshed even if the shader reports unchanged uniforms. Per-call shared state
resets before material preparation. Unmerged and direct-update paths retain
their original behavior. Pool ownership is bounded to eight renderers per
render thread. Renderer graphics-resource cleanup and keyed QRhi cleanup
callbacks dispose of pool buffers and registrations.

The epoch comes from existing diagnostic D3D11 frame tracking, using native
context identity and the tracked frame-end serial. It is not a public portable
Qt frame counter. This dependency must be replaced or integrated and verified
before production use. The current offscreen tracker does not advance that
serial; in that case the book retains immutable entries and falls back when
full rather than reusing slots under an unproven boundary. Normal QRhi update
and resource lifetime behavior, including existing D3D11 reuse proofs, still
govern backend updates. No driver shadow buffers are written directly.

## Verification completed

Fourteen helper QtTest results pass, including full-byte comparisons under
forced hash collisions, bounded capacity, retaining existing hits after the
pool fills, nonconsecutive keys within one epoch, pass reentry, epoch changes,
queued/unchanged distinctions, nested census isolation and no payload access
without an eligible owner.

Native D3D11 at actual DPR 1.75: all 45 rendering results pass with the feature
disabled, then all 45 pass with it enabled; zero skips. All 15 PNG outputs match
exactly by SHA-256 between those runs. The new fixture interleaves twelve small
native-text labels with rounded backgrounds, changes colors and font size,
resizes, deliberately bypasses sharing, resumes sharing, and replaces its
window/renderer. It verifies each named text leaf's physical scene origin and
unscaled scene unit vectors, plus nonempty painted glyph pixels in every label.
Enabled execution asserts both shared hits and real shared uploads occurred;
disabled execution asserts neither occurred. The final small-caption capture
was visually inspected and is legible and sharp.

The first fixture run failed because QObject descendant traversal did not
enumerate Repeater's visual children; the retained failure log records zero
leaves. Traversing `childItems()` corrected the fixture. Initial fixture build
issues (an ambiguous empty-point overload and moc parsing the embedded QML
body) were fixed using an explicit QPointF and a moc-only method declaration.
The final 45-result count and feature-use assertions ensure the new test runs.

Three static import-isolation results and the Windows system-DLL audit pass.
Diagnostic intermediate output resides in the C: archive with its original D:
path preserved by a junction; the freshly linked standalone host remains under
the worktree's artifacts directory.

## Remaining verification

Run the same host in a controlled A/B/B/A live scrolling comparison, with
candidate counters proving sharing and no allocation failure, and evaluate
original display, motion, coverage and foreground gates unchanged. Compare
preparation/recording/submission distributions, update counts, pool creation
and fallback behavior rather than assuming fewer calls remove all stalls.
Retain cold-wheel, verified-warm and quarter-height middle-scroll acceptance
scope. Add explicit owner-capacity, standalone RHI-cleanup and device-loss
regressions if this remains a worthwhile production candidate. Current tests
cover ordinary renderer/window replacement, not every device-loss path.

Evidence: `profiler/experiments/masonry-shared-text-uniform-prototype-2026-10-30/`.

> AI assistance has been used to create this output.
