# User menu scripts

F2 user-menu entries can contain a complete script instead of a list of
one-line commands. Put a standard shebang on the first line of the command
field; f4 removes that line, feeds the remaining script to the selected
interpreter through the normal command window, and keeps all existing panel
substitutions such as `!.!`:

```text
#!/usr/bin/env bash
printf 'current file: %s\\n' !.!
```

Every menu item may select a different interpreter. Examples include
`#!/usr/bin/env bash`, `#!/usr/bin/python3 -u`, `#!/usr/bin/perl -w`, and
`#!/usr/bin/env node`. On POSIX command windows the script is shell-quoted and
sent through stdin, so it also works when the active panel is a remote POSIX
PTY. On Windows f4 creates a temporary script file, runs the selected
interpreter, and removes the file afterward.

Items without a shebang retain the existing FarMenu-compatible behavior: each
non-comment command line is sent to the command window in sequence.

## Importing Far Manager 3 menus

Open **Settings > User menus & macros > Import Far Manager 3 user menu** to
merge a menu into the global-menu draft. Review or edit the imported entries,
then choose **Apply** to save them; **Cancel** discards the draft.

**Commands > Import Far Manager 3 User Menu** opens the same path dialog and
saves directly to the global F4 user menu. The command palette and hotkey
configurator expose it as `UserMenu.ImportFar3` in the Shell area.

Enter a `FarMenu.ini` file, a Far profile directory, or an installation folder
(or `Far.exe`). Installation lookup checks its global `FarMenu.ini` first,
then the roaming profile selected by `Far.exe.ini`: `%APPDATA%/Far Manager`
for system profiles, or `UserProfileDir` / `%FARHOME%/Profile` for portable
profiles. An explicit file selects a local or custom menu unambiguously.

Import preserves order, nested and empty submenus, separator labels, hotkeys,
and multiline command strings, including Far substitutions. UTF-8 and
BOM-marked UTF-16/UTF-32 are accepted. Unsupported encodings produce an error;
resave such a file as UTF-8 in Far before importing. Existing entries remain,
identical complete entries are skipped, and repeated imports add no duplicates.
Conflicting hotkeys are retained for you to edit. Import does not execute
commands or rewrite platform-specific tools and paths.

The source format and menu locations follow
[Far Manager's user menu implementation](https://github.com/FarGroup/FarManager/blob/master/far/usermenu.cpp).

## Editing one menu entry

Press **F4** on an entry to open its own editor. This uses the same fields and
save validation as Settings, with no settings categories, search, or list of
other records. The complete owning menu stays in the transaction, preserving
siblings and nested separators. **Cancel** discards edits; **Apply** or **OK**
saves to the captured menu source. Insert uses this editor for a new item.

Repeated identical separators are structural boundaries and are retained by
Far3 import. Reimporting the same menu also restores separators omitted by the
older importer, without duplicating the unchanged commands.
