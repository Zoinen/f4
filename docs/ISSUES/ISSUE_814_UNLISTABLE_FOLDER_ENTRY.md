# Issue #814 solution review — entering a folder the user may not list

Report: on Windows, Enter on a folder the current user has no access to shows
the folder being entered and left, then `Access Denied`. Far 3 shows the error
without the panel moving.

## Cause

Established from the code:

- Enter on a directory row calls `OSVFS.SetPath`, which accepted any path that
  `hostfs.Stat` reports as a directory, and then `ReadDirectory`.
- `readDirectoryEx` replaces the rows with `..` and redraws under the new path
  at once; `ReadDir` runs in the background.
- When `ReadDir` fails with a permission error the panel goes back to the
  parent (`moveToParentAfterLoadFailure`) and shows `Cannot access folder`.
  That sequence is the enter-and-leave the issue describes.

Why `Stat` and `ReadDir` disagree on Windows, from the Go 1.26.6 sources:
`os.Stat` answers a non-reparse path from `GetFileAttributesEx`, which needs only
the right to read attributes; `os.ReadDir` opens the directory with
`CreateFile(GENERIC_READ, FILE_FLAG_BACKUP_SEMANTICS)`, which needs the right to
list it. `ERROR_ACCESS_DENIED` answers `errors.Is(err, fs.ErrPermission)`.

Far 3 (`FileList::ChangeDir`) and far2l call `FarChDir` first and show the error
without updating the panel when it fails.

## Change

- `OSVFS.SetPath` opens the directory the way `ReadDir` starts (`hostfs.Open`,
  the same `CreateFile` access on Windows, `open(2)` with `O_RDONLY` elsewhere),
  retrying the reparse candidates `ReadDir` retries. A permission refusal is
  returned as `*vfs.NotListableError`, which unwraps to the host error, so the
  message text is the one `ReadDir` produced. Other errors are left to
  `ReadDir` as before.
- When the sudo client is available (every non-Windows run of f4) the path is still
  accepted and `ReadDir` lists it through elevation, as before.
- Enter already reports a `SetPath` error in place.
- `NavigateToPath` (command-line `cd`, bookmarks, Ctrl+PgUp to a parent,
  workspace restore) reports the refusal with the same message and returns
  true, so a `cd` is not handed on to the shell. The panel stays where it was
  instead of ending up in the target's parent.
- The folder history walk skips such an entry, as it skips any entry that
  cannot be opened.
- The temp panel no longer falls through to `Execute` on such a folder, and
  "show on passive panel" reports the refusal.

Cost: one extra open and close of the directory per local navigation.

## Verification

The regression tests deny listing with `chmod 0` on Unix and with
`icacls /deny *S-1-1-0:(RD)` on Windows, and check first that the host produces
the reported state (Stat succeeds, opening for listing is refused); otherwise
they are skipped with what was observed, e.g. when run as root.

Run on Linux as a non-root user: with the `SetPath` check removed all four
tests fail (the panel path becomes the denied folder; the history walk ends in
its parent), with it they pass. The Windows variant runs in CI.
