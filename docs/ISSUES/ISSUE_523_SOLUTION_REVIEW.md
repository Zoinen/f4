# Issue #523 follow-up solution review

## Reproduction

The maintainer-provided `history.log.1.gz` is a valid gzip stream. Its
decompressed payload is 24,226 bytes and contains one apt history log.

On the current f4/upstream dependency chain, the stream opens and reads
successfully, but the root listing reports the physical name
`history.log.1.gz` and the compressed size. The expected Far2l behavior is a
single logical regular file named `history.log.1`, with the decompressed size
and contents.

## Three-pass comparison

1. **Fix f4's archive VFS or special-case `.gz`.** Rejected. This would make
   f4 compensate for metadata supplied by its dependency and would not fix
   other consumers of `archives.FileSystem`.

2. **Fix `zipper`'s fallback adapter.** Rejected. `zipper` delegates format
   detection and the single-file filesystem to `unxed/archives`; adding a
   presentation-only rename there would duplicate the responsible layer's
   behavior.

3. **Fix `archives.FileFS` for compressed regular files.** Selected. The
   library already transparently decompresses reads, so `FileFS` should expose
   the same logical view through `ReadDir`, `Stat`, and `Open`: strip the
   compression suffix, accept the logical name, and report the decompressed
   size. The physical basename remains accepted for compatibility.

## Safety review

The change is limited to the `FileSystem` branch that identifies a compressed
regular file. Archive members and ordinary uncompressed files retain their
existing behavior. Reads remain streaming and read-only. Metadata calculation
counts the decompressed stream and propagates decoder errors instead of
silently hiding corrupt input.

Validation includes the generic gzip regression test and the exact attached
`history.log.1.gz` sample: one entry named `history.log.1`, size 24,226 bytes,
and a successful full read of the decompressed payload.
