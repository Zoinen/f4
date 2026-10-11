# Git client

f4's built-in git client (f4#659): status, diffs, staging, log and branches,
wrapping the host's own `git` binary the way `plugins/multiarc` wraps
archive tools (f4#609) rather than linking a Go git implementation.

## What this first part does (f4#659 part 1 of N)

- Registers a `vfs.PanelProvider` (`f4.gitstatus`) -- like `plugins/proclist`
  (f4#312), this plugin's first consumer of that API. Opening it (menu,
  command palette, or **Commands -> Git status** / **Ctrl+Alt+G**) replaces
  the active panel slot with a read-only list of `git status`'s changed,
  unmerged and untracked paths for that panel's current directory.
- The **Status** column is the same two-letter code `git status --short`
  prints (`M `, ` M`, `A `, `??`, `R `, `UU`, ...) rather than a f4-invented
  vocabulary, so anyone who already knows `git status -s` recognizes it
  immediately. A rename or copy shows as `old -> new` in the **Path** column.
- **F5** re-runs `git status` and replaces the list. There is no background
  refresh: unlike ProcList's live `/proc` view, a git status is a
  point-in-time snapshot the user asks for, not something that needs a
  ticker.
- The panel's own keys (F5 here, and Enter, Insert, Ctrl+K, Ctrl+E and
  Ctrl+S from the parts below) are declared through the host's shared
  panel-plugin key primitive (`PanelKeys`, `vfs.PanelKeyProvider`; see
  `docs/PLUGINS.md`, "Panel-only plugins"), not switched on in `ProcessKey`.
  That makes them win over the file panel's own F5/Insert/Enter while the
  status panel has the focus and puts **F5 Refresh** on the keybar (the
  other keys have no keybar row). Enter and Insert are disabled -- consumed,
  nothing runs -- while the list is empty; Ctrl+K stays enabled with nothing
  staged so it can still say so in a toast.
- Sortable by either column (click a header) and has type-to-filter
  (`vtui.Table.QuickSearch`).
- If the active panel's directory is not inside a git repository (or `git`
  is not on `PATH`), opening the panel fails and the host shows git's own
  error as a toast (`internal/panel/plugins.go`'s existing
  `PanelProvider.Open` error handling) -- nothing extra to build here.

## Part 2: diffing a changed file (`diff.go`)

- **Enter** on a listed entry opens a side-by-side diff (`internal/diffview`,
  f4#613) of that one file: its `HEAD` content on the left, its current
  working-tree content on the right -- the exact same widget
  `internal/app/compare_content_ui.go` already uses for f4's plain "Compare
  files by content", per the scope split agreed in f4#613.
- A file with no `HEAD` version yet (new/untracked, or an unborn branch) just
  shows an empty left side, so the whole file appears inserted -- the same
  shape `git diff --no-index /dev/null <file>` would produce. A file deleted
  from the worktree since the panel last loaded shows the mirror image: an
  empty right side. A rename or copy diffs the old name's `HEAD` content
  against the new name's worktree content.
- Same guards as "Compare files by content": a side over 8 MiB, or one that
  looks binary (a NUL byte), is refused with a message instead of diffed.

## Part 3: staging and unstaging a whole file (`stage.go`)

- **Insert** on a listed entry stages it (`git add`) if nothing about it is
  staged yet -- an unstaged modification, a deletion, or an untracked file
  (`??`) -- or unstages it (`git restore --staged`) if the index already has
  something staged for it. This is a toggle per file, not per hunk: staging
  part of a file's changes is a follow-up part of f4#659 (it needs the diff
  widget from f4#613 to show and select hunks, not just the two-letter
  status this panel already displays).
- A merge conflict (`DD`, `AU`, `UD`, `UA`, `DU`, `AA` or `UU`) always runs
  `git add`, regardless of which of those seven codes it is: the only
  sensible action from this panel is marking it resolved, and there is
  nothing meaningful to "unstage" from a conflict.
- A rename or copy runs the command against *both* the old and new names,
  not just the one this panel displays: git records a rename as two separate
  index changes (an add and a delete) that `git status` only *displays*
  combined into one line, so touching only the new name would stage or
  unstage half of it and leave the other half behind.
- Insert was chosen over the Space key some other git clients (`lazygit`,
  `tig`) use for the same gesture: this table's `QuickSearch` (like every
  other `vtui.Table` in f4) claims every printable character while focused,
  Space included, so Space here would type into the filter instead of
  staging anything. Insert is f4's own existing "mark an item" key in a
  Far/Norton-Commander-style file panel (see `isAddItemKey` in
  `internal/panel/menukeys.go`), so it also fits this panel's own
  vocabulary better than a foreign tool's convention would.
- After a successful stage/unstage, the panel reloads (the same `git status`
  call F5 runs) and moves the cursor back to the same path, so toggling
  several files in a row by stepping down the list does not keep resetting
  the cursor to the top of a re-sorted table. A failed git command leaves
  the panel untouched and shows the failure as a toast, the same way a
  failed F5 refresh already does.

## Part 4: committing staged changes (`commit.go`)

- **Ctrl+K** opens a one-line commit message prompt over whatever Insert
  (part 3) has already staged, reusing `internal/dialog.FileInputBox` -- the
  same single-line input dialog `internal/app/actions.go`'s Rename command
  already builds its own prompt from -- rather than composing a new
  `vtui.Window`/`vtui.Edit` pair from scratch for the same shape of dialog.
  Confirming it runs `git commit -m "<message>"` over the index and reloads
  the panel; a leading/trailing-whitespace-only message is rejected the same
  way an empty one is, with a toast, before any commit runs.
- Nothing staged means nothing for `git commit` to record: pressing Ctrl+K
  with an empty index shows a toast ("nothing staged to commit") instead of
  opening a dialog the user would only have to cancel.
- Ctrl+K, not a bare letter: this table's `QuickSearch` claims every
  printable character while focused (the same reason Insert, not Space, was
  chosen for staging in part 3), and Ctrl+K is not already one of f4's own
  action hotkeys anywhere else in the application.
- Deliberately minimal for this first cut: a single-line message only (no
  multi-line body, no `--amend`, no commit signature/author override). Those
  are their own follow-up parts if and when they turn out to be needed --
  see the ticket.

## Part 5: a read-only commit log (`log.go`, `logview.go`, `logdiff.go`)

- **Ctrl+E** on the status panel opens a read-only list of the repository's
  last 200 commits (hash, author, date, subject) -- `git log
  --pretty=format:...`, parsed once by `parseLog` (log.go). Not Ctrl+L, the
  more obvious mnemonic ("Log"): this panel is itself one of `PanelsFrame`'s
  `AltPanels`, and Ctrl+L is already the global **Info Panel** toggle, whose
  own key handling deliberately falls through past *any* focused AltPanel so
  it still works no matter what the active side is showing -- claiming it
  here for something unrelated would break that. Ctrl+G ("Git") is likewise
  already the global **Apply command**. Ctrl+E is free (see panel.go's
  `ProcessKey` doc comment for the full reasoning) and, unlike Ctrl+I or
  Ctrl+J, is not a letter whose Ctrl form some terminals conflate with a
  plain control character (Tab, Line Feed).
- It opens as its own full-screen `vtui.Frame`
  (`vtui.FrameManager.AddScreen`), the same way Enter's diff (part 2) already
  does, rather than a second `vfs.PanelProvider` replacing the status panel
  in its slot: that needs no "go back to what was open before" stack of its
  own, and Escape simply pops back to the status panel underneath, exactly
  as closing a diff already does.
- **F5** re-runs `git log` and replaces the list, the same point-in-time
  refresh the status panel's own F5 is. Sorting is not offered -- a commit
  log's only meaningful order is the one `git log` already produced -- but
  QuickSearch is, the same type-to-filter gesture the status panel has,
  useful here for jumping to a commit by author or by a word from its
  subject.
- **Enter** on a commit shows a side-by-side diff, reusing
  `internal/diffview` (f4#613) exactly as the status panel's own Enter
  (part 2) does -- against the commit's parent and the commit itself
  (`<hash>^` and `<hash>`) instead of HEAD and the worktree. This first
  version only handles a commit that changes exactly one file:
  `diffview.DiffView` takes two whole files, not a multi-file patch, and
  picking one file out of several (or rendering a patch-shaped view instead)
  is its own follow-up. A commit with zero changed files (a merge commit,
  which `git show` does not diff without `-m`/`-c`) or more than one shows a
  toast instead of guessing.

## Part 7: switching branches (`branch.go`, `branchview.go`)

- **Ctrl+S** on the status panel opens a read-only list of local branches
  (`git branch --list --no-color`, parsed by `parseBranchList` in
  `branch.go`), the current one marked with `*` in its own column -- the
  same `vtui.BorderedFrame`+`vtui.Table` full-screen-frame shape Ctrl+E's log
  (part 5) already uses, for the same reason: it needs no panel-provider "go
  back to what was open before" stack of its own, so Escape/F10 simply pops
  it and the status panel underneath is exactly as it was. Not Ctrl+B (the
  more obvious mnemonic, "Branch"): plain `B` is claimed by this table's own
  QuickSearch like every letter, and Ctrl+B is already the global
  **Panel.ToggleKeyBar**, which this panel's own key handling has no
  fallthrough exception for (unlike Ctrl+L/Ctrl+G, see panel.go's
  `ProcessKey` doc comment) -- claiming it here would silently break the key
  bar toggle while this view is open. Ctrl+S ("**S**witch branch") is free by
  the same `grep DefaultKeys` check that justified Ctrl+K and Ctrl+E, and is
  already precedent for a view-local Ctrl+S: `internal/media/image_view.go`
  binds it to that view's own slide-show toggle, scoped the same way.
- **Enter** on a branch runs `git switch <branch>` (not `git checkout`: the
  newer, purpose-built command gives a clearer refusal than checkout's own
  more overloaded one when the working tree has changes a switch would
  overwrite) and, on success, reloads both this list (moving the `*` to the
  new current branch) and the status panel underneath (so its title and
  entries reflect the new HEAD). The view itself stays open afterward --
  switching branches does not imply "done looking at branches," the same way
  showing a diff from the log view does not close the log.
- A working tree with local changes that switching would overwrite is not
  detected ahead of time by this plugin: `git switch` itself refuses with
  its own explanatory message in that case, which surfaces here as a toast
  exactly like any other failed git command in this plugin (`toggleStage`,
  `runCommit`) -- there is no attempt here to stash or merge on the user's
  behalf.
- **F5** re-runs `git branch --list` and replaces the list, the same
  point-in-time refresh every other list in this plugin has. Sorting is not
  offered (a short branch list has no order worth resorting away from), but
  QuickSearch is, the same type-to-filter gesture the status and log views
  already have.

## Part 9: a multi-line commit message editor (`commit.go`)

- **Ctrl+K** now opens `showCommitMessageEditor`, a `vtui.MultiLineEdit`
  field, instead of part 4's one-line `internal/dialog.FileInputBox` prompt
  -- the "editor for commit messages" the ticket asked for from the start,
  which a single-line field could never really be: a message with its own
  subject line, a blank line, and a longer body (the shape git itself
  expects from an `$EDITOR`-composed `COMMIT_EDITMSG`) had nowhere to go
  before this. Composed by hand from `vtui.NewCenteredDialog`,
  `vtui.NewButton` and `vtui.NewHBoxLayout`, the same way
  `plugins/envman/dialogs.go`'s own profile dialog builds its own
  `MultiLineEdit` field, rather than through `internal/dialog.FileDialog`:
  that helper's fixed heights are sized for its own family of file dialogs
  (copy/move/rename), not a resizable paragraph of text.
- Confirming (the **Ok** button, or its own mnemonic) runs `git commit -m
  "<message>"` exactly as part 4 did -- `-m`'s argument reaches git through
  `exec.Cmd`'s own argv, never a shell, so an embedded newline needs no
  escaping and git records the message verbatim (after its own
  `commit.cleanup=strip`, the same cleanup a message typed into `$EDITOR`
  would get either way). Only the dialog changed; `onCommitMessageEntered`
  and `runCommit` (commit.go) are otherwise unchanged from part 4.
- Enter inside the field types a newline, the way any multi-line text field
  should -- it does not submit the dialog the way a single-line `vtui.Edit`'s
  Enter would have. `strings.TrimSpace` on the confirmed text still only
  strips a blank line the user left at the very start or end of the field;
  it does not touch a blank line between the subject and the body, which is
  exactly the convention a multi-line message needs to keep. A message that
  is blank throughout is rejected with the same toast an empty single-line
  one already was.
- `--amend` is part 17; a commit signature/author override remains out of
  scope -- see the ticket for the remaining list.

## Part 10: creating and deleting branches (`branchview.go`)

- **Insert** on the branch list opens a single-line
  `internal/dialog.FileInputBox` prompt for a new branch's name -- the same
  dialog part 4 first built its own one-line commit prompt from, before
  part 9 replaced that one with a multi-line editor; a branch name is
  always one line, so there is no reason to reach for the heavier widget
  here. Confirming it runs plain `git branch <name>`, not `git switch -c
  <name>`: creating a branch and switching to it are two separate gestures
  this panel already keeps apart (switching is Enter, part 7), and leaving
  the checked-out branch untouched is the safer default -- nothing stops a
  user who does want to switch from pressing Enter on the freshly created
  entry right afterward. A blank name (confirming the dialog without typing
  anything) is rejected with a toast before any git command runs, the same
  way part 4's blank commit message was.
- **Delete/F8** on the branch list deletes the branch under the cursor,
  after a Yes/No-style confirmation (`vtui.ShowMessageOn`) -- the same
  "confirm, then act on button 0" shape
  `internal/plughost/permissions_ui.go`'s own Revoke button already uses
  for its own irreversible action. It runs `git branch -d <name>` (the safe
  delete), never `-D`: git itself refuses `-d` when the branch has commits
  not reachable from any other ref ("not fully merged"), and that refusal
  surfaces here as git's own message, the same way every other failed
  command in this plugin already reports its own. The branch checked out
  right now is refused outright, with its own toast, before a confirmation
  dialog even opens -- git would refuse it anyway, and there is nothing
  useful about asking the user to confirm an operation that cannot succeed.
- Both reload the branch list afterward (so a newly created or deleted
  branch shows up or disappears immediately), the same point-in-time
  refresh F5 already gives this list.

## Part 11: picking one file out of a multi-file commit's diff (`logdifffiles.go`)

- **Enter** on a log-view commit that touches more than one path (part 5's
  own `showDiff`, `logdiff.go`) no longer shows a toast: it opens
  `LogDiffFilesView`, a small read-only list of just that commit's changed
  paths (status letter + path, the same two columns and "old -> new" rename
  rendering the status panel's own table already has). **Enter** on an
  entry there opens the same side-by-side `internal/diffview` diff a
  single-file commit's own Enter already gave, against that one path's
  parent and commit revisions.
- This did not need f4#613 itself for anything new -- `diffview.DiffView`
  already compares exactly two whole files, which is exactly what one path
  out of a multi-file commit still is. The only missing piece was *which*
  path to hand it, not a different or heavier widget.
- A merge commit (or any other commit `git show` reports zero changed paths
  for -- `commitChangedFiles`'s own doc comment) still shows a toast instead
  of an empty list: there is nothing to pick from either way.

## Part 12: staging part of a file, hunk by hunk (`hunk.go`, `hunkview.go`)

- **F4** on a status-panel entry (keybar: **Hunks**) opens `HunkView`, the
  file's *unstaged* changes (`git diff`, index vs. worktree -- what
  `git add -p` offers) as a list of hunks: each hunk's `@@` line followed
  by its `-`/`+`/context lines.
- **Insert** or **Space** picks the hunk under the cursor (or drops it
  again) and moves the cursor to the next hunk's `@@` line, so repeated
  Insert walks the file hunk by hunk; picked hunks are painted in the
  "selected" color a marked file has in a file panel, and the title counts
  them. **Enter** or **F2** stages the picked hunks and returns to the
  status panel, reloaded, with the cursor on the same file (now `MM` if
  some changes remain unstaged). **Esc**/**F10** return without staging.
- Staging rebuilds a patch from the file header and the picked hunks only
  and hands it to `git apply --cached`. The `+` start of each kept hunk is
  shifted back by the line-count change of every hunk left out before it,
  the same adjustment `git add -p` makes; a mode change is left out of the
  patch (Insert still stages the whole file, mode included). `git apply`
  runs at the repository root, because from a subdirectory it silently
  skips root-relative patch paths outside that subdirectory.
- The diff is taken with color, external diff drivers and textconv turned
  off and with the standard `a/`/`b/` prefixes forced, so user settings
  (`diff.noprefix`, `diff.mnemonicPrefix`, `diff.relative`, ...) cannot
  produce text `git apply` would not read back.
- An untracked file, a binary file or a mode-only change has no hunks to
  offer: F4 says so in a toast, and Insert remains the way to stage it.

## Part 13: unstaging part of a file, hunk by hunk (Shift+F4)

- **Shift+F4** on a status-panel entry (Shift keybar row: **Unstage**)
  opens the same `HunkView` over the file's *staged* changes
  (`git diff --cached`, HEAD vs. index -- what `git reset -p` offers). The
  keys are the same as for F4; **Enter**/**F2** takes the picked hunks out
  of the index (the worktree is not touched) and returns to the reloaded
  status panel.
- A separate key rather than F4 guessing the direction from the status: an
  `MM` file has hunks on both sides, and either may be the one wanted.
  Shift+F4 is "edit a new file" in a file panel, which means nothing here.
- Unstaging applies the patch of the picked hunks in reverse
  (`git apply --cached -R`, at the repository root, same diff flags). Here
  the `+` side describes the index as it is, and the `-` start of each kept
  hunk is shifted forward by the line-count change of every hunk left out
  (and so left in the index) before it.
- A staged new file is a single hunk; unstaging it leaves the file
  untracked. A staged rename is not offered in parts (the diff of the new
  path alone reads as an added file): Insert unstages it whole.

## Part 14: picking single lines inside a hunk (F4 and Shift+F4)

- In `HunkView` (both directions) **Insert**/**Space** on a `+` or `-`
  line picks or drops that line alone and moves the cursor one line down;
  on the `@@` line or a context line it still picks or drops the whole
  hunk (a half-picked hunk is picked whole first) and jumps to the next
  hunk. A picked line is painted in the "selected" color; the `@@` and
  context lines are painted only while the whole hunk is picked, and a
  half-picked hunk's `@@` line ends in `[picked/changed]`.
- This covers splitting a hunk as well (`git add -p`'s `s`): picking the
  lines of one run of changes between context lines stages just that run.
- `buildPatch` rebuilds each hunk with picked lines the way `git add -p`'s
  `e` asks the user to edit it by hand. The side that is the index now
  (`-` when staging, `+` when unstaging, since that patch is applied with
  `-R`) keeps every line: its unpicked changed lines become context. The
  other side's unpicked lines are dropped, together with a
  `\ No newline at end of file` marker after them. Both counts of the `@@`
  line are recounted, and the start of the side that is not the index is
  moved by the line-count change of the rebuilt hunks before it.
- Two picks cannot become a patch and are refused with a toast, nothing
  applied: part of a hunk of a new file (that would turn the creation into
  a modification -- pick it whole; a deleted file can be picked in part, see
  Part 25), and a pick
  that would keep a last line without a trailing newline in the middle of
  the file.
- Tests on real repositories (`hunk_test.go`) stage and unstage single
  lines of two hunks where the second one has to move by the rebuilt first
  one, check the index with `git show :f.txt`, and stage around a missing
  final newline.

## Part 15: discarding part of the working file (F8)

- **F8** on a status-panel entry (keybar: **Discard**) opens the same
  `HunkView` over the file's unstaged changes (`git diff`, index vs.
  worktree -- what `git checkout -p` offers). Picking hunks and single
  lines works as with F4. **Enter**/**F2** first asks: "Discard N changed
  lines (k of n hunks) from the working file ...? This cannot be undone."
  Only **Discard** goes on; Cancel or Esc leaves the file, the index and
  the picks as they were.
- F8 because it is the destructive key of a Far-style keybar -- Delete in
  a file panel and in the branch list (Part 7); the caption says
  "Discard", since the file itself stays.
- The picked lines are taken out of the working file with `git apply -R`
  (no `--cached`: the index is not touched), which puts back what the
  index has there -- not HEAD, so staged changes stay in the file too.
  The patch is rebuilt the way it is for unstaging: the `+` side is the
  working file as it is and keeps every line (unpicked `+` lines become
  context), the `-` side takes the picked lines only, and the `-` start of
  each kept hunk moves by the line-count change of the rebuilt hunks
  before it. `git apply` changes nothing if the file no longer matches.
- A file deleted from the working tree is one hunk: discarding it whole
  brings the file back; part of it is refused before the question, as are
  the other picks `buildPatch` cannot represent.
- Tests on real repositories (`hunk_discard_test.go`) check the working
  file's bytes after discarding single lines of two hunks (the second one
  found a line higher), a whole hunk from a subdirectory, a change on top
  of staged lines, a deleted file, and that cancelling changes nothing.

## Part 16: F8 on an untracked file deletes it

- `git diff` shows nothing for an untracked entry (`??`) -- there is no
  committed or indexed version to compare it against -- so before this
  part **F8** on one just toasted "nothing to discard in parts" and did
  nothing else. Now it asks to delete the path from disk outright instead:
  "Delete `<path>` from disk? Git does not track it, so there is nothing
  to discard it back to -- this removes it outright. This cannot be
  undone." Only **Delete** goes on; Cancel or Esc leaves it.
- The same confirm-first shape Part 15's discard uses, just without
  `HunkView` in between -- there are no hunks to pick from an untracked
  path, so the whole path is the only thing F8 can offer.
- An untracked directory is one first-level `?? <dir>/` status entry, not
  descended into (`parseStatus`, `status.go`): its trailing `/` is what
  tells `confirmDeleteUntracked`/`deleteUntracked` (`untracked.go`) to
  remove it recursively (`os.RemoveAll`) rather than as a single file
  (`os.Remove`).
- Tests (`untracked_test.go`) delete an untracked file and an untracked
  directory with its content, check the panel reloads without the entry
  and keeps the cursor sensible, check Cancel/Esc leaves the path alone,
  and dump the confirmation screen in English and Russian.

## Design notes

- Every git invocation goes through `execGit` (a package-level var, the
  same substitution seam `plugins/multiarc/tools.go`'s `runToolIn` is), with
  `-c core.quotepath=false` always prepended (`runGitIn`) so a non-ASCII
  path comes back as literal UTF-8 instead of git's octal-escaped quoting.
  A path containing a literal double quote or backslash is a known,
  unhandled edge case for this first version.
- `parseStatus` (`status.go`) is a pure function over `git status
  --porcelain=v2 --branch`'s stable, documented output shape, tested with
  fixed sample text rather than a real repository -- the same split
  `internal/textdiff`'s algorithm keeps from its caller (f4#613).
- Ahead/behind and upstream tracking (`# branch.ab`/`# branch.upstream`) are
  parsed by git but not surfaced by this panel yet; `parseStatus` does not
  keep them either, on the principle that an unused, untested field is the
  wrong thing to carry around. A later part that wants them only has to add
  the two lines back.
- `headFileContent` (`diff.go`) runs `git show HEAD:./<path>`, never bare
  `HEAD:<path>` -- per `gitrevisions(7)`, a path after the colon is resolved
  relative to the repository's top level unless it starts with `./` or
  `../`, in which case it is resolved relative to the current directory
  instead. Since every git invocation here runs with `dir` (a possible
  subdirectory of the repository) as its working directory, and
  `entry.Path`/`entry.OrigPath` come from `git status` reported the same way
  relative to that same `dir`, the `./` prefix is required for the two to
  agree on what the path means.

## Part 17: amending the last commit (Ctrl+K)

- The Ctrl+K commit dialog gets an **Amend the previous commit** checkbox
  whenever the repository already has a commit (`headCommitMessage`,
  `git log -1 --format=%B`). Checking it while the message field is empty
  loads the last commit's message into it, as `git commit --amend` does in
  `$EDITOR`; unchecking it takes that message out again if it was not edited.
- Ok then runs `git commit --amend -m <message>` (`runCommitAmend`): the
  staged changes are folded into the last commit and its message is replaced.
  Without the box nothing changes: `runCommit` is `git commit -m`.
- With an empty index Ctrl+K still opens the dialog when the repository has a
  commit (`statusResult.HasCommit`, from `# branch.oid`): amend then just
  rewords the last commit. A plain commit with nothing staged answers "nothing
  staged to commit" as before, and Ctrl+K in a repository with no commit and
  nothing staged shows the toast without a dialog.
- `showCommitMessageEditor` keeps its signature and shows no checkbox;
  `showCommitMessageEditorEx` is the amend-aware variant the panel uses.

## Part 20: fetch, pull and push (`remote.go`)

- **Shift+F5** runs `git fetch`, **Shift+F6** `git pull --ff-only`, **Shift+F7**
  `git push` (after a confirmation: it publishes commits). They sit on the
  Shift row of the keybar with their own captions, the same row Shift+F4
  (unstage hunks) uses.
- All three run off the UI goroutine (`vtui.RunAsync`); the panel reloads
  afterwards and a toast shows git's first output line. A failure -- no
  upstream, authentication, diverged history -- is an error dialog with git's
  first line.
- Pull is fast-forward only, so it never creates a merge commit or leaves a
  conflicted tree; diverged history is reported and merging is left to the
  user's own tools. Merge and stash are not part of this panel yet.
- `execGit` now sets `GIT_TERMINAL_PROMPT=0` for every call: a password prompt
  over https would wait on a terminal nobody sees. Use a credential helper or
  an SSH agent for remotes that need authentication.

## Part 21: stash and unstash (`remote.go`)

- **Shift+F2** runs `git stash push` (tracked changes go into the stash, the
  working tree returns to HEAD) and **Shift+F3** runs `git stash pop` (the
  newest stash comes back and is dropped). Same background run, reload and
  toast/error dialog as fetch/pull/push; a pop that conflicts keeps the stash
  and shows git's message. Untracked files are not stashed.

## Part 22: author of a commit (`commit.go`)

- The Ctrl+K commit dialog has an **Author** field. Empty keeps the configured
  identity; "Name <email>" (or a name git finds among existing commits) is
  passed as `git commit --author=...`, with or without `--amend`/`--signoff`.
  An invalid value is git's own error in the failure toast.
- The dialog's switches travel as one `commitOptions` value now.

## F1: the plugin's own help (`help.go`, f4#272)

- **F1** on the status panel opens the plugin's help -- a Markdown text in
  vtui's Markdown viewer, the same window the F3 view of `.md` files uses. The
  text is the language-file string `GitStatus.Help` (title `GitStatus.HelpTitle`),
  so it follows the interface language; it lists every key of the panel.
- No new host API: a plugin declares F1 among its `PanelKeys`
  (`vfs.PanelKeyProvider`), which the host already runs ahead of the global
  Help binding and puts on the keybar, and shows its own text this way.

## Part 23: merging a branch (`branchview.go`)

- **F6** on the branch list (Ctrl+S) merges the branch under the cursor into
  the current one after a confirmation: `git merge --no-edit`. A merge that
  stops -- conflicts, local changes in the way -- is undone with
  `git merge --abort` at once and reported in an error dialog, so the tree is
  never left half merged; resolving conflicts is left to the user's own tools.
- The current branch cannot be merged into itself. After a merge the branch
  list and the status panel reload.

## Part 24: picking lines of an untracked file (`hunk.go`, `hunkview.go`)

- **F4** on an untracked file (`??`) opens the hunk view too: `git diff` shows
  nothing for such a file, so `loadUntrackedFilePatch` takes the patch from
  `git diff --no-index /dev/null <file>` (exit status 1 is the normal outcome
  there) and makes the header paths relative to the repository root.
- Staging picked lines creates the index entry with just those lines; the
  working file is left as it is, so the file shows as added and modified
  afterwards. An untracked directory (`dir/`) is expanded into its files
  with F4 (part 27); Insert on it stages it whole.

## Part 25: single lines of a deleted file

- A file deleted from the working tree (F4, F8) or staged as deleted
  (Shift+F4) can be picked line by line; `buildPatch` no longer refuses
  part of it (`filePatch.deleted`).
- Unstaging (Shift+F4) and discarding (F8) apply the patch in reverse, and
  the deletion of just the picked lines, reversed, re-creates the file with
  exactly those lines -- in the index or in the working tree; the patch
  keeps its `deleted file mode` header.
- Staging (F4) removes the picked lines from the index copy: with a line
  left out the patch is written as a modification (`deleted file mode`
  dropped, `+++ /dev/null` replaced by the `--- a/` path as `+++ b/`, the
  unpicked lines context), so the file stays in the index with the unpicked
  lines while the working tree stays without it. Picked whole, the deletion
  stays a deletion. A new file of a real diff is still picked whole only.
- Tests (`hunk_deleted_test.go`): the patch text for each mode, a quoted
  path, and F4, Shift+F4 and F8 on real repositories, checking the index
  and the working file.

## Part 26: unstaging single lines of a staged new file

- Shift+F4 on a file staged as new (`A `) can now take back just the picked
  lines: the index keeps the file with the unpicked ones. The patch, applied
  in reverse, is written as a modification (`new file mode` dropped,
  `--- /dev/null` replaced by the `+++ b/` path as `--- a/`, the unpicked
  lines context); with every line picked it stays a creation, and the file
  becomes untracked as before.
- Part 28 extends this to a file added with `git add -N` (intent-to-add):
  F4 stages just the picked lines (the patch stays a creation, as for an
  untracked file) and F8 removes just the picked lines from the working
  file (written as a modification, like the unstaging above).
- Tests (`hunk_new_test.go`): the patch text, and Shift+F4 on a real
  repository checking the index and the working file.

## Part 27: F4 on an untracked directory lists its files

- An untracked directory is one row (`?? d/`) with no lines of its own. F4
  on it now turns the row into one `??` row per file inside (`git ls-files
  --others --exclude-standard -z`), cursor on the first; F4 on a file opens
  the line picker of part 24. The directory stays expanded across reloads
  (`statusPanel.expanded`). Insert on the directory row still stages all of
  it at once.
- Tests (`untracked_dir_test.go`): the listing, staging two lines of the
  first file on a real repository (the other file stays untracked), and a
  directory with no files staying a single row.
