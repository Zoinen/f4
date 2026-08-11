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

	// btnBackground exists only for an operation that can genuinely be left
	// running when the window goes away. Copying through this client cannot:
	// closing its dialog has to stop it. Work that happens on a remote host
	// can, and says so by calling EnableBackground.
	btnBackground *vtui.Button
	vbox          *vtui.VBoxLayout
	hbox          *vtui.HBoxLayout
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

	dlg.btnCancel = vtui.NewButton(0, 0, Msg("FileOp.BtnCancel"))

	dlg.AddItem(dlg.lblCurrent)
	dlg.AddItem(dlg.pbCurrent)
	dlg.AddItem(dlg.lblTotal)
	dlg.AddItem(dlg.pbTotal)
	dlg.AddItem(dlg.lblSpeed)
	dlg.AddItem(dlg.lblHint)
	dlg.AddItem(dlg.btnCancel)

	vbox := vtui.NewVBoxLayout(dlg.X1+3, dlg.Y1+2, width-6, height-4)
	dlg.vbox = vbox
	vbox.Add(dlg.lblCurrent, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(dlg.pbCurrent, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Add(dlg.lblTotal, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Add(dlg.pbTotal, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Add(dlg.lblSpeed, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Add(dlg.lblHint, vtui.Margins{Top: 1}, vtui.AlignCenter)

	hbox := vtui.NewHBoxLayout(0, 0, width-6, 1)
	hbox.HorizontalAlign = vtui.AlignCenter
	hbox.Add(dlg.btnCancel, vtui.Margins{}, vtui.AlignTop)
	dlg.hbox = hbox

	vbox.Add(hbox, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()

	return dlg
}

// EnableBackground adds a second button that closes the dialog and leaves
// the work running. Only an operation that survives its window should offer
// it: the caller says what "leave it running" means by what onBackground
// does, and the job registry is what the user gets back to it through.
//
// Calling it twice is harmless, which matters because a dialog may be
// reconfigured while an operation changes phase.
func (d *FileOpProgressDialog) EnableBackground(onBackground func()) {
	if d.btnBackground != nil {
		d.btnBackground.OnClick = onBackground
		return
	}
	d.btnBackground = vtui.NewButton(0, 0, Msg("FileOp.BtnBackground"))
	d.btnBackground.OnClick = onBackground
	d.AddItem(d.btnBackground)
	d.hbox.Add(d.btnBackground, vtui.Margins{Left: 2}, vtui.AlignTop)
	d.vbox.Apply()
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

// UpdateCounting sets the dialog to counting mode: one thing at a time out
// of a known total. Scanning cannot use it because a walk does not know how
// much is left; hashing does, because the tree was already walked to decide
// which files are worth reading.
func (d *FileOpProgressDialog) UpdateCounting(action, currentPath string, done, total int64) {
	d.lblCurrent.SetText(runewidth.Truncate(action+": "+currentPath, 54, "..."))
	d.lblTotal.SetText(fmt.Sprintf("%d of %d files", done, total))

	d.pbCurrent.SetVisible(false)
	if total > 0 {
		d.pbTotal.SetVisible(true)
		d.pbTotal.SetPercent(int(done * 100 / total))
	} else {
		d.pbTotal.SetVisible(false)
	}
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
