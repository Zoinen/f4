# Issue #249 solution review

Reported against Linux Mint Cinnamon amd64: run `mc` in the built-in terminal,
press `Ctrl+O`, and mc disappears behind the panels; further `Ctrl+O` presses
do nothing, then draw a blue line, then return to f4. A blue stripe runs the
full width under the terminal output.

The report is three separate defects. Each was pinned to an observed cause
before anything was changed; the evidence is quoted with each one.

## 1. A program that paints once is never drawn

`TerminalRedrawScheduler` (`internal/terminal/redraw.go`, added for #248) is a
leading-edge coalescer: the first request of a burst calls `Redraw`, every
request in the following 16 ms is dropped, and nothing is drawn afterwards.
The dropped requests carry the newest terminal state, so if the last output of
a burst lands inside that window and the program then falls silent, the frame
holding that output is never rendered.

That is exactly mc's first screen. An instrumented build, logging every PTY
chunk and every scheduler decision:

```
16.017 parsed 87 bytes        (ESC[?1049h / l / h -- the screen is still empty)
16.017 request FIRED          -> the empty screen is what gets drawn
16.022 parsed 2256 bytes      request DROPPED
16.022 parsed 1653 bytes      request DROPPED
16.023 parsed 32 / 2048 / 1493 / 48 bytes   all DROPPED
```

mc then waits for input, and a byte-level capture of what f4 wrote to the host
terminal shows nothing at all until the next keypress. htop and vim hide the
defect because they repaint on a timer, so the next burst's leading edge shows
the previous state; a program that draws one screen and stops does not.

**Fix:** give the coalescer a trailing edge. Requests arriving inside the
window set a flag; when the window closes, one trailing frame is drawn and a
new window opens for whatever arrives during it. The cost of a sustained
stream is unchanged -- at most one frame per interval -- and the last chunk of
a burst can no longer be left unseen. Rejected alternatives: dropping the
coalescer (reintroduces #248), and shortening the interval (narrows the race
without closing it).

## 2. Ctrl+O is arbitrated by the alternate screen -- and stays that way

`Panel.Toggle` is bound as `CtrlO:NoAltScreenApp`, so f4 claims the key
whenever the program in the terminal is not on the alternate screen. mc's own
`Ctrl+O` leaves the alternate screen to show its subshell -- so the first press
reaches mc, and the second is taken by f4, which shows the panels over a
still-running mc. The trace:

```
01.605  AltScreen enabled: false            (press 1 reached mc)
03.706  HOTKEY: Executing action Panel.Toggle for CtrlO   (press 2 taken by f4)
```

Everything after that in the report follows from the same rule. Typing `mc`
again starts a second mc inside the first one's subshell, which reports that
mc is already running and continues without subshell support; its `Ctrl+O` has
no console to show and only flips the screen buffer, so the presses appear to
do nothing until the flip happens to leave the alternate screen and f4 takes
the key again.

The obvious repair -- gate the key with `NoTerminalApp`, the way Far, far2l and
mc all treat the panel toggle as a prompt-level key -- was tried and rejected:
issue #50 requires the opposite. A blocked CLI tool or a GUI program that holds
the PTY never gives the key back, and f4 would have no way out of the terminal
at all; `TestPanelsFrame_KeyHandling` pins that requirement. Being locked out of
the panels is the worse failure, so the binding stays as it is and this part of
the report is accepted behaviour, not a defect. Returning to mc after the
panels come back is `Ctrl+O` again -- mc keeps running the whole time.

## 3. The blue stripe under the output

While a program is on the alternate screen the terminal view covers the last
row; when the program leaves it the layout shrinks the view by one row to
reserve the keybar row, deliberately, so that starting and ending a command
does not resize the PTY. But the keybar stands down while a child is busy
(`frame.go`), and so does the command line, so nothing painted that row and
vtui's `Desktop` filled it with `Palette[ColDesktopBackground]`. f4's own
output confirms it wrote the row itself:

```
ESC[30;1H ESC[38;5;250;48;5;19m <100 spaces>
```

A control program that paints the alternate screen **red** and then leaves it
produces a **blue** row, which rules out leftover pixels from the program.

**Fix:** with the panels hidden and the terminal owned by an alternate-screen
or busy child, fill the rows between the terminal view and the bottom of the
screen with the terminal background, so the console reads as one surface --
which is what Far shows on `Ctrl+O` and what mc shows for its subshell.
Rejected alternative: giving the row back to the PTY, which would resize the
child on every command start and end, the thing the reservation exists to
prevent.

## Validation

- `TestTerminalRedrawSchedulerCoalescesBurst` now pins leading frame, exactly
  one trailing frame, and silence afterwards, including after `Stop`.
- `TestTerminalRedrawSchedulerFlushesLastChunk_Issue249` replays the shape of
  mc's first paint -- several chunks inside one window, then silence -- and
  requires the last state to reach the renderer.
- `TestPanelToggle_CtrlOBelongsToRunningProgram_Issue249` covers all four
  states: alt-screen program, running program off the alternate screen, idle
  prompt, panels shown.
- `TestUnpaintedTerminalRows_Issue249` covers the row arithmetic for busy,
  idle, and alt-screen layouts.
- Manual replay of the report's 13 steps under a scripted pty: mc is visible as
  soon as it starts, `Ctrl+O` toggles mc's own subshell view in both
  directions, the panels no longer appear over a running mc, and the bottom row
  is terminal background rather than desktop blue. With the panels hidden and
  the shell idle, `Ctrl+O` still shows the panels.

## Risk review

The scheduler change cannot lose or reorder PTY bytes: parsing is untouched and
only the wake-up policy changes. The trailing chain ends as soon as a window
closes with no pending request, and `Stop` clears both flags, so a closing
frame cannot be woken by a late timer. The binding change makes `Ctrl+O` behave
like `Esc` already did, and `ShellModeSimpleInline` keeps its unconditional
toggle, so the Wine path in `docs/WINE.md` is unaffected. The row fill draws
only rows no other widget claims and only while the panels are hidden.
