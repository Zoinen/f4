# Terminal under Windows — umbrella analysis (issue #425)

Companion to `TERMINAL.md`, which describes the architecture. This file
describes what is currently broken in it and in what order it is meant to be
fixed. It covers issues #165, #362, #409 and the terminal half of #424.

## 0. The shared root cause

f4 drives `cmd.exe` by typing service text into the visible shell session and
then trying to un-type it with a textual excision in the parser. That cannot be
made reliable on ConPTY, which (see `TERMINAL.md`, Appendix A) echoes
everything written to its input, fragments output at arbitrary offsets, and
prints the next prompt before the finished command is fully drained.

## 1. Where the code lives

| Subject | Location |
| --- | --- |
| cwd sync into the shell | `panels_frame.go`, `syncShellPath` area (`rem f4_sync`) |
| Excision of the echoed sync command | `ansi_parser.go`, top of `AnsiParser.Process` |
| Command started from the command line | `panels_frame.go`, Enter handling |
| File started with Enter on the panel | `actions.go`, `actionExecute` |
| `PROMPT=$E]133;D$E\$P$G` | `panels_frame.go`, `initPTY` |
| OSC 133 handling | `terminal_view.go`, `HandleOSC133` |
| Busy state | `panels_frame.go`, `isPtyBusy`; `pty_windows.go`, `PTY.IsBusy` |
| Key to PTY byte translation | `input_translation.go`, `TranslateInput` |
| Key events in GUI mode | `vtui/gogpu_host.go` |
| Ctrl+C / Ctrl+Break | `console_ctrl_handler_windows.go` |
| Workspace clone | `panels_frame.go`, `PanelsFrame.Clone`; `terminal_view.go`, `CloneStateFrom` |

## 2. Issue #165 — output scrolls up, sync command becomes visible

Changing the panel directory writes `cd /d "<path>" & rem f4_sync` into the
PTY. The parser excises the echo and writes `\r\x1b[2K` in its place.

Two independent defects:

1. The excision erases the text of the line but not the newline that already
   happened, nor the prompt reprinted after it. Every directory change costs at
   least one line, so the terminal creeps upwards. No textual excision can fix
   this.
2. The excision runs on a single `Read` chunk with no carry buffer. ConPTY
   splits output arbitrarily, so a long enough path pushes the marker over a
   chunk boundary, `bytes.Index` misses, and the user sees the raw command.
   This is why the reporter saw it appear "at some nesting depth".

Additionally the fallback branch that excises `cd /d "..." & ` without the
`rem f4_sync` marker also mangles a `cd /d "X" & dir` a human typed.

Fix, in order of increasing cost:

* Stop sending the sync on a plain directory change. The working directory is
  imposed anyway when a command is launched (`cd /d "%s" & %s`), so nothing is
  lost, and the whole class of symptoms disappears.
* Later: track the shell's cwd from the shell (OSC 7 / OSC 9;9) instead of
  imposing it, and suppress echo by position against what we wrote, with a
  carry buffer across chunks.

## 3. Issue #409 — bat/cmd appear to run in the background

Established from the code:

* The Windows path sends `cd /d "dir" & <cmd>` with no OSC 133 framing, unlike
  the Unix path, which wraps the command in `133;C` … `133;D`.
* The only source of a marker on Windows is `PROMPT`, which emits `133;D` at
  every prompt. `133;C` is never emitted at all.
* `isPtyBusy()` is `PTY.IsBusy() || pf.executing`.
* `PTY.IsBusy()` on Windows asks whether the shell process has a child.
  `cmd.exe` interprets `.bat`/`.cmd` in-process and creates no child, so this
  is always false for a batch file.

So the busy state for a batch file rests entirely on `pf.executing`, which is
cleared by the first `133;D` — a marker tied to the prompt, not to the command.

Hypotheses for the early `D`, none yet confirmed three ways:

* H1: the `D` from the prompt printed *before* our command is parsed after
  `pf.executing` was set (the flag is set synchronously on Enter, PTY data
  arrives asynchronously).
* H2: the batch file changes or restores `PROMPT`, or spawns a nested `cmd`.
* H3: ConPTY's premature prompt (Appendix A, observation 3).

H1 fits the "panels come back immediately" report best. Before fixing, collect
evidence: log the chunk number, the offset inside the chunk and a hexdump
around every OSC 133 marker in `HandleOSC133`, plus a timestamp and the exact
wire command at both send sites; then run a `.bat` containing `timeout /t 5`.

Direction once confirmed:

1. Ignore a `D` that arrives without a matching `C` (parity gate). Cheapest and
   also covers H3.
2. Frame Windows commands the way Unix ones are framed, via an `F4E`
   environment variable holding ESC, so `C`/`D` belong to the command.
3. Replace `PTY.IsBusy` with the pseudoconsole's process list rather than the
   shell's children, which is blind to batch files and builtins by design.

## 4. Issue #362 — Ctrl+C does not interrupt in f4-gui

`TranslateInput` only produces a control byte when `e.Char != 0`; with
`Char == 0` it falls through and returns an empty string, so nothing is
written to the PTY.

In the console build the Windows console API delivers Ctrl+C with
`UnicodeChar == 0x03`, so it works — matching the report that the tty version
is fine. In GUI mode `vtui/gogpu_host.go` treats any key held with Ctrl as a
"special or modified" key and emits it immediately with `Char == 0`, and no
text-input event follows for a control combination.

The gap covers the whole family: Ctrl+C, Ctrl+D, Ctrl+Z, Ctrl+\, Ctrl+], Ctrl+@.

Fix: in `TranslateInput`, when Ctrl is held and `Char == 0`, map the virtual
key code (`'A'`–`'Z'` to 1–26, `VK_OEM_4/5/6` to 27/28/29, `VK_6` to 30,
`VK_OEM_MINUS` to 31, `VK_2`/`VK_SPACE` to 0). Check whether the x11 and
wayland hosts behave the same way.

Ctrl+Break closing f4 (from the same issue's comment) is not reproduced from
the code: `console_ctrl_handler_windows.go` swallows both events and is
installed from `main`, i.e. after the Go runtime's own handler, and Windows
calls handlers in reverse order of registration. Log the entry of
`consoleCtrlHandlerRoutine` and re-test both builds before assuming anything.

## 5. Issue #424 — the terminal half

`TerminalView.CloneStateFrom` copies `other.pty` into the clone. The clone's
own `initPTY` goroutine assigns its freshly created PTY to the same field
asynchronously, so which one wins is a race. That is the mechanism behind
"a command run in one workspace shows up in another".

`pty` is ownership, not visual state, and does not belong in a state clone.
`Clone` should set it explicitly under `ptyMutex` after copying the grid.

The routing half of #424 (actions landing in the wrong workspace) was a
separate defect in `findPanelsFrameAnyScreen` and is already fixed; see
`workspace_routing_test.go`.

## 6. Target shape

One shell-session object per PTY owning: the working directory (read from the
shell, imposed only on launch), the construction of the wire command with OSC
133 framing identical on both platforms, echo suppression matched against what
we wrote with a carry buffer, and the busy state derived from `C`/`D` parity.
Today the command construction is duplicated between `panels_frame.go` and
`actions.go` and differs between platforms.

## 7. Order of work

| # | Task | Issue | Risk |
| --- | --- | --- | --- |
| 1 | VK fallback for Ctrl+letter in `TranslateInput` | #362 | low |
| 2 | Drop the cwd sync on directory change | #165 | low |
| 3 | OSC 133 parity gate | #409 | low |
| 4 | Stop copying `pty` in `CloneStateFrom` | #424 | low |
| 5 | Logging patch for OSC 133 | #409 | none |
| 6 | Frame Windows commands with `%F4E%` markers | #409 | medium |
| 7 | `IsBusy` from the pseudoconsole process list | #409 | medium |
| 8 | Shell-session layer, echo suppression with carry buffer | all | high |
| 9 | Diagnose Ctrl+Break | #362 | none |

## 8. Still missing

* A debug log for the #409 scenario, after step 5.
* Confirmation that the Ctrl+Break report still reproduces on master, and in
  which build.
* A decision on whether Ctrl+N should start a new shell or share one, which
  determines the shape of the fix in section 5.