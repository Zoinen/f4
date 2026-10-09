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

The in-place item editor (F4 in the user menu) places the hotkey and item
label below their captions. The command field shows six lines, with a
separator above the command section and another above the Save/Cancel buttons.
Submenu entries omit the command section and keep the button separator.
The frame includes a close button. Save/Cancel remain centered in the visible
dialog width when the terminal is resized, including narrow viewports.
When the dialog is enlarged, Label and Commands stretch horizontally together
with the separators. Commands also expands vertically; the buttons and their
separator remain anchored to the bottom of the frame.
The minimum width is 40 columns. The command editor can shrink from its initial
16 rows to 11, leaving one editable command row with its caption above it.
The submenu editor retains an eight-row minimum.
