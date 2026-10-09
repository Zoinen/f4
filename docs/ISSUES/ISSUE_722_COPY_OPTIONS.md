# Issue #722: the other copy and move options

## What was asked

Beyond access rights (`ISSUE_722_COPY_ACCESS_RIGHTS.md`) the issue asks for:

- an "Already existing files" choice: ask, overwrite, skip;
- "Copy contents of symbolic links", in the copy dialog only;
- an "Advanced options" button with "Ignore read errors" — with a number of
  read attempts, 1 by default, offered once the option is turned on — and
  "Ignore write errors". With either on, the operation has to end with a notice
  of how many files were copied and how many failed, and a button that opens a
  detailed log, kept in the temporary folder, in f4's viewer.

## Already existing files

Ask, Overwrite or Skip, for F5 and F6. The choice sets what the conflict
question's "remember" sets for the rest of an operation, so Ask is exactly the
old behaviour. It starts from Ask every time the dialog opens, as far2l's copy
dialog does (`far2l/src/copy.cpp` selects `CM_ASK` unless confirmations are
off): a remembered Overwrite would replace files in a copy it was not chosen
for.

A move keeps the source of an item whose copy skipped or failed anything. The
counter it looked at ran for the whole operation, so one skipped file kept the
sources of every later item as well; each item is now judged by what it left
behind.

## Copy contents of symbolic links

In the copy dialog only, kept for the session and on by default, because
following links is what f4 always did. Turned off, a link is recreated as a link
with the same unresolved target, so a relative link stays relative. That needs
both sides to have links (`vfs.SymlinkVFS`) and a target the source can read;
otherwise, a Windows junction for one, the contents are copied as before rather
than a target invented.

A move always carries a link as a link. Before, a move to another filesystem
copied what a link pointed at and then deleted the link.

## Advanced options

"Ignore read errors" covers opening a source file, reading it and reading a
folder; "Ignore write errors" covers creating folders and files, writing and
committing them. An ignored error records the item as failed, discards an
incomplete new file the way a failed copy always has, and goes on with the next
item. Cancellation and an outcome a remote provider could not confirm are never
ignored.

"Read attempts" counts failures in a row. A failed `Read` leaves the stream
position undefined, so the retry and every later read of that file address it
by offset; a read that returns data starts the count again. A failed open is
retried the same number of times.

Both options last for the session and stay out of the configuration: a copy
started without the dialog — Shift+F5, or F5 with its confirmation off — must
never ignore errors because of a choice made for another copy.

With either option on, the operation ends with a summary — files copied or
moved, failed, skipped, and how it ended if it did not finish — and a "View log"
button, in queue, background and foreground modes alike. The log is
`f4-copy-*.log` (`f4-move-*.log` for a move) in the temporary folder, written
line by line: the options, every file with OK, SKIPPED or FAILED and the error,
and every retry.

## Tests

`internal/fileops/copy_options_test.go`: read attempts (a retry by offset that
finishes the file, the last attempt giving up, no retry without the option, a
write failure told apart from a read failure); an unreadable file failing alone
while the rest of its folder is copied; a link copied as a link and as contents;
the options applied to the state of an operation.
