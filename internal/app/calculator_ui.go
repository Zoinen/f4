package app

import (
	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/history"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/vtui"
)

// Built-in calculator (f4#384, an idea carried over from DOS Navigator): a
// small dialog, reachable from the panels without an editor open, that
// evaluates an arithmetic expression and can insert the result into the
// active panel's command line or copy it to the clipboard (Copy reuses the
// shared Copy.Btn label rather than a calculator-only string). Expressions that
// evaluated successfully are kept in a history list (f4#1600).
//
// The actual math reuses editor.EvaluateArithmetic/FormatCalculatorResult,
// the same engine f4#1463's "calculate selection" editor command already
// added -- this dialog is a second entry point onto that engine for when
// there is no file open to select an expression in, not a second
// implementation of arithmetic parsing.

const calcDialogWidth = 50 // wide enough for three buttons in longer locales (e.g. ru: Вставить, Копировать, Отмена)

// calcDialogHeight must fit all four stacked items (prompt label, expression
// edit, result label, button row) below the title and above the bottom
// border with the validator's usual 1-cell clearance on each side (see
// showMkDirDialog's identically-shaped 4-item stack in actions.go, which
// needs the same 11 rows for a 40-wide dialog): vbox starts at Y1+2, each
// item is 1 row tall and separated by a Margins{Top: 1} gap, so the last
// item lands 6 rows below the vbox top and needs 2 more rows of clearance
// before the bottom border -- 2 (top border+clearance) + 6 + 2 (bottom
// clearance+border) - 1 = 11.
const calcDialogHeight = 11

func showCalculatorDialog() {
	dlg := vtui.NewCenteredDialog(calcDialogWidth, calcDialogHeight, i18n.Msg("Calculator.Title"))
	dlg.ShowClose = true

	lblExpr := vtui.NewLabel(0, 0, i18n.Msg("Calculator.Prompt"), nil)
	editExpr := vtui.NewEdit(0, 0, calcDialogWidth-8, "")
	// Past expressions: the same history machinery as the other dialog inputs
	// (Ctrl+E / Ctrl+X walk it, Ctrl+Down opens the list, persisted across runs).
	history.AttachHistory(editExpr, history.CalculatorHistoryID)
	lblExpr.FocusLink = editExpr
	dlg.SetFocusedItem(editExpr)

	lblResult := vtui.NewLabel(0, 0, i18n.Msg("Calculator.ResultEmpty"), nil)
	lblResult.X2 = lblResult.X1 + (calcDialogWidth - 8) - 1 // fixed width, independent of result length

	var lastResult float64
	var haveResult bool

	evaluate := func() {
		v, err := editor.EvaluateArithmetic(editExpr.GetText())
		if err != nil {
			haveResult = false
			lblResult.SetText(i18n.Msg("Calculator.ResultError"))
			return
		}
		lastResult = v
		haveResult = true
		history.CommitHistory(editExpr, editExpr.GetText())
		lblResult.SetText("= " + editor.FormatCalculatorResult(v))
	}

	btnInsert := vtui.NewButton(0, 0, i18n.Msg("Calculator.BtnInsert"))
	btnInsert.IsDefault = true
	btnCopy := vtui.NewButton(0, 0, i18n.Msg("Copy.Btn"))
	btnClose := vtui.NewButton(0, 0, i18n.Msg("vtui.Cancel"))

	editExpr.OnAction = func() { evaluate() }

	dlg.AddItem(lblExpr)
	dlg.AddItem(editExpr)
	dlg.AddItem(lblResult)
	dlg.AddItem(btnInsert)
	dlg.AddItem(btnCopy)
	dlg.AddItem(btnClose)

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, calcDialogWidth-4, calcDialogHeight-4)
	vbox.Add(lblExpr, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(editExpr, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Add(lblResult, vtui.Margins{Top: 1}, vtui.AlignLeft)

	hbox := vtui.NewHBoxLayout(0, 0, calcDialogWidth-4, 1)
	hbox.HorizontalAlign = vtui.AlignCenter
	hbox.Spacing = 2
	hbox.Add(btnInsert, vtui.Margins{}, vtui.AlignTop)
	hbox.Add(btnCopy, vtui.Margins{}, vtui.AlignTop)
	hbox.Add(btnClose, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(hbox, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()

	btnInsert.OnClick = func() {
		evaluate()
		if !haveResult {
			return
		}
		text := editor.FormatCalculatorResult(lastResult)
		dlg.Close()
		insertTextIntoCommandLine(text)
	}
	btnCopy.OnClick = func() {
		evaluate()
		if !haveResult {
			return
		}
		text := editor.FormatCalculatorResult(lastResult)
		dlg.Close()
		// Async: the clipboard round trip can block on far2l IPC.
		terminal.SetClipboardAsync(text)
	}
	btnClose.OnClick = func() { dlg.Close() }

	vtui.FrameManager.Push(dlg)
}

// insertTextIntoCommandLine puts text at the cursor of the active panel's
// command line, the same destination the "insert current file name"-style
// commands already use. Nothing happens if there is no command line to
// insert into (e.g. a terminal or plugin panel is active) -- the calculator
// still showed the result, so the round trip was not wasted.
func insertTextIntoCommandLine(text string) {
	pf := panel.FindPanelsFrameAnyScreen()
	if pf == nil || pf.CmdLine == nil || pf.CmdLine.Edit == nil {
		return
	}
	pf.CmdLine.Edit.InsertString(text)
}
