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

## No Insert key

MacBooks and most Apple keyboards have no `Insert` key, and Far puts a lot on
it. Most of it has another way in that f4 already understands on every
platform, with or without Mac keyboard mode:

| Far key                          | Without Insert                                   |
| -------------------------------- | ------------------------------------------------ |
| `Ins` on the panels              | `Shift+↑` / `Shift+↓`, and `Shift+PgUp`, `Shift+PgDn`, `Shift+Home`, `Shift+End` for a range |
| `Ctrl+Ins` / `Shift+Ins` in the editor | `Cmd+C` / `Cmd+V`                          |

In the GUI window two keys also arrive as `Insert` itself, the way far2l
treats them: `Help` on older full-size Apple keyboards, and `0` on the numeric
keypad (macOS reports no NumLock, so the keypad stays in navigation mode;
`Shift` turns the key back into the keypad digit).

The remaining `Insert` chords have no second key by default: `Ctrl+Ins`,
`Ctrl+Shift+Ins`, `Alt+Shift+Ins` and `Ctrl+Alt+Ins` on the panels copy names
and paths, `Alt+Ins` starts the screen grabber, and `Ins` in the editor
toggles overtype. Run them by name from the command palette (`Ctrl+Shift+P`),
or give them a key of your own in `Options > Hotkey Configuration` or in
[`keymap.ini`](KEYMAP.md).

Some menus take `Ins` on its own. In the user menu (`F2`) it adds an item, and
`Ctrl+N` does the same there. In the Bookmarks dialog (`F9 > Commands`) it
stores the panel's folder in a slot, and in the folders history it pins a
folder; neither has a second key. These menu keys are not in the Hotkey
Configurator, but `keymap.ini` reaches them, since it substitutes the key
before any window sees it. In the GUI window this rule gives the physical
Control key an `Insert` everywhere, menus included, and leaves `Command+I`
as it was:

```ini
[Common]
RCtrlI=Ins
```

Under `[Common]` the rule also applies at a shell prompt, where `Control+I`
is otherwise a Tab; list the areas you want (`[Shell]`, `[Menu]`, `[Dialog]`,
`[Editor]`) instead if you use it that way.

In the GUI window a chord of your own can use the `Control` key without taking
anything from `Command`: Command chords are spelled `Ctrl`, the physical
Control key is spelled `RCtrl`, and a binding on `RCtrl` answers only to
Control. A Control chord with no binding of its own still does what the
matching `Ctrl` chord does. In a terminal there is no such split — see above.

## See also

[`keymap.ini`](KEYMAP.md) rewrites any key into any other and is the tool to
reach for when this table is not the layout you want.
