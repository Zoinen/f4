# Synchronize dirs

`Commands → Synchronize dirs` (action `Panel.SyncDirs`) compares the two panel
folders and then copies or deletes what differs. The reference is Total
Commander's window of the same name, including the meaning of its four
options; issue #612 asked for that behaviour specifically.

The command has no default key. Bind one in the hotkey configurator if you
use it often.

## What it compares

The comparison is the one in `internal/fileops/compare.go`, the same engine
the Advanced Compare dialog uses, so the two commands can never disagree
about whether two files are the same file. `config.SyncOptions.CompareOptions`
is the translation between the two option sets.

Files are compared, folders are not. A folder carries no content of its own,
and the folders a copy needs are created on the way. The one exception is a
file facing a folder of the same name: that pair is listed and locked, so the
reason nothing will happen to it is visible rather than silent.

Modification times are always compared with two seconds of slack. FAT stores
the time in two-second steps, so without it every file copied to a memory
card comes back as differing.

## Options

| Option | Meaning |
| --- | --- |
| Asymmetric | The left folder is the master and the right one is made to look like it: everything missing or different goes right, and what only the right side has is deleted there. Nothing else in the dialog produces a deletion by itself. |
| Subfolders | Compare the whole tree instead of the two folders' own files. |
| By content | Read the files whose size and time already match, to find the ones that only look equal. |
| Ignore date | Name and size are the whole truth. Total Commander documents the consequence, and f4 follows it: such a comparison can only answer "equal" or "not equal", so the direction is left to the user. With "by content" as well, files of the same size are still read. |
| File mask | A far2l-style mask, with an exclude section after `|`. It is matched against file names, not against paths. |

The options are remembered in the `[Sync]` section of `settings.ini`.

## The result window

Columns are: name, the left side's size and date, the direction, the right
side's date and size. The panel paths are shown in the option dialog that
opened the comparison.

The direction column is symbols rather than words, because it is four cells
wide and has to read the same in every language:

| Token | Meaning |
| --- | --- |
| `-->` | copy from the left to the right |
| `<--` | copy from the right to the left |
| `--×` | delete the file on the right |
| `×--` | delete the file on the left |
| `=` | the two files are equal; nothing to do |
| `!=` | they differ, and nothing in the result says which way to copy |
| `-` | a direction the user cleared |

Keys:

| Key | Effect |
| --- | --- |
| `Space` | step the row through the directions it can have |
| `→` / `←` | copy this row to the right / to the left |
| `Del` | delete this row's file; again to switch to the other side |
| `Backspace` | leave this row alone |
| `F3` | view the file |

The four checkboxes under the table hide and show whole groups — the same
`->`, `=`, `!=` and `<-` filters Total Commander puts in its toolbar, with the
count of each. They follow the comparison result rather than the current
direction, so clearing a row's direction does not make it vanish.

The line under them is what pressing **Synchronize** would do, and it is
recomputed on every change.

## Running the plan

**Synchronize** opens a confirmation showing the file counts and the number of
bytes, with Total Commander's three switches: left to right, right to left,
and delete. A switch that is off drops those rows from the run; it does not
change the rows themselves.

Deletions follow the global trash preference, exactly as `F8` does.

The run needs no scan of its own: the plan already knows every file and its
size, so it starts moving bytes immediately. Overwriting is not asked about
again — the window is where that question was answered, row by row.

Both panels are re-read when the run finishes. The window closes with the
run; comparing again after it is one keystroke, and re-reading a large tree
without being asked is not.

## Where the code lives

| Part | File |
| --- | --- |
| Pairing, the default directions, execution | `internal/fileops/sync.go` |
| Option dialog, result window, confirmation | `internal/app/sync_dirs_ui.go` |
| Persisted options | `internal/config` (`SyncOptions`, `[Sync]`) |

`internal/fileops/sync.go` is free of UI so that planning can be tested
against a temporary directory; `internal/fileops/sync_test.go` does that.
