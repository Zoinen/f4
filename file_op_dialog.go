package main

import (
	"fmt"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/vtui"
)

// FileOpProgressDialog is a specialized dialog for file operations (Copy/Move/Delete).
// It supports switching between a scanning mode (text only) and a transfer mode (dual progress bars).
type FileOpProgressDialog struct {
	*vtui.Window
	lblCurrent *vtui.Text
	pbCurrent  *vtui.ProgressBar
	lblTotal   *vtui.Text
	pbTotal    *vtui.ProgressBar
	lblSpeed   *vtui.Text
	lblHint    *vtui.Text
	btnCancel  *vtui.Button
}

// NewFileOpProgressDialog creates a new initialized dialog.
func NewFileOpProgressDialog(title string) *FileOpProgressDialog {
	width := 60
	height := 17
	dlg := &FileOpProgressDialog{
		Window: vtui.NewCenteredDialog(width, height, title),
	}
	dlg.AttentionSuppressed = true

	textColor := vtui.Palette[vtui.ColDialogText]

	dlg.lblCurrent = vtui.NewText(0, 0, strings.Repeat(" ", 54), textColor)
	dlg.pbCurrent = vtui.NewProgressBar(0, 0, width-6)
	dlg.lblTotal = vtui.NewText(0, 0, strings.Repeat(" ", 54), textColor)
	dlg.pbTotal = vtui.NewProgressBar(0, 0, width-6)
	dlg.lblSpeed = vtui.NewText(0, 0, strings.Repeat(" ", 54), textColor)
	dlg.lblHint = vtui.NewText(0, 0, Msg("Op.SwitchHint"), vtui.Palette[vtui.ColDialogText])

	dlg.btnCancel = vtui.NewButton(0, 0, "&Cancel")

	dlg.AddItem(dlg.lblCurrent)
	dlg.AddItem(dlg.pbCurrent)
	dlg.AddItem(dlg.lblTotal)
	dlg.AddItem(dlg.pbTotal)
	dlg.AddItem(dlg.lblSpeed)
	dlg.AddItem(dlg.lblHint)
	dlg.AddItem(dlg.btnCancel)

	vbox := vtui.NewVBoxLayout(dlg.X1+3, dlg.Y1+2, width-6, height-4)
	vbox.Add(dlg.lblCurrent, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(dlg.pbCurrent, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Add(dlg.lblTotal, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Add(dlg.pbTotal, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Add(dlg.lblSpeed, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Add(dlg.lblHint, vtui.Margins{Top: 1}, vtui.AlignCenter)

	hbox := vtui.NewHBoxLayout(0, 0, width-6, 1)
	hbox.HorizontalAlign = vtui.AlignCenter
	hbox.Add(dlg.btnCancel, vtui.Margins{}, vtui.AlignTop)

	vbox.Add(hbox, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()

	return dlg
}

// UpdateScan sets the dialog to Scanning mode (hides progress bars).
func (d *FileOpProgressDialog) UpdateScan(currentPath string, files, dirs int64) {
	safePath := runewidth.Truncate("Scanning: "+currentPath, 54, "...")
	d.lblCurrent.SetText(safePath)
	d.lblTotal.SetText(fmt.Sprintf("Found: %d files, %d folders", files, dirs))

	d.pbCurrent.SetVisible(false)
	d.pbTotal.SetVisible(false)
	d.lblSpeed.SetVisible(false)
}

// UpdateTransfer sets the dialog to Transfer mode (shows progress bars and speed).
func (d *FileOpProgressDialog) UpdateTransfer(action string, filename string, currentPct int, totalText string, totalPct int, speedText string) {
	safeName := runewidth.Truncate(action+": "+filename, 54, "...")
	d.lblCurrent.SetText(safeName)

	if currentPct >= 0 {
		d.pbCurrent.SetVisible(true)
		d.pbCurrent.SetPercent(currentPct)
	} else {
		d.pbCurrent.SetVisible(false)
	}

	d.lblTotal.SetText(runewidth.Truncate(totalText, 54, "..."))

	if totalPct >= 0 {
		d.pbTotal.SetVisible(true)
		d.pbTotal.SetPercent(totalPct)
	} else {
		d.pbTotal.SetVisible(false)
	}

	d.lblSpeed.SetVisible(true)
	d.lblSpeed.SetText(speedText)
}
