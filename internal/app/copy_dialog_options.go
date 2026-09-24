package app

import (
	"strconv"
	"strings"

	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/vtui"
)

// copyDialogSession keeps choices of the F5/F6 dialog (#722) for the rest of
// the session without writing them to the configuration.
//
// Symlink contents starts on, which is what f4 did before it asked. The
// advanced options stay out of the configuration on purpose: a copy started
// without the dialog must never ignore errors because of a choice made for a
// different copy in an earlier session.
var copyDialogSession = struct {
	symlinkContents bool
	advanced        fileops.FileOpOptions
}{
	symlinkContents: true,
	advanced:        fileops.FileOpOptions{ReadAttempts: 1},
}

func newChoiceCombo(choices []string, selected int) *vtui.ComboBox {
	combo := vtui.NewComboBox(0, 0, rightsComboWidth, choices)
	combo.DropdownOnly = true
	if selected < 0 || selected >= len(choices) {
		selected = 0
	}
	combo.Menu.SetSelectPos(selected)
	combo.Edit.SetText(choiceText(choices, selected))
	return combo
}

func captionCells(caption string) int {
	return vtui.StringWidth(strings.ReplaceAll(caption, "&", ""))
}

// optionRow lays out a caption and its field, padding the caption to
// captionWidth so that the fields of consecutive rows start in one column.
func optionRow(width int, label *vtui.Text, field *vtui.ComboBox, caption string, captionWidth int) *vtui.HBoxLayout {
	row := vtui.NewHBoxLayout(0, 0, width, 1)
	row.Add(label, vtui.Margins{Right: 1 + captionWidth - captionCells(caption)}, vtui.AlignLeft)
	row.Add(field, vtui.Margins{}, vtui.AlignFill)
	return row
}

const (
	advancedOptionsWidth  = 50
	advancedOptionsHeight = 9
)

// showCopyAdvancedOptions is the "Advanced options" dialog of F5/F6 (#722). It
// edits opts in place when confirmed; the copy dialog reads it when its own
// button is pressed.
func showCopyAdvancedOptions(opts *fileops.FileOpOptions) {
	w, h := advancedOptionsWidth, advancedOptionsHeight
	dlg := vtui.NewCenteredDialog(w, h, i18n.Msg("Copy.Advanced.Title"))

	chkRead := vtui.NewCheckbox(0, 0, i18n.Msg("Copy.Advanced.IgnoreRead"), false)
	attempts := opts.ReadAttempts
	if attempts < 1 {
		attempts = 1
	}
	editAttempts := vtui.NewEdit(0, 0, 6, strconv.Itoa(attempts))
	lblAttempts := vtui.NewLabel(0, 0, i18n.Msg("Copy.Advanced.ReadAttempts"), editAttempts)
	chkWrite := vtui.NewCheckbox(0, 0, i18n.Msg("Copy.Advanced.IgnoreWrite"), false)
	if opts.IgnoreReadErrors {
		chkRead.State = 1
	}
	if opts.IgnoreWriteErrors {
		chkWrite.State = 1
	}

	// The count means something only while read errors are ignored, and
	// turning that on offers the field straight away.
	enableAttempts := func(on bool) {
		lblAttempts.SetDisabled(!on)
		editAttempts.SetDisabled(!on)
	}
	enableAttempts(opts.IgnoreReadErrors)
	chkRead.OnChange = func(state int) {
		enableAttempts(state == 1)
		if state == 1 {
			dlg.SetFocusedItem(editAttempts)
		}
	}

	btnOk := vtui.NewButton(0, 0, i18n.Msg("vtui.Ok"))
	btnOk.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, i18n.Msg("vtui.Cancel"))
	btnOk.OnClick = func() {
		opts.IgnoreReadErrors = chkRead.State == 1
		opts.IgnoreWriteErrors = chkWrite.State == 1
		opts.ReadAttempts = parseReadAttempts(editAttempts.GetText())
		dlg.Close()
	}
	btnCancel.OnClick = func() { dlg.Close() }

	dlg.AddItem(chkRead)
	dlg.AddItem(lblAttempts)
	dlg.AddItem(editAttempts)
	dlg.AddItem(chkWrite)
	dlg.AddItem(btnOk)
	dlg.AddItem(btnCancel)

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, w-4, h-4)
	vbox.Add(chkRead, vtui.Margins{}, vtui.AlignLeft)
	rowAttempts := vtui.NewHBoxLayout(0, 0, w-4, 1)
	rowAttempts.Add(lblAttempts, vtui.Margins{Left: 4, Right: 1}, vtui.AlignLeft)
	rowAttempts.Add(editAttempts, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(rowAttempts, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(chkWrite, vtui.Margins{}, vtui.AlignLeft)
	buttons := vtui.NewHBoxLayout(0, 0, w-4, 1)
	buttons.HorizontalAlign = vtui.AlignCenter
	buttons.Spacing = 2
	buttons.Add(btnOk, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(buttons, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()

	dlg.SetFocusedItem(chkRead)
	vtui.FrameManager.Push(dlg)
}

// parseReadAttempts reads the field leniently: anything that is not a positive
// number is one attempt, the value the issue gives as the default.
func parseReadAttempts(text string) int {
	n, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || n < 1 {
		return 1
	}
	return min(n, fileops.MaxReadAttempts)
}
