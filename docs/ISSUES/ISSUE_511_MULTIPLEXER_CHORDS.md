# Issue #511 solution review

## Report and current behavior

The report asks for two things. Terminal multiplexers (tmux, zellij, screen,
dvtm) run their commands from Ctrl/Alt chords, and with f4 in the foreground
the keypress may not survive the trip, so f4's own control combinations should
be at least partially overridable — or the modifier changeable. Separately,
keyboards without an F1-F12 row cannot send the function keys f4 is built
around.

Most of the first half already works. `HotkeyManager` overlays `hotkeys.ini`
onto defaults taken from the action registry, the Hotkey Configurator writes
that file, and every registered command can be rebound per area. The gaps are
narrower than the report suggests but real:

- The manager binds *actions*. Framework shortcuts (`Ctrl+Tab`), dialog keys,
  menu keys and the editor's own bindings are consumed before or beside it and
  cannot be moved at all.
- "Change the modifier" has no compact form: it is one entry per key, per
  modifier row, per area.
- The F-row case is the same problem repeated 48 times.
- The chords that actually collide are f4 defaults: `CtrlB` (tmux prefix),
  `CtrlA` (screen prefix), `CtrlO`, `CtrlP`, `CtrlG`, `CtrlN` (zellij).

## Three-pass review

### Pass 1: extend `hotkeys.ini` with alias entries

Let a binding name another key instead of an action, resolved inside
`HotkeyManager.GetAction`. Reject: the resolution would only run where the
hotkey manager runs, which is precisely the set of keys that is already
configurable. Keys owned by vtui, by dialogs or by the editor — the actual gap
— would still be unreachable, and `HotkeyManager.Save` rewrites the file
wholesale from binding diffs, so alias entries would need a second, parallel
persistence path in the same file.

### Pass 2: teach the input backends a modifier-swap option

Add a setting that rewrites Ctrl to Ctrl+Alt (or similar) in the terminal
translation layer. Reject: the collision is per-chord, not per-modifier — a
zellij user needs `Ctrl+P` and `Ctrl+O` moved while `Ctrl+C` and `Ctrl+D` must
stay where the shell expects them. A global swap trades one conflict set for
another, and it would sit below `TranslateInput`, where the same rewrite would
also hit keys forwarded to a child process.

### Pass 3: a substitution table in front of the input filter

Add `keymap.ini`, read into a `KeyRemap` table, applied at the top of
`MacroManager.Filter`. That filter is the single choke point every real
keystroke passes through, and vtui hands it the same `*vtinput.InputEvent`
that it later dispatches to frames, so rewriting in place reaches macros,
plugin interception, configurable hotkeys and the frames themselves with one
substitution. Source spellings come from `EventToHotkeyString` (the same
strings the Hotkey Configurator shows) and targets are parsed by `ParseFarKey`
(the same parser macro playback uses), so no third key vocabulary appears. A
trailing `*` on both sides rewrites a modifier prefix, which is the report's
"change the modifier" in one line. Select this pass.

## Safety checks

Substitution is applied once and never re-entered, so rules cannot chain or
loop. `keymap.ini` is its own file: `HotkeyManager.Save` rewrites `hotkeys.ini`
in full, and a section living there would be lost on the next save from the
dialog.

Remapping is suspended while the panels are hidden and an AltScreen program or
a busy PTY owns the keyboard, matching the handover the `NoAltScreenApp` and
`NoTerminalApp` conditions already make; otherwise vim or htop would receive a
chord the user never pressed. Bare modifier presses are excluded for the same
reason — the key bar and the terminal forwarder track them separately. Lock
states in `ControlKeyState` describe the keyboard rather than the chord and are
carried over untouched, while the scan code, which named the physical key, is
cleared as macro-injected events already leave it.

An absent or fully commented `keymap.ini` produces an empty table that `Apply`
short-circuits on before building any key string, so users who never touch the
file pay nothing per keystroke. The shipped sample is inert by construction and
a test asserts it: f4's INI reader has no notion of comments, so `;Alt1=F1`
would otherwise register as a rule with a `;Alt1` source.

Unit tests cover area precedence, `RCtrl` falling back to a `Ctrl` rule,
longest-prefix wildcard order, exact rules winning over wildcards, rejection of
one-sided `*` rules, in-place event rewriting including preserved lock state,
the absence of chaining, and bare-modifier and non-key events passing through.
Validation under zellij, tmux and screen, as the report asks, remains a manual
check.

## Follow-up after the first round of testing

Testing under zellij on `a11f504d` found the layer working for plain `Ctrl`
chords and failing for everything the report's own sample file demonstrated.
Four separate causes, all in the same handful of lines.

**A note on a rule was parsed as part of the rule.** f4's INI reader keeps
everything after the `=`, so `AltShift1=ShiftF1   ; the Shift row works the
same way` reached `ParseFarKey` with the note attached. `ShiftF1   ; ...` does
not parse as a function key — `strconv.Atoi` rejects the tail — and the
fall-through named the letter `F` instead, which is why enabling that line
appeared to redraw something and change nothing. The wildcard sample was worse:
`Ctrl*      ; every Ctrl chord...` no longer ends in `*`, so the rule was
rejected outright as one-sided. Both sides of a rule now stop at a `;` or `#`
that begins a field or follows whitespace, which still leaves `Alt;=F1` naming
the semicolon key.

**A shifted key had no single name.** Terminals cannot report Shift separately
for a printable key: `Alt+Shift+1` arrives as ESC `!` and spells as `Alt!`.
Under the kitty protocol the same chord spells `AltShift!`, and a backend with
virtual keys spells it `AltShift1`. No rule could match all three, and a
multiplexer stripping the protocol negotiation changes which one is in force.
`canonicalKeySpelling` now folds a US-layout shifted character back onto its
key, so the three names collapse to `AltShift1` on both sides of the table.

**Modifier order was significant.** Sources were stored lower-cased but not
reordered, so `ShiftAlt1` never matched an event that spelled itself
`AltShift1`. Canonicalisation now applies to sources and wildcard prefixes, not
only to targets.

**The sample pointed at things that do not exist.** `Options -> Key bindings`
is `Options > Hotkey Configuration`; `Ctrl+P` toggles the passive panel while
the command palette is `Ctrl+Shift+P`, so the sample was telling users to
recover a key onto the wrong command. The file also had every `[Common]` header
commented out, so uncommenting a rule under one left it in no section at all —
`ParseIni` drops such lines — and it now ships with one live header.

Two limits are documented rather than fixed, because they are not f4's to fix.
`Ctrl` does nothing to a digit in a plain terminal, so `Ctrl+1`, `Alt+1` and
`Ctrl+Alt+1` are the same bytes and `CtrlAlt0=F11` cannot be separated from
`Alt0=F10`; that collision is what made a remapped exit key intermittent. And
`=` cannot appear on the left of a rule, since the first `=` of the line
separates the two sides.

The report's remaining points were documentation, not behaviour: the README
said nothing about remapping, and the Hotkey Configurator's *Assign* button —
which does reassign keys, by waiting for the chord — was not mentioned
anywhere. Both are covered now, together with the command palette, which
answers "the multiplexer ate my shortcut" without any configuration at all and
should be the first thing a user reaches for.
