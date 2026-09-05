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
