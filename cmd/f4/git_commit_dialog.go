package main

import (
	"context"
	"errors"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

const (
	commitDialogWidth  = 64
	commitMessageRows  = 7
	commitDialogHeight = 17
)

func defaultCommitDialogTitle() string {
	return " " + Msg("Git.Commit.Title") + " "
}

// commitMessageDialog is the console implementation behind
// vfs.CommitDialogHost. It only collects UI data; repository and signing
// policy belong to the caller's asynchronous callback.
type commitMessageDialog struct {
	*vtui.Window

	messageLabel *vtui.Text
	message      *vtui.MultiLineEdit
	sign         *vtui.Checkbox
	commit       *vtui.Button
	cancel       *vtui.Button

	onAccept func(vfs.CommitDialogResult)
}

// newCommitMessageDialog builds the UI separately from PanelsFrame so its
// layout, focus behaviour, and runtime palette use can be render-tested
// without constructing a terminal workspace.
func newCommitMessageDialog(request vfs.CommitDialogRequest, onAccept func(vfs.CommitDialogResult)) *commitMessageDialog {
	title := request.Title
	if title == "" {
		title = defaultCommitDialogTitle()
	}

	window := vtui.NewCenteredDialog(commitDialogWidth, commitDialogHeight, title)
	window.ShowClose = true

	contentWidth := commitDialogWidth - 4
	messageLabel := vtui.NewText(0, 0, Msg("Git.Commit.Message"), 0)
	message := vtui.NewMultiLineEdit(0, 0, contentWidth, commitMessageRows, request.InitialMessage)
	sign := vtui.NewCheckbox(0, 0, Msg("Git.Commit.Sign"), false)
	sign.State = boolToCheckboxState(request.InitialSign)
	commit := vtui.NewButton(0, 0, Msg("Git.Commit.Button"))
	commit.IsDefault = true
	cancel := vtui.NewButton(0, 0, Msg("vtui.Cancel"))

	dialog := &commitMessageDialog{
		Window:       window,
		messageLabel: messageLabel,
		message:      message,
		sign:         sign,
		commit:       commit,
		cancel:       cancel,
		onAccept:     onAccept,
	}

	for _, item := range []vtui.UIElement{messageLabel, message, sign, commit, cancel} {
		window.AddItem(item)
	}

	vbox := vtui.NewVBoxLayout(window.X1+2, window.Y1+2, contentWidth, commitDialogHeight-4)
	vbox.Add(messageLabel, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(message, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Add(sign, vtui.Margins{Top: 1}, vtui.AlignLeft)

	buttons := vtui.NewHBoxLayout(0, 0, contentWidth, 1)
	buttons.HorizontalAlign = vtui.AlignCenter
	buttons.Spacing = 2
	buttons.Add(commit, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(cancel, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(buttons, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()

	cancel.OnClick = window.Close
	commit.OnClick = dialog.accept
	window.SetFocusedItem(message)
	return dialog
}

func boolToCheckboxState(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (dialog *commitMessageDialog) accept() {
	if dialog == nil || dialog.Window == nil || dialog.IsDone() {
		return
	}
	result := vfs.CommitDialogResult{
		Message: dialog.message.GetText(),
		Sign:    dialog.sign.State == 1,
	}
	dialog.Close()
	if dialog.onAccept != nil {
		dialog.onAccept(result)
	}
}

// OpenCommitDialog implements vfs.CommitDialogHost. Opening is asynchronous
// with respect to the caller, and accepting the dialog runs the supplied
// repository callback through the normal cancellable background task bridge.
func (pf *PanelsFrame) OpenCommitDialog(request vfs.CommitDialogRequest) error {
	if pf == nil {
		return errors.New("commit dialog requires a panels frame")
	}
	if request.OnCommit == nil {
		return errors.New("commit dialog requires an OnCommit callback")
	}

	vtui.FrameManager.PostTask(func() {
		dialog := newCommitMessageDialog(request, func(result vfs.CommitDialogResult) {
			title := request.Title
			if title == "" {
				title = defaultCommitDialogTitle()
			}
			progress := Msg("Git.Commit.Progress")
			pf.RunProgressTask(title, progress, false,
				func(ctx context.Context, update func(string, int)) error {
					update(progress, -1)
					return request.OnCommit(ctx, result)
				},
				func(err error) {
					if err == nil || errors.Is(err, context.Canceled) {
						return
					}
					vtui.ShowMessage(defaultCommitDialogTitle(), err.Error(), []string{Msg("vtui.Ok")})
				},
			)
		})
		vtui.FrameManager.Push(dialog)
	})
	return nil
}

var _ vfs.CommitDialogHost = (*PanelsFrame)(nil)
