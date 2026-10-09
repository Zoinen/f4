# f4#1716: saving an edited file over SFTP failed (`rename failed: SSH_FX_FAILURE`)

## Symptom

NetFox/SFTP: creating a file (Shift+F4, F2) worked, but reopening it (F4) and
saving again (F2) showed `Failed to finalize save (rename failed)` with
`sftp: "Failure" (SSH_FX_FAILURE)`.

## Cause

The editor saves through a staged temp file and renames it onto the target,
passing `vfs.WithDestinationOverwrite(ctx, true)` when the target exists.
`SFTPVFS.Rename` ignored that decision and always sent a plain
`SSH_FXP_RENAME`, which SFTP v3 servers (OpenSSH included) refuse when the
target exists. The first save worked only because the target was absent.

## Fix

`SFTPVFS.Rename` (`plugins/netfox/sftp_vfs.go`) replaces an existing target
when the context says overwrite (`DestinationOverwrite` known and true):

1. `posix-rename@openssh.com` when the server advertises it (atomic);
2. otherwise `renameSwap`: plain rename, and if the target exists, park it
   under `<target>.f4bak<hex>`, move the source in, delete the backup; on a
   failed move the backup is put back.

Without an explicit overwrite decision (unknown or `false`) the plain RENAME
stays, so it still refuses to clobber.

Tests: `plugins/netfox/sftp_rename_overwrite_test.go` (real `pkg/sftp`
request server over loopback).

## Doubtful points (not changed on purpose)

- `renameSwap` is not atomic: between the two renames the target name briefly
  does not exist. Only servers without `posix-rename` take this path.
- Other callers that rename onto an existing SFTP target without passing the
  overwrite decision still get the plain RENAME. Not audited; if another
  "rename failed" report appears, check whether that caller sets the context.
