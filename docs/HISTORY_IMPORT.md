# Importing Far Manager 3 history

Open **Commands > History > Import Far3 History**, or find **Import Far3
History** in the command palette (`Ctrl+Shift+P`). The action is also available
in the Hotkey Configurator as `History.ImportFar3` in the Shell area.

Enter the Far3 installation directory (for example `C:\Programs\Far3`), its
profile directory, or the full path to `history.db`, then choose **Import**.
The last successful location is remembered. Installation lookup follows
`Far.exe.ini`: portable profiles, `UserLocalProfileDir`, and the AppData system
profiles are supported. For a profile selected with Far's command-line options,
enter that profile directory directly.

The dialog works in the console and Qt frontend. It reads the database in
read-only mode, including committed entries in a live WAL, and merges:

- Commands, including their working directories and multiline text.
- Viewer and editor file history, retaining the latest opening mode.
- Local folder history.

Dates and locks are preserved. Matching records are merged by identity, keeping
the newest timestamp and either record's lock; repeated imports add no
duplicates. Existing F4 entries remain. Importing does not execute commands or
open files. Far plugin paths and external-launch history cannot be reopened
faithfully and are reported as skipped.

Large imports are not cut to the ordinary 100-entry limit. Subsequent history
updates retain the larger list size, replacing the oldest unlocked entries as
new ones arrive. Explicit history deletion still works normally.

The result dialog reports new commands, viewer/editor files, folders and skipped
records. Imported data is stored in F4's existing `history.json`.
