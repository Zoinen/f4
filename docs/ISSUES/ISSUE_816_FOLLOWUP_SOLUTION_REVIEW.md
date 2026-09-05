# Issue 816 follow-up solution review

## Reported failures

The follow-up contains two separate regressions:

1. F3/F4 on a member of an encrypted archive can leave the password dialog
   underneath the delayed `Opening...` progress screen.
2. RAR archives split into volumes fail while the archive VFS reads the
   directory, because the generic fallback passes only one input stream to
   `rardecode`.

The second failure was reproduced on Windows 10 x64 with the two-volume RAR
fixture shipped by `github.com/unxed/archives`: the current code returned
`rardecode: multi-volume archive continues in next file`.

## Three candidate approaches

### 1. Fix the shared archive libraries

Add a volume-aware `FileSystem` implementation to `unxed/archives` or change
`unxed/zipper` so its fallback always supplies the first volume name and a
filesystem rooted at the volume directory. This would put the behavior at the
most reusable layer, but it would require a separate dependency change and
would still leave `ArchiveFS.Open` needing a lifetime-safe member reader.

### 2. Concatenate all volumes in f4

Discover the volume files, concatenate them into a temporary stream, and keep
using the existing generic fallback. This preserves the current f4 interfaces,
but it requires extra disk space, loses the decoder's volume semantics, makes
encrypted and solid archives more fragile, and cannot handle missing volumes
with a useful error.

### 3. Add an f4 RAR filesystem adapter and guard progress dialogs

When the identified local format is RAR, configure `archives.Rar` with the
first volume's basename and a directory `fs.FS`. Use the shared archive index
for `ReadDir`/`Stat`, and materialize a requested member while the
volume-aware decoder is still alive before returning an `fs.File`. Separately,
make delayed progress screens wait while another modal prompt is active.

## Comparison and decision

Candidate 3 is selected. It is a small, local integration fix that exercises
the API already provided by the pinned archive libraries, avoids a dependency
fork, and gives F3/F4 a stable member lifetime across volume boundaries. The
adapter is used only after format identification confirms RAR, so ZIP, 7z,
tar, and ordinary fallback behavior remain unchanged. The progress change is
generic and protects any modal prompt that races with a delayed operation
screen, not only archive passwords.

The implementation is covered by a Windows-capable two-volume RAR regression
test and a UI regression test that keeps a password-style modal above the
delayed viewer progress screen. Existing archive and viewer tests remain part
of validation.

— zoin-bot
