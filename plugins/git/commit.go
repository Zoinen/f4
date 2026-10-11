package git

import (
	"context"
	"fmt"
	"strings"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/toast"
	"github.com/unxed/vtui"
)

// commitDialogWidth and commitDialogHeight size the multi-line commit
// message editor showCommitMessageEditor opens. A fixed size, clamped to
// the current terminal in showCommitMessageEditor itself, is the same
// choice plugins/envman/dialogs.go's own profile dialog makes for its
// MultiLineEdit field: this is a short paragraph of text, not a file list
// or a table that should track the terminal width the way
// internal/dialog.FileDialog's copy/move/rename dialogs do.
const (
	commitDialogWidth  = 70
	commitDialogHeight = 16
)

// hasStagedChanges reports whether any entry currently listed in the panel
// already has something staged for it -- the same "index column is not
// blank/untracked" test stageActionFor (stage.go) uses to decide whether its
// own toggle key should stage or unstage an entry, reused here to decide
// whether there is anything for a commit to record.
//
// A conflicted entry (unmergedXY, stage.go) is never counted as staged here,
// even though its index slot is not blank: stageActionFor always resolves it
// to "add" (resolve), so this panel treats an unresolved conflict the same
// way Insert already does -- something to mark resolved first, not something
// ready to commit as-is.
func (p *statusPanel) hasStagedChanges() bool {
	for _, r := range p.table.Rows {
		row, ok := r.(statusRow)
		if !ok {
			continue
		}
		if stageActionFor(row.entry) == stageActionRestore {
			return true
		}
	}
	return false
}

// showCommitDialog is Ctrl+K on the status panel (panel.go's PanelKeys): a
// commit message prompt over whatever Insert (stage.go) has already staged.
// Part 4 first built this from internal/dialog.FileInputBox, the same
// single-line input dialog internal/app/actions.go's actionRename builds its
// Rename prompt from -- deliberately minimal, a single-line message only,
// see part 4's own commit for the reasoning. That single line was never
// enough for a message with its own subject and body -- the "editor for
// commit messages" the ticket asked for from the start -- so this now opens
// showCommitMessageEditor below instead: a real multi-line field, not a
// wider one-liner.
//
// Nothing staged means nothing for `git commit` to record: opening a dialog
// only for the user to cancel it is worse than not opening one at all, so
// this shows a toast instead, the same guard actionMkDir and friends skip
// only because they have no equivalent "there is nothing to do" case.
func (p *statusPanel) showCommitDialog() {
	// With an empty index the dialog still opens when there is a commit to
	// amend: amending only rewords it, so it needs nothing staged.
	if !p.hasStagedChanges() && !p.hasCommit {
		toast.Show(i18n.Msg("GitStatus.NothingToCommit"), 3e9)
		return
	}

	showCommitMessageEditorEx("", headCommitMessage(p.dir), p.onCommitDialogOk)
}

// onCommitDialogOk is the commit dialog's OnOk: a plain commit needs
// something staged, an amend does not.
func (p *statusPanel) onCommitDialogOk(message string, opts commitOptions) {
	if !opts.amend && !p.hasStagedChanges() {
		toast.Show(i18n.Msg("GitStatus.NothingToCommit"), 3e9)
		return
	}
	p.onCommitMessageEnteredOpts(message, opts)
}

// headCommitMessage is the full message of the last commit, or "" when the
// repository has none yet (nothing to amend) or git could not say.
func headCommitMessage(dir string) string {
	output, err := runGitIn(context.Background(), dir, "log", "-1", "--format=%B")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// showCommitMessageEditor builds and opens the Ctrl+K commit message
// dialog: a vtui.MultiLineEdit field rather than internal/dialog.FileInputBox's
// single-line vtui.Edit, so a message can have a subject line, a blank line,
// and a longer body -- exactly the shape git itself expects when it opens
// $EDITOR on COMMIT_EDITMSG, and exactly what part 4's one-line prompt could
// not hold. Composed by hand from vtui.NewCenteredDialog, vtui.NewButton and
// vtui.NewHBoxLayout, the same way plugins/envman/dialogs.go's own profile
// dialog builds its MultiLineEdit field, rather than through
// internal/dialog.FileDialog: that helper's fixed heights and
// half-the-terminal width are sized for its own family of file dialogs
// (copy/move/rename), not a resizable paragraph of text.
//
// onOk is called with the buffer's full text (its lines joined by "\n",
// vtui.MultiLineEdit.GetText's own shape) exactly as typed -- including any
// leading or trailing blank line the user left in the field --
// onCommitMessageEntered below is the one place that trims and validates it,
// the same split showCommitDialog already kept between itself (the trigger)
// and onCommitMessageEntered (the pure decision) before this part.
func showCommitMessageEditor(initial string, onOk func(string)) {
	showCommitMessageEditorEx(initial, "", func(message string, _ commitOptions) {
		if onOk != nil {
			onOk(message)
		}
	})
}

// showCommitMessageEditorEx is showCommitMessageEditor plus git's --amend
// (f4#659 part 17): when headMessage is not empty the dialog also offers an
// "Amend the previous commit" checkbox. Checking it while the field is
// empty (or still holds the message it filled in itself) loads the last
// commit's message into the field, the way `git commit --amend` opens it in
// $EDITOR; unchecking takes that message out again. The dialog also has an
// "Add Signed-off-by" checkbox (f4#659 part 19: `git commit --signoff`).
// onOk gets the field's text and whether each box was checked.
func showCommitMessageEditorEx(initial, headMessage string, onOk func(message string, opts commitOptions)) {
	if vtui.FrameManager == nil {
		return
	}

	width, height := commitDialogWidth, commitDialogHeight
	if maxW := vtui.FrameManager.GetScreenSize() - 4; maxW > 20 && width > maxW {
		width = maxW
	}
	if maxH := vtui.FrameManager.GetScreenHeight() - 4; maxH > 8 && height > maxH {
		height = maxH
	}

	dlg := vtui.NewCenteredDialog(width, height, i18n.Msg("GitStatus.CommitTitle"))
	dlg.ShowClose = true

	x, y := dlg.X1+2, dlg.Y1+2
	dlg.AddItem(vtui.NewText(x, y, i18n.Msg("GitStatus.CommitPrompt"), 0))
	y++
	dlg.AddItem(vtui.NewText(x, y, i18n.Msg("GitStatus.CommitEditorHint"), 0))
	y += 2

	// -4 leaves room for the button row two lines below the field plus the
	// dialog's own bottom border, the same margin
	// plugins/envman/dialogs.go's own MultiLineEdit field leaves below
	// itself for the Save/Cancel row.
	editHeight := dlg.Y2 - y - 4
	editHeight -= 2 // rows for the author field and the amend and sign-off checkboxes above the buttons
	if editHeight < 3 {
		editHeight = 3
	}
	edit := vtui.NewMultiLineEdit(x, y, width-4, editHeight, initial)
	edit.SetGrowMode(vtui.GrowAll)
	dlg.AddItem(edit)

	var amend *vtui.Checkbox
	if headMessage != "" {
		amend = vtui.NewCheckbox(x, dlg.Y2-4, i18n.Msg("GitStatus.CommitAmend"), false)
		amend.OnChange = func(state int) {
			text := strings.TrimSpace(edit.GetText())
			switch {
			case state != 0 && text == "":
				edit.SetText(headMessage)
			case state == 0 && text == headMessage:
				edit.SetText("")
			}
		}
		dlg.AddItem(amend)
	}

	// The author of the commit (`git commit --author`, f4#659 part 22): empty
	// keeps the configured identity, otherwise "Name <email>" (or a name git
	// can look up among the existing commits).
	dlg.AddItem(vtui.NewText(x, dlg.Y2-5, i18n.Msg("GitStatus.CommitAuthor"), 0))
	authorEdit := vtui.NewEdit(x+12, dlg.Y2-5, width-4-12, "")
	dlg.AddItem(authorEdit)

	// Sign-off shares the row of the amend box, to its right.
	signoff := vtui.NewCheckbox(x+34, dlg.Y2-4, i18n.Msg("GitStatus.CommitSignoff"), false)
	dlg.AddItem(signoff)

	okButton := vtui.NewButton(0, 0, i18n.Msg("vtui.Ok"))
	cancelButton := vtui.NewButton(0, 0, i18n.Msg("vtui.Cancel"))
	okButton.IsDefault = true
	dlg.AddItem(okButton)
	dlg.AddItem(cancelButton)

	buttons := vtui.NewHBoxLayout(dlg.X1+2, dlg.Y2-2, width-4, 1)
	buttons.HorizontalAlign = vtui.AlignCenter
	buttons.Spacing = 2
	buttons.Add(okButton, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(cancelButton, vtui.Margins{}, vtui.AlignTop)
	buttons.Apply()
	okButton.SetGrowMode(vtui.GrowAll)
	cancelButton.SetGrowMode(vtui.GrowAll)

	// Enter inside the field itself types a newline (vtui.MultiLineEdit's
	// own key handling), not "submit" the way a single-line vtui.Edit's
	// Enter would -- committing is deliberately a button (or its own
	// mnemonic) here, never a key the text field would otherwise need for
	// the message's own line breaks.
	okButton.OnClick = func() {
		if onOk != nil {
			onOk(edit.GetText(), commitOptions{
				amend:   amend != nil && amend.State != 0,
				signoff: signoff.State != 0,
				author:  strings.TrimSpace(authorEdit.GetText()),
			})
		}
		dlg.SetExitCode(1)
	}
	cancelButton.OnClick = func() { dlg.SetExitCode(-1) }

	vtui.FrameManager.Push(dlg)
}

// onCommitMessageEntered is the commit dialog's OnOk callback
// (showCommitMessageEditor above), split out on its own so a test can drive
// the "empty/whitespace-only message" and "run git commit" paths directly,
// without going through the editor's own UI plumbing to reach them -- the
// same split diff.go keeps between showDiff (the trigger) and presentDiff
// (the pure decision it makes once the data is in hand).
//
// strings.TrimSpace only strips leading/trailing blank lines the user left
// in the field (a MultiLineEdit starting or ending on an empty row) -- it
// does not touch blank lines *between* a subject and a body, which is
// exactly the git convention a multi-line message needs to keep. A message
// that is blank throughout (including one that is only whitespace/newlines)
// is rejected the same way a blank one-line message already was.
func (p *statusPanel) onCommitMessageEntered(message string) {
	p.onCommitMessageEnteredAmend(message, false)
}

// onCommitMessageEnteredAmend is onCommitMessageEntered for the dialog that
// can amend: amend runs `git commit --amend` (the staged changes are folded
// into the last commit and its message is replaced by the one entered).
func (p *statusPanel) onCommitMessageEnteredAmend(message string, amend bool) {
	p.onCommitMessageEnteredOpts(message, commitOptions{amend: amend})
}

// onCommitMessageEnteredOpts adds `--signoff` (a Signed-off-by trailer with
// the committer's identity) to onCommitMessageEnteredAmend.
func (p *statusPanel) onCommitMessageEnteredOpts(message string, opts commitOptions) {
	message = strings.TrimSpace(message)
	if message == "" {
		toast.Show(i18n.Msg("GitStatus.CommitMessageEmpty"), 3e9)
		return
	}
	p.runCommitOpts(message, opts)
}

// runCommit runs `git commit -m message` over the currently staged changes
// and reloads the panel, the same "mutate, then reload, toast on failure"
// shape toggleStage (stage.go) already uses for git add/restore. message can
// now span several lines (showCommitMessageEditor): -m's argument is passed
// straight to the git process by exec.Cmd's own argv, never through a shell,
// so an embedded "\n" needs no escaping and git records it verbatim (minus
// its own commit.cleanup=strip trimming, the same cleanup a message typed
// into $EDITOR would get). --amend is runCommitAmend below; a commit
// signature/author override is still out of scope -- see the ticket.
func (p *statusPanel) runCommit(message string) {
	p.runCommitAmend(message, false)
}

// runCommitAmend is runCommit with git's --amend switch (f4#659 part 17).
func (p *statusPanel) runCommitAmend(message string, amend bool) {
	p.runCommitOpts(message, commitOptions{amend: amend})
}

// commitOptions are the switches of the commit dialog beyond the message:
// --amend, --signoff and --author (f4#659 parts 17, 19, 22).
type commitOptions struct {
	amend, signoff bool
	author         string
}

// runCommitOpts is runCommitAmend with git's --signoff switch (f4#659 part 19).
func (p *statusPanel) runCommitOpts(message string, opts commitOptions) {
	args := []string{"commit"}
	if opts.amend {
		args = append(args, "--amend")
	}
	if opts.signoff {
		args = append(args, "--signoff")
	}
	if opts.author != "" {
		args = append(args, "--author="+opts.author)
	}
	args = append(args, "-m", message)
	output, err := runGitIn(context.Background(), p.dir, args...)
	if err != nil {
		toast.Show(fmt.Sprintf(i18n.Msg("GitStatus.CommitFailed"), firstLine(string(output), err)), 3e9)
		return
	}

	if err := p.reload(); err != nil {
		toast.Show(fmt.Sprintf(i18n.Msg("GitStatus.RefreshFailed"), err), 3e9)
	} else {
		toast.Show(fmt.Sprintf(i18n.Msg("GitStatus.CommitDone"), firstOutputLine(string(output))), 3e9)
	}
	if vtui.FrameManager != nil {
		vtui.FrameManager.Redraw()
	}
}

// firstOutputLine returns output's first line, or output itself if it has
// none. Unlike firstLine (panel.go), which falls back to a non-nil err's own
// message for a failed command, this has no error to fall back to: it is
// only ever used on `git commit`'s own success output, which is never empty
// (git always prints at least the "[branch sha] message" summary line).
func firstOutputLine(output string) string {
	if idx := strings.IndexByte(output, '\n'); idx >= 0 {
		return output[:idx]
	}
	return output
}
