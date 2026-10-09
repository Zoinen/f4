# Issue #722: access rights of copies

## Scope

Issue #722 asks for several copy/move options at once: access rights, an
existing-file mode, symlink contents, retry counts and a log. The reporter was
asked which part comes first and answered "first time access rights options",
so access rights came first. The other options are described in
`ISSUE_722_COPY_OPTIONS.md`.

## What the option does

The F5/F6 dialog has an "Access rights" field with the three entries Far
Manager offers, and the choice becomes the default of the next operation, the
way far2l remembers the options of its copy dialog. The same value is a setting
(`[Panel] CopyAccessRights`, Settings Center: File operations -> Execution), so
a copy started without the dialog — Shift+F5, or F5 with its confirmation
turned off — follows it too.

| Choice | New destination | Destination that already existed |
| --- | --- | --- |
| Default | permission bits of the source | keeps its own permissions |
| Copy | permissions of the source | permissions of the source |
| Inherit | permissions of the destination folder | permissions of the destination folder |

A file never inherits the folder's execute bits: a folder needs them to be
entered at all, which says nothing about running the files inside it. So a copy
landing in a `0750` folder becomes `0640`, and a folder copied into it becomes
`0750` — and the folder receives that before its contents are copied, so the
whole copied tree inherits rather than only its root.

## Why these three meanings

far2l is no help here: it replaced Far's three-way choice with a single
"Copy access mode" checkbox (`far2l/src/copy.cpp`), and only the commented-out
`ID_SC_ACCOPY` / `ID_SC_ACINHERIT` / `ID_SC_ACLEAVE` remain of the original.
The meanings above were therefore chosen against what the platforms do:

- "Default" follows `cp` and Win32 `CopyFile` where it matters to an
  overwrite: an overwritten file keeps the permissions it had, because
  overwriting its contents is not a request to change who may read it. A new
  copy takes the source's permission bits without the umask `cp` would apply;
  that is what f4 did before the option existed.
- "Copy" is the explicit request to carry the permissions across in every
  case.
- "Inherit" is the POSIX reading of the Windows "reset the ACL so it is
  inherited from the target folder": the copy is given what the folder gives a
  new object.

"Inherit" cannot be implemented as "do not chmod at all". `OSVFS.Create` opens
a destination that does not yet exist with `0600`, so that its contents are
never readable through a permissive umask while they are still incomplete, and
the final permissions are restored afterwards. Leaving that alone would give
every inherited copy `0600`.

## Behaviour change

Only one case changes for a user who never opens the field: overwriting an
existing file with "Default" leaves that file's own permissions in place, where
f4 previously forced the source's permissions onto it. "Copy" restores the old
behaviour explicitly.

## A rename, Windows, and the other copy paths

- A move within one filesystem is a rename, and a rename keeps the object's
  own permissions, which are the source's: "Default" and "Copy" have nothing to
  add. "Inherit" does. Far resets the security of a renamed object under this
  choice (`far/copy.cpp`: `ResetSecurity` right after `move_file`, in both
  rename paths), and f4 gives the renamed tree its new folder's rights
  (`inheritMovedTree`). An earlier version of this note said the option was
  about copies only and that Far behaved the same; Far's source says otherwise.
- On Windows the access rights are the DACL, which permission bits cannot
  express: Go's `Chmod` there only toggles the read-only attribute, and the
  copied attributes are applied again right after it. "Copy" puts the source's
  DACL on the copy, protected or not as it was on the source; "Inherit"
  replaces the copy's explicit DACL with an empty unprotected one, so that it
  has what its folder gives it; "Default" leaves it to Windows. This follows
  Far's own `set_security` and `reset_security`
  (`far/platform.security.cpp`). Under "Inherit" f4 also resets a file a copy
  overwrote, where Far resets only renamed objects: the table above promises
  the folder's rights to a destination that already existed.
- A server-side copy and a server-to-server `scp -p` do not stream through
  f4. The chosen permissions are applied after them, and "Default" puts back
  what an overwritten file had, because `scp -p` forces the source's onto it.
- A bulk copier (sequential archive extraction) consults none of the dialog's
  choices, so any choice other than the defaults sends the copy down the
  per-file path.
- A destination VFS that does not carry Unix permissions ignores the mode, as
  it already did: the mode travels through `SetAttributes`, and a zero
  `UnixMode` is how every VFS in the tree is told to leave permissions alone.

## Tests

`internal/fileops/access_rights_test.go` copies a file into a new destination,
onto an existing destination, and a whole folder, in each of the three modes,
and asserts the resulting permission bits. The Unix assertions skip on Windows.
`TestAccessRightsModeFromConfig` covers a stored value f4 does not know and the
path a copy takes when no dialog was shown. `TestInheritReachesARenamedTree`
covers a rename, and `security_windows_test.go` pins the DACL of each choice on
Windows.
