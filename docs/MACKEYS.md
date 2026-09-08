# Mac keyboard mode

macOS spells the text-editing chords differently from every other platform f4
runs on. `Cmd` is the shortcut modifier, `Opt` is where word navigation lives,
and an arrow key with `Cmd` goes to the edge of the line or of the document —
never a word. Far's layout gives several of those keys other meanings, so a
Mac user pressing `Cmd+Left` to reach the start of the line jumps a word
instead.

Mac keyboard mode rewrites the macOS chords into the Far chords that mean the
same thing, as the keystroke arrives, before anything in f4 has looked at it.
It is the same layer `keymap.ini` works on, one step later: an explicit rule of
your own always wins over the built-in table.

## The setting

```ini
[Interface]
MacKeyboard = auto
```

| Value  | Meaning                                                     |
| ------ | ----------------------------------------------------------- |
| `auto` | on when f4 is running on macOS, off everywhere else (default)|
| `on`   | always on — for an Apple keyboard plugged into a PC          |
| `off`  | always off — keep the Far layout on macOS too                |

Options → Mac keyboard toggles the same setting from inside f4, and
`F4_INTERFACE_MAC_KEYBOARD=off` overrides it for a single run.

## What it does

| You press     | f4 sees      | Result                       |
| ------------- | ------------ | ---------------------------- |
| `Cmd+←` / `→` | `Home`/`End` | start / end of the line      |
| `Cmd+↑` / `↓` | `Ctrl+Home`/`Ctrl+End` | start / end of the file |
| `Opt+←` / `→` | `Ctrl+←`/`→` | previous / next word         |

Each of them also answers with `Shift` held, which extends the selection the
way it does on the rest of the system.

`Cmd+C`, `Cmd+V`, `Cmd+X`, `Cmd+Z`, `Cmd+A`, `Cmd+S`, `Cmd+F` need nothing from
this mode: the GUI backend already delivers Command as Ctrl, so they reach f4
as the `Ctrl` shortcuts they should be.

## What it does not do

* **The panels keep the Far layout.** The rules apply in the editor and in
  dialog input fields only. On the panels `Alt+←`/`→` still scroll long file
  names and walk the folder history, and `Ctrl+←`/`→` still move the split —
  those are f4 commands, not text editing, and the key bar and the help
  promise them.
* **The physical `Control` key is untouched.** `Ctrl+←` is still a word jump
  everywhere, so Far muscle memory keeps working next to the Mac one rather
  than instead of it. See below for why both can be true at once.
* **`Opt+Backspace` is left alone.** macOS deletes the word to the left with
  it; f4 has no command for that to map onto.

## Command in a terminal

Two things have to be true before the `Cmd` rules can run, and only one of them
is about the setting.

In the **GUI window** the backend separates the two modifiers the way far2l
does: both Command keys arrive on the left Ctrl channel and the physical
Control key on the right one. That split is what lets f4 rewrite `Cmd+Left`
into `Home` while leaving `Ctrl+Left` a word jump.

In a **terminal** there is no split. Terminal.app and iTerm2 do not send
Command at all, and what does arrive on the left Ctrl channel there is the
physical Control key. Rewriting it would break the Far layout for nothing, so
the `Cmd` rules are skipped when f4 cannot tell the two apart — `MacKeyboard =
on` does not force them. The `Opt` rules have no such problem: Option reaches
f4 as Alt on every backend, so word navigation works in a terminal too.

## See also

[`keymap.ini`](KEYMAP.md) rewrites any key into any other and is the tool to
reach for when this table is not the layout you want.
