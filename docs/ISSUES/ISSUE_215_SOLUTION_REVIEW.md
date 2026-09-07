# Issue #215 solution review

Reported behavior: a non-administrator Windows installation tried to update
itself and displayed `failed to spawn sudo process`; the binary had been
installed from the GitHub release and worked only when launched as
Administrator.

## Candidate 1: keep the shared sudo fallback

The existing fallback is Unix-specific: it starts `sudo`, creates a Unix
socket, and passes file descriptors through Unix IPC. It cannot work on
Windows. Keeping it produces the reported error and is rejected.

## Candidate 2: tell the user to relaunch f4 as Administrator

This would avoid the error but makes every protected update a manual two-step
operation and does not use the Windows permission model. It is a workaround,
not an updater fix, and is rejected.

## Candidate 3: use a short-lived Windows UAC update helper (chosen)

The normal process downloads the archive and first tries the existing direct
installation path. If the executable directory is protected, or extraction
returns a Windows permission error, it saves the archive to a temporary file
and starts the same executable with `ShellExecuteExW` and the `runas` verb.
The elevated process enters a private `--update-helper` path, extracts the
archive using the existing format-specific extractors, reports its exit code,
and terminates without starting the UI.

## Three-pass review

1. The helper is isolated from the Unix dispatcher, so Windows cannot invoke
   `sudo`; the elevated process still uses ordinary Windows file APIs.
2. The parent waits for completion and reports UAC cancellation or a non-zero
   helper exit code. A temporary archive is removed after either result, and
   the helper reuses the existing traversal-safe extractors.
3. The normal writable-directory path is unchanged, while protected
   installations get one UAC prompt before extraction. The shared VFS sudo
   client is also marked unavailable on Windows, preventing the same false
   Unix fallback for other file operations.

This is the smallest general fix for the reported Windows updater failure;
implementing a full Windows privileged VFS dispatcher would require a
separate IPC and handle-transfer design and is outside this ticket.
