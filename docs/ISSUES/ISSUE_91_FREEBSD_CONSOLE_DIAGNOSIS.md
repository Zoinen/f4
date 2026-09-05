# Issue #91 follow-up — FreeBSD `sc` console, PTY in tmux, `CGO_ENABLED=0`

Diagnosis only; fixes are the next step. Everything below is established from
source (vtui, vtinput, f4 and FreeBSD `main`: `sys/teken`, `sys/dev/syscons`,
`sys/dev/vt`), not from a reproduction. Reporter runs `kern.vty=sc` and has
jails (cbsd/iocage) on the hosts involved.

## 1. The reports (August 2026)

1. On a remote server, ssh + tmux: toast "could not start the local terminal".
   Locally no toast.
2. System console (`sc`): "strong artifacts with blinking cursors". After
   vtui `7cef390` + f4 `7f3e130`: panel cursor and key bar lost their colour
   but still blink.
3. `CGO_ENABLED=0` build fails in
   `unxed/goffi@v0.1.4/internal/fakecgo/freebsd.go:23,24`.
4. Reporter suspects the May tests were on the *other* console (`vt`).

## 2. Facts: the two consoles

Both `sc` (syscons) and `vt` drive the same emulator, `sys/teken`. They differ
in rendering, and that difference explains the whole report 2.

| | `sc` (syscons) | `vt` |
|---|---|---|
| Text decoding | **8-bit**: GENERIC has no `TEKEN_UTF8`, so `scterm-teken.c` calls `teken_set_8bit()`; every UTF-8 byte is one cell of the 8-bit font (`ГöéГöé` on the May photo) | UTF-8 always |
| Background colours | 8. `scteken_te_to_sc_attr()` puts `bg & 8` into bit 7 of the VGA attribute; the text renderer (`scvgarndr.c`, `vga_flipattr(a, TRUE)`) treats bit 7 as **blink**, and `vga.c` never clears blink-enable in ATC 0x10 | 16, no blink (`vt_determine_colors`: `TF_BLINK` → light bg) |
| Bold | XORs fg bit 3 (`attr ^= 8`): bold + bright fg = dim fg | fg → light |
| Hardware cursor | VGA cursor, blinks by default | software cursor |

So on `sc` **any bright background blinks**: `ESC[100..107m`, `48;5;8..15`,
and `SGR 7` (reverse) combined with a bright *foreground* (teken swaps fg/bg
before syscons maps them). This is the "blinking cursors" and the still-blinking
panel cursor / key bar. It is not our text cursor and no cursor-shape sequence
will change it. The May tests looked better because they were on `vt` — the
reporter's suspicion in report 4 is consistent with the source.

## 3. Facts: what teken does with what we send

From `teken.c` / generated `teken_state.h` (`gensequences sequences`):

* Unknown CSI final byte, unknown DEC private mode: silently discarded. So
  `?2026`, `?1049`, `?1002/1003/1006/1015`, `?1004`, `?2004`, `?9001`,
  `ESC[>15u`, `DECSCUSR` (`teken_subr_set_cursor_style` is a TODO) are
  harmless no-ops. `?25h/l` **is** supported (`TP_SHOWCURSOR`), and
  `ESC[=0S/=1S` maps to the same thing — the vtui patch swapped equivalents.
* `OSC` / `DCS` payloads are discarded until `BEL` or `ESC` (`TS_INSTRING`).
  The claim in vtui `7cef390` that syscons prints the OSC 4 payload is wrong;
  the early return in `SetPalette` is harmless but was not the fix.
* **Printed as text** (real artifact sources, both consoles):
  * `ESC _ far2l1 ESC \` — teken has no APC; after `ESC _` the parser resets and
    `far2l1` lands on screen (`far2l0` on exit). Sent by
    `vtinput.Far2lExtensions` (in `DefaultProtocols`).
  * `ESC[<1u` — `<` is not a CSI prefix teken knows; `1u` is printed on exit.
  * Any CSI with ≥ 8 parameters: `T_NUMSIZE` is 8 (`teken.h:137`); the
    remainder is printed. That is exactly the May `0;0;160m` garbage
    (`ESC[0;38;2;R;G;B;48;2;0;0;160m`).
  * On `sc` only: every non-ASCII rune. `symbols.go`/`scrollbar.go` replace
    box drawing and scrollbar glyphs when `IsFreeBSDConsole`, nothing else;
    `cellAdvanceTrusted()` (screenbuf.go) wrongly trusts Cyrillic/Greek/box
    ranges as one column there, so one such glyph shifts the rest of the row.
* No alternate screen on either console (`47`/`1049` ignored): the screen is
  not restored on exit regardless of what we send.

## 4. What vtui emits today

* `detectColorProfile()` (screenbuf.go:28) → `ColorProfile16` for a bare
  FreeBSD tty (the check is duplicated twice, harmless). In that profile
  `ansi_writer.go` emits `100..107` for indices 8..15 and `7` for
  `CommonLvbReverse`; both blink on `sc` (§2).
* RGB attributes are quantised with `findNearestColor(rgb, activePal, 16)`
  against the first 16 entries of `ThemePalette` (xterm defaults with f4's
  0/7/8 overrides, `colors.go:60`). With the hard-coded palette in
  `SetDefaultF4Palette` the cursor (`0x00A0A0`) and key-bar label
  (`0x06989A`) land on index 6 — not bright. The shipped default style is
  **Radiola** (`config.go:477`, `ColorStyle`); its values were not checked
  here and are the first thing to check in step 2: whichever cells the
  reporter sees blinking are cells whose bg quantised to 8..15, or reverse
  video with a bright fg (`ansi_parser.go:972`, `info_panel.go:1050`).
* `IsFreeBSDConsole` (screenbuf.go:59) = freebsd && no
  `DISPLAY`/`SSH_CLIENT`/`TMUX`/`WAYLAND_DISPLAY`. It cannot tell `sc` from
  `vt`; `sysctl -n kern.vty` (or `kern.vt.*` presence) can. Computed once at
  init from the daemon's environment.
* "Colour disappeared" after `7cef390`: the same commit also dropped
  `ESC[?7l`, `seqResetPalette`, alt-screen and `seqBlinkingUnderline` on the
  console path (`modernVT`). Nothing there should change SGR output; to be
  confirmed by diffing a `VTUI_DEBUG` frame dump before/after in step 2. No
  action needed from the reporter for this.

## 5. Built-in terminal in ssh + tmux

* The toast is `Terminal.PTYAllocFailed`, raised only when `NewPTY()`
  (`pty_bsd.go`) fails; `initPTY()` logs the cause with `vtui.DebugLog` and
  `logPTYDiagnostics()` (`panels_frame.go`). Before `7f3e130` the toast did not
  include the cause; now it does — the exact text is needed first.
* `posix_openpt` vs `/dev/ptmx` (`7f3e130`) reach the same kernel allocator
  (`pts_alloc()` in `tty_pts.c`), so the switch cannot fix a limit. That
  allocator returns `EAGAIN` when `RLIMIT_NPTS` (`login.conf`
  `pseudoterminals`, `ulimit -p`) or the rctl `pseudoterminals` rule of a
  jail is exhausted — the strongest candidate for a cbsd/iocage host,
  especially with the known orphaned-shell leak noted in `pty_bsd.go`
  (each dead f4 leaves a shell holding a pts).
* Other candidates: devfs ruleset hiding `pts/*` in a jail (open of the slave
  fails, not the master); `TIOCSCTTY`/`Setctty` failing for a process that
  already has a controlling tty.
* The "built-in terminal" is the shell behind the command line under the
  panels (Ctrl+O shows it); the reporter never used it, which is why locally
  the failure is invisible except for the toast.

## 6. `CGO_ENABLED=0`

`cmd/compile` (`noder.go`) allows `//go:cgo_*` directives other than
`cgo_import_dynamic` only in `_cgo_*` files or the standard library.
`internal/fakecgo/freebsd.go` in `unxed/goffi@v0.1.4` copies
`runtime/cgo/freebsd.go`, which is permitted only because it is stdlib. With
the native `CGO_ENABLED=1` default the `!cgo` constraint hides the file, which
is why the reporter's normal build works. **Decision:** the goffi-based GUI
backend is not needed on FreeBSD (X11 is enough); step 2 excludes it there
with a build constraint instead of fixing fakecgo. CI must build FreeBSD with
`CGO_ENABLED=0` explicitly to keep this covered.

## 7. Data to request from the reporter

Console: `sysctl kern.vty`, `echo $TERM`, `vidcontrol -i mode`, whether
UEFI/BIOS boot, and the screen with `f4 --tty` (photo is fine). PTY: run
`f4 --debug --log /tmp/f4.log` inside tmux, reproduce, send the log (lines
containing `PTY`), plus `ulimit -p`, `rctl -hu jail:<name>` or
`rctl -l` if in a jail, and `ls /dev/pts | wc -l` before/after.

## 8. Step 2 plan

1. **Done** (vtui). `sc` colour policy: never emit bright bg or `SGR 7` on
   `sc` — quantise to 0..7 for backgrounds, drop reverse by swapping colours
   ourselves; also drop `SGR 1`, because `scteken_te_to_sc_attr()` XORs bit 3
   of the fg for `TF_BOLD` and bold over a bright colour comes out dark. Keep
   16 fg. Detect `sc` vs `vt` via `kern.vty`. See §9.
2. `sc` text policy: ASCII for every glyph, or map to CP437 bytes with 8-bit
   output; fix `cellAdvanceTrusted` for 8-bit consoles.
3. Do not send APC (`far2l1`) and `ESC[<1u` to a FreeBSD console.
4. Exclude the goffi GUI backend on FreeBSD; CI FreeBSD job with
   `CGO_ENABLED=0`.
5. PTY: reporter says the toast is gone; the follow-up report in its place
   is the `3P`/`3Q` one below.
6. **Done** (vtinput). `ESC [ O` is the prefix of both focus-out and the
   broken-SS3 function keys (`ESC [ O 3 P` = Alt+F1, `ESC [ O 3 Q` = Alt+F2).
   `reader.go` only checked `parseBuf[3]` when more than three bytes were
   buffered, so a read split after the third byte was taken for focus-out and
   the trailing `3P` came back as printable text — straight into the command
   line under the panels. It now waits for the fourth byte, and emits focus
   only if none arrives.

## 9. Measurements (FreeBSD 14.4, `kern.vty=sc`, BIOS mode 3)

Taken by writing each sequence to `/dev/ttyv0` and reading the VGA text
buffer at `0xB8000` back, so these are the attribute bytes syscons actually
produced, not an inference from the sources:

| written | attribute byte | |
|---|---|---|
| `ESC[101m` | `0xC7` | bright background |
| `ESC[5m` `ESC[41m` | `0xC7` | explicit blink — the same byte |
| `ESC[41m` | `0x47` | dark background, no blink |
| `ESC[7m` `ESC[91m` | `0xC0` | reverse over a bright fg: blink again |
| `ESC[1m` `ESC[91m` | `0x04` | bold cancels bright: dark red |
| `ESC[91m` | `0x0C` | a bright fg on its own is fine |

f4 on that console before the vtui fix: 54 cells at `0x87` (the key bar) and
18 at `0xE0` (the panel cursor), both with bit 7 set. After: the whole screen
is `0x00`/`0x17`/`0x30`/`0x70`/`0x7e`, nothing with bit 7.

Note for §2: the blink aliasing is specific to syscons in a *text* mode.
`scvgarndr.c` passes `TRUE` to `vga_flipattr`/`vga_cursorattr_adj` only from
the text renderers; the pixel-mode ones pass `FALSE` and render bit 7 as a
bright background. That, not `kern.vty`, is the likely difference between the
reporter's two `sc` hosts. Distinguishing the two needs a console ioctl, and
a dark background is a smaller loss than a blinking one, so the policy covers
syscons as a whole.

## 10. Still open

The reporter's "Alt-F1/F2 work once per tmux session, then print `3P`/`3Q`"
was not reproduced as a *once-per-session* effect: tmux 3.4 with `TERM=screen`
and with `xterm-256color`, three f4 runs in one session, opens the drive menu
every time. §8.6 fixes a mechanism that produces exactly that text, but does
not by itself explain the persistence. To close it, ask for `f4 --debug --log`
covering the moment it breaks, plus `tmux -V` and
`tmux show -g extended-keys`.
