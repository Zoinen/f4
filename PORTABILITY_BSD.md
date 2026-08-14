# BSD portability & session start-up robustness

Working document for issue [#429](https://github.com/unxed/f4/issues/429)
("hangs at start in a terminal on OpenBSD 7.8+").

It contains (1) what is established from the source, (2) what is still unknown,
(3) a prioritised work plan. Sections 4–7 are meant to be actionable without a
reproduction: none of the fixes below depend on what the OpenBSD trigger turns
out to be.

Line numbers refer to `main` as of 2026-08-13.

---

## 1. The report

Title: *"для какой версии openbsd заявлена поддержка?"*
Body: *"7.8+ в терминале повисает на старте"*. No comments, no logs.

Missing: f4 version and provenance (nightly artifact vs. self-built), arch,
terminal emulator, `TERM`, whether `DISPLAY`/`WAYLAND_DISPLAY` are set, whether
7.7 ever worked. Note that nothing in the report establishes a regression —
"7.8+" may simply be the only versions the reporter has.

## 2. Established facts

### 2.1 What bounds OpenBSD support

The boundary is set by Go, not by f4:

* OpenBSD 7.5 removed the indirect syscall entry point (`syscall(2)`), which
  means Go's `syscall.Syscall*` no longer works there
  (golang/go#63900, golang/go#36435).
* Go keeps exactly one compatibility shim: `syscallInternal` reroutes
  `SYS_IOCTL` to the libc stub and returns `ENOSYS` for every other trap number.
  See `src/syscall/syscall_openbsd_libc.go` — the comment there marks the shim
  as temporary ("for the time being").
* Go binaries on OpenBSD are dynamically linked against `libc.so` even with
  `CGO_ENABLED=0`, and Go supports the two most recent OpenBSD releases.

So 7.8/7.9 are within what the toolchain (go 1.26 per `go.mod`) supports, and
"version support" is unlikely to be the root cause on its own. But the ENOSYS
rule already bites in one place — see §4.4.

### 2.2 How a terminal start-up actually works

`main()` → `ManageSessions()` (`main.go:217`, `main.go:248`) →
`startNewSession()` (`session_unix.go:131`):

1. The client spawns `f4 --server <sock>` with `Setsid: true` and
   stdin/stdout/stderr redirected to `/dev/null`.
2. It waits for the socket to appear, at most 50 × 10 ms
   (`session_unix.go:160`), then calls `runClient()` unconditionally
   (`session_unix.go:167`).
3. `runClient()` (`session_unix.go:170`) sends its own fd 0, fd 1 and the write
   end of a notify pipe to the daemon over a `unixgram` socket via `SCM_RIGHTS`.
4. It closes its own copy of the write end and blocks on
   `syscall.Read(notifyPipe[0], dummy)` (`session_unix.go:226`) — **no timeout,
   no liveness check on the daemon, no fallback**.
5. The daemon (`runServer`, `session_unix.go:230`) receives the fds, swaps
   `os.Stdin`/`os.Stdout`, enables raw mode, redraws and enters
   `FrameManager.Run(reader)`.

The daemon cannot report anything to the user: its stderr is redirected to
`crashes/stderr_<ts>_<pid>.log` by `vtui.SetupStderrLog()`, and `DebugLog` only
reaches disk when `VTUI_DEBUG` is set.

### 2.3 Consequences that follow from the architecture alone

* **If the shell prompt does not come back, the daemon reached the point where
  it created the socket and received the fds.** Every earlier failure makes
  `WriteMsgUnix` fail with `ENOENT`, and the client exits normally. So
  `InitCore()`/`SetupUI()` (config, plugins, VFS) complete on OpenBSD, and the
  stall is *after* the `SERVER: FDs received` log line.
* **A daemon crash is also indistinguishable from a hang.** Fds received via
  `SCM_RIGHTS` carry no `FD_CLOEXEC`, and Go's `forkExec` does not close
  unknown descriptors, so the notify-pipe write end is inherited by the shell
  that `initPTY()` (`panels_frame.go:829`, called from `panels_frame.go:429`)
  starts right after attach. If the daemon dies afterwards, the grandchild
  keeps the pipe open and the client waits forever.
* The built-in PTY itself cannot block start-up: `initPTY` runs in a goroutine
  and only logs failures. It can, however, leave the built-in terminal dead
  (cf. issue #444).

### 2.4 Ranked hypotheses for the OpenBSD trigger

1. The daemon panics shortly after attach; the symptom is masked by the
   notify-pipe leak described above.
2. The daemon is alive but stuck between `PrepareTerminal()` → `Redraw()` →
   `fm.Run(reader)`. Candidates: `term.MakeRaw` on a tty owned by another
   session; the first large flush into a tty that Go switched to `O_NONBLOCK`
   (the `clearNonBlock` comment at `session_unix.go:341` documents that this
   happens); `unix.Poll` in `vtinput/reader_unix.go`.
3. Not the terminal path at all: `shouldTryGui()` (`main.go:251`) returns true
   whenever `DISPLAY` or `WAYLAND_DISPLAY` is set, so f4 detaches
   (`detach_unix.go`, parent calls `os.Exit(0)`) and tries a GUI backend that
   almost certainly fails to load on OpenBSD. The symptom differs (silent exit
   rather than a hang), but it is one command to rule out.
4. PTY allocation. `pty_ptm.go` looks correct for OpenBSD: `PTMGET =
   0x40287401` is `_IOR('t', 1, 40)`, matching `struct ptmget` with
   `cn[16]/sn[16]`. It is wrong for NetBSD — see §4.5.

## 3. Data to request from the reporter

* `echo $TERM; echo $DISPLAY; uname -a; f4 --version`, and whether the binary is
  a nightly artifact or a local build.
* `DISPLAY= WAYLAND_DISPLAY= f4` — separates hypothesis 3 from 1–2.
* `f4 --debug --log /tmp/f4.log` and the whole log. The daemon's milestones are
  already there and bracket every step: `SERVER: Daemon listener active` →
  `SERVER: FDs received` → `SERVER: Raw mode and environment enabled` →
  `SERVER: Entering fm.Run()`. The last line from the daemon's `[P<pid>]`
  pinpoints the stall.
* Everything in `crashes/` next to the profile directory (`stderr_*.log` and any
  crash report). **As of 4.8, this is now the fastest path:** find the daemon
  pid (`ps -axl | grep f4` or the session picker), run `kill -USR1 <pid>`, and
  send back the resulting `crashes/crash_<ts>_<pid>.log` — no `--debug`
  rebuild or reproduction steps needed, and the process is left running.
* OpenBSD has no `truss`: `ktrace -di -f /tmp/f4.kt f4`, then
  `kdump -f /tmp/f4.kt | tail -200`. Useful if the `SIGUSR1` dump points at a
  specific blocking syscall and more detail is needed on what that syscall is
  doing.
* Did it work on 7.7 — regression or never-worked.

---

## 4. Work plan

Constraints for every change below: `CGO_ENABLED=0`, cross-compiled from Linux
CI, must not regress linux / darwin / windows / freebsd / netbsd / dragonfly /
illumos / solaris. Prefer `golang.org/x/sys/unix` wrappers (they go through libc
stubs on OpenBSD) over `syscall.Syscall`.

### P0 — turn a silent hang into a diagnosable error

These are worth doing regardless of #429; they are what makes the *next* report
of this kind answerable in one round-trip.

**4.1 Set `FD_CLOEXEC` on descriptors received via `SCM_RIGHTS`**
`session_unix.go`, right after `syscall.ParseUnixRights` (`session_unix.go:279`)
and before the fds are wrapped in `os.NewFile` (`session_unix.go:288-290`).
Use `unix.FcntlInt(uintptr(fd), unix.F_SETFD, unix.FD_CLOEXEC)` on all three.
The daemon uses these fds itself and never needs to pass them to a child; the
built-in shell gets its own PTY. Effect: a dying daemon immediately releases the
notify pipe and the client wakes up instead of hanging.

*Test:* start a session, spawn a child in the daemon, kill the daemon, assert
the client's read returns. A unit-level variant: assert `FD_CLOEXEC` is set on
all fds returned by `ParseUnixRights` in the attach path.

**4.2 Timeout + liveness check in the client**
Replace the bare `syscall.Read(notifyPipe[0], dummy)` (`session_unix.go:226`)
with a `unix.Poll` loop (~250 ms per tick) that also checks
`syscall.Kill(serverPID, 0)`. `serverPID` is already looked up at
`session_unix.go:174-180`; when it is 0 (the session file has not been written
yet) fall back to the pid returned by `cmd.Start()` — thread it through from
`startNewSession`.

On a dead daemon: defensively restore the terminal (the daemon may have left
raw mode / the alternate screen on), print one line naming
`crashes/stderr_*.log` and the `--debug` flag, exit non-zero.

**4.3 Honest wait for the socket**
`session_unix.go:160-167`. Raise the budget to a few seconds, run `cmd.Wait()`
in a goroutine in parallel, and stop waiting as soon as the daemon exits —
reporting its exit code and log path — instead of proceeding to `runClient` and
failing with a bare `ENOENT`. Also avoids leaving an orphan daemon that later
shows up in the session picker.

### P1 — definite bugs, fixable without a reproduction

**4.4 `clearNonBlock` never runs on OpenBSD 7.5+ — DONE**
`session_unix.go:341-343` used `syscall.Syscall(syscall.SYS_FCNTL, ...)`, which
returns `ENOSYS` there (§2.1), so `O_NONBLOCK` was never cleared from the
shared tty description on exit and the parent shell inherited a non-blocking
stdin. This is the same failure class as #444.

Replaced with a package-level `clearNonBlock(f *os.File)` using
`unix.FcntlInt(f.Fd(), unix.F_GETFL/F_SETFL, ...)`, which goes through the
libc `fcntl(2)` stub and keeps working under OpenBSD's restriction. Errors
from both fcntl calls are now logged via `vtui.DebugLog` instead of discarded.

*Test:* `TestClearNonBlock_ClearsFlag` (`session_unix_test.go`) sets
`O_NONBLOCK` on a pipe fd, calls `clearNonBlock`, and asserts `F_GETFL` no
longer reports it. Negative control: with the fix reverted, the test fails
with "O_NONBLOCK still set after clearNonBlock" (confirmed locally, not
committed).

*Verification:* builds clean on `openbsd/amd64`, `linux/amd64`; scoped test
run (session + clearNonBlock + setCloseOnExec) green. Full `go test .` has two
pre-existing failures (`TestExecuteFileOp_Move_PermissionDenied_Recovery`,
`TestUpdateFailureMessageRepro`) reproduced identically on an unmodified
checkout — unrelated to this change (root/network environment effects).

**4.5 NetBSD `PTMGET` constant is wrong**
`pty_ptm.go:38` hardcodes the OpenBSD value for a file built with
`//go:build openbsd || netbsd`. NetBSD's `struct ptmget` has 1024-byte name
fields, so the request number differs. `golang.org/x/sys/unix` provides
`IoctlGetPtmget(fd, unix.PTMGET)` for NetBSD — use it there. OpenBSD has no
such wrapper (no `Ptmget` type, no exported `ioctlPtr`), so keep the manual
ioctl on that side, but check `errno` and log a specific message.
Splitting the file into `pty_ptm_openbsd.go` / `pty_ptm_netbsd.go` is the
cleanest shape.

**4.6 Migrate the remaining raw ioctls to `x/sys/unix` wrappers**
`pty_ptm.go:107,126`, `pty_bsd.go:113,132`, `pty_unix.go:113,135`,
`pty_darwin.go:121,140` and friends work today only because of Go's temporary
`SYS_IOCTL` shim. Where wrappers exist, use them: `unix.IoctlSetWinsize` for
`TIOCSWINSZ`, `unix.IoctlGetInt` for `TIOCGPGRP`, etc. Cheap insurance against
the next OpenBSD release.

**4.7 The GUI detour is silent**
`shouldTryGui()` (`main.go:251`) diverts to GUI mode on any Unix with `DISPLAY`
set, and `checkAndDetach` (`detach_unix.go`) exits the parent with `os.Exit(0)`,
so a failed backend produces no output whatsoever. Minimum: print one line
before detaching (mentioning `--tty` to stay in the terminal), and propagate the
GUI failure reason back to the terminal — via a temp file or the same notify
channel — instead of only into `/dev/null`.

### P2 — permanent diagnostics

**4.8 State dump on signal — DONE**
`installHangDumpHandler()` (`hang_dump_unix.go`, no-op stub in
`hang_dump_windows.go`), wired up in `main()` right after `vtui.CrashDirFull`
is set — so it covers every process shape (`--server` daemon, client, GUI,
plain foreground run) from as early as possible, including a stall inside
`InitCore()`/`SetupUI()` itself. On `SIGUSR1` it calls the already-exported
`vtui.RecordCrash(...)` — no changes needed in vtui — which writes goroutine
stacks, `TERM`/`LANG`, open fd count, terminal size, the UI frame stack and
recent log history to `crashes/crash_<ts>_<pid>.log`, without exiting or
otherwise disturbing the process. The answer to any "it hangs" is now
`kill -USR1 <pid>` (pid from `ps` or the session picker) instead of "rebuild
with --debug and reproduce".

*Verification:* built the daemon (`--server` mode) standalone, backgrounded
it, confirmed via `ps` it was genuinely blocked (not exited), sent
`SIGUSR1`, and got a dump showing goroutine 1 parked exactly where expected —
inside `net.(*UnixConn).ReadMsgUnix` called from `main.runServer`
(`session_unix.go:285`) — while the process stayed alive and undisturbed
afterward. This is the same mechanism that would catch a real OpenBSD stall,
wherever in the startup path it turns out to be.

*Not done yet, left for later:* a client-side SIGUSR1 handler exists too
(same `installHangDumpHandler` call in `main()` covers it), but it hasn't been
exercised against a client stuck in the `runClient` notify-pipe read — worth
a quick check before relying on it for that specific case.

**4.9 Always-on session milestone log**
A dozen lines with rotation — spawn / listen / attach / raw mode / first flush /
run / exit — written regardless of `VTUI_DEBUG`. These are exactly the lines
missing today to close a report in one round-trip.

**4.10 `--no-daemon` mode**
Run the session in the current process, skipping the client/daemon split. Both
a workaround for users and a way to separate IPC/fd-passing problems from
terminal problems in a single run.

**4.11 Propagate `VTUI_DEBUG`/`--log` explicitly**
`startNewSession` relies on the daemon inheriting the environment that argument
parsing set via `os.Setenv`. Set it explicitly in `cmd.Env`; the current
behaviour breaks the first time argument parsing is reordered.

### P3 — CI

**4.12 Run something on OpenBSD**
CI cross-builds OpenBSD binaries but never executes them. A VM-based job
(`vmactions/openbsd-vm` or equivalent) running `go test ./...` plus a smoke
start with `--no-daemon` and an immediate exit would catch the whole ENOSYS
class automatically.

---

## 5. Suggested sequencing

* **Patch 1 (minimal):** 4.1, 4.2, 4.3, 4.4 + tests. This alone converts the
  #429 symptom from "hangs forever, no output" into a message naming a log file,
  and fixes one confirmed OpenBSD bug.
* **Patch 2:** 4.5–4.7.
* **Patch 3:** 4.8–4.12.

## 6. What patch 1 explicitly does *not* do

It does not claim to fix #429. The trigger is still unknown, and identifying it
requires the data in §3. The point of patch 1 is that after it, the same report
arrives with a log path attached.
