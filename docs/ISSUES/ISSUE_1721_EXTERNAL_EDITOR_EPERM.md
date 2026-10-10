# f4#1721: external console editor failed on Linux (`fork/exec /usr/bin/nano: operation not permitted`)

## Symptom

Ubuntu 22.04, f4 started in xterm, console editor `nano`: opening a file
showed `fork/exec /usr/bin/nano: operation not permitted`. nano from the shell
worked, the GUI editor (Kate) worked, FreeBSD worked.

## Cause

`runExternalEditor` gives the editor `os.Stdin`. `ConfigureExternalEditorProcess`
(`internal/editor/external_unix.go`) always asked for `Setsid + Setctty`, so
the child starts a new session and then does `ioctl(0, TIOCSCTTY, 1)` (Go
passes arg 1, "steal"). When f4 itself runs on that terminal the terminal is
already the controlling terminal of f4's session; stealing it needs
CAP_SYS_ADMIN, so a regular user gets EPERM. The FreeBSD build never did this
(its variant is empty), GUI editors skip it (`gui.Running`).

The old tests missed it: they used a fresh PTY without a session, and root
(CAP_SYS_ADMIN) is allowed to steal.

## Fix

`inheritsControllingTTY` (`internal/editor/external_ctty_linux.go`): when
stdin is a terminal whose session (`TIOCGSID`) is our own session, the editor
simply inherits it, like on FreeBSD. A terminal with no session (the Unix
session daemon case that `Setsid + Setctty` was added for) keeps the old path.

Test: `TestExternalEditorStartsOnOwnControllingTTY` re-executes the test binary
as a session leader on a PTY (like f4 in a terminal emulator), drops root to an
unprivileged user and starts an editor through `ConfigureExternalEditorProcess`.
Without the fix it fails with the exact error from the ticket.

## Doubtful points (not changed on purpose)

- Linux only. `TIOCGSID` is missing in x/sys for darwin; other platforms use a
  stub that keeps the old behaviour. macOS/OpenBSD/NetBSD very likely hit the
  same EPERM for a regular user; unverified. If reported, extend
  `inheritsControllingTTY` (on darwin probe with `getsid` of the tty's
  foreground group instead of `TIOCGSID`).
- Not verified on the reporter's machine; ask them to confirm with a build
  containing this change.
