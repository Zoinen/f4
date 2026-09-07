# Issue #262 solution review

## Reproduction

Opening a NetFox connection mounts a child VFS whose `GetPath()` is a remote
POSIX path such as `/home/user`. `FileSystemPanel.readDirectoryEx` passed that
path directly to `AddFolderHistory`. The global folder-history navigator later
interpreted it as a local OS path, so remote navigation appeared as local
folders.

## Three candidate solutions

1. Give NetFox a new persistent `netfox://connection/path` URI and implement
   standalone restoration from saved connection settings. This would provide
   the richest history, but requires URI design, config lookup, migration, and
   restore tests across every NetFox protocol.
2. Store the remote path together with the active provider instance and teach
   history navigation to reuse that instance. This helps within one session,
   but persisted history would still be ambiguous after restart and would
   retain provider-specific state in the core.
3. Record nested absolute paths only when the path is already a persistent URI
   or is owned by a standalone provider; suppress unqualified nested absolute
   paths. This fixes the reported misclassification for NetFox and protects
   every provider that exposes an internal absolute path without claiming a
   restore route.

## Selected solution and three-pass review

The third solution is selected. It is implemented in the common panel history
boundary, so NetFox requires no transport changes and archive/URI providers
keep their existing history behavior.

### Pass 1: functional behavior

The remote `/home/user` path is no longer saved as a local history entry.
Standalone archive paths and persistent URI paths remain eligible, while local
panels and relative nested paths keep their existing behavior.

### Pass 2: safety and compatibility

The change only removes entries that cannot be reopened by the host. It does
not change VFS navigation, remote I/O, selection state, or path normalization.
Slash-rooted POSIX paths are recognized even when f4 itself runs on Windows,
which covers NetFox's cross-platform path format.

### Pass 3: regression scope

The regression test covers the reported unqualified nested absolute path,
relative nested paths, and persistent URI paths. The full `cmd/f4` tests and a
native Windows build will be run before the pull request is finalized.
