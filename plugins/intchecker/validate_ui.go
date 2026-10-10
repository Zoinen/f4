package intchecker

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// validateOptions are the "Validate files" run's user-facing choices: the
// checksum file encoding and the two checkboxes f4#1623 point 2 added --
// "Ignore missing files" and "Stop on first mismatch".
type validateOptions struct {
	encoding       fileEncoding
	ignoreMissing  bool
	stopOnMismatch bool
}

// validateReopener is what finishValidate calls to bring the dialog back
// after a run found none of the listed files, preloaded with what was just
// tried, so the user can pick another directory or encoding without
// retyping the checksum file path. p.openValidateDialog satisfies it.
type validateReopener func(app vfs.App, fs vfs.VFS, note, hashText, dirText string, opts validateOptions)

// showValidate is the "Validate files" menu command. With the cursor on a
// checksum file it checks the files next to it right away; otherwise it asks
// for the checksum file and, optionally, the directory with the files. The
// quick, cursor-on-checksum-file path keeps auto-detecting the file's
// encoding, since that already reads the actual file rather than guessing;
// only the dialog's preselected default changes with f4#1623 point 2, to
// whatever "Generate hashes" last used (p.store's Encoding). Both paths
// apply the persisted "Ignore missing files"/"Stop on first mismatch"
// checkboxes -- the dialog is not the only way to reach this command, and a
// user who always validates from the cursor should not lose the settings
// they picked the one time they did see the dialog.
func (p *Plugin) showValidate(app vfs.App) {
	fs := app.GetActivePanelVFS()
	if fs == nil {
		return
	}
	dir := fs.GetPath()
	if name := app.GetSelectedName(); name != "" && name != ".." && isChecksumFileName(name) {
		// Reading and stat'ing may be slow on a remote panel, so they run
		// off the UI goroutine; app.Message waits there, too.
		go startValidate(app, fs, fs.Join(dir, name), dir, p.quickValidateOptions(), p.openValidateDialog)
		return
	}
	p.openValidateDialog(app, fs, "", "", "", p.dialogValidateOptions())
}

// quickValidateOptions is what the cursor-on-checksum-file path of
// showValidate runs with: the persisted "Ignore missing files"/"Stop on
// first mismatch" checkboxes, but auto-detection for the encoding, since
// that path never shows a dialog to ask and auto-detection already reads
// the actual file rather than guessing.
func (p *Plugin) quickValidateOptions() validateOptions {
	settings := p.store.snapshot()
	return validateOptions{
		encoding:       autoDetectEncoding,
		ignoreMissing:  settings.ValidateIgnoreMissing,
		stopOnMismatch: settings.ValidateStopOnMismatch,
	}
}

// dialogValidateOptions is what a freshly opened "Validate files" dialog
// presets: the persisted checkboxes, same as quickValidateOptions, and, for
// the encoding, f4#1623 point 2's literal ask -- the same encoding
// "Generate hashes" last used (settings.Encoding), not auto-detection.
func (p *Plugin) dialogValidateOptions() validateOptions {
	settings := p.store.snapshot()
	return validateOptions{
		encoding:       settings.Encoding,
		ignoreMissing:  settings.ValidateIgnoreMissing,
		stopOnMismatch: settings.ValidateStopOnMismatch,
	}
}

// validateDialog asks for the checksum file, the directory to check, the
// checksum file encoding, and the "Ignore missing files"/"Stop on first
// mismatch" checkboxes (f4#1623 point 2).
type validateDialog struct {
	win            *vtui.Window
	editFile       *vtui.Edit
	editDir        *vtui.Edit
	encoding       *encodingCombo
	ignoreMissing  *vtui.Checkbox
	stopOnMismatch *vtui.Checkbox
	btnOK          *vtui.Button
}

// newValidateDialog builds the dialog. note, when not empty, is shown on top
// and moves the focus to the directory: it explains why the dialog came back.
// opts presets the encoding combo and the two checkboxes.
func newValidateDialog(note, hashText, dirText string, opts validateOptions) *validateDialog {
	width, height := 70, 16
	if note != "" {
		height += 2
	}
	d := &validateDialog{win: vtui.NewCenteredDialog(width, height, vtui.Msg("IntChecker.ValidateTitle"))}
	d.win.ShowClose = true

	d.editFile = vtui.NewEdit(0, 0, width-6, hashText)
	lblFile := vtui.NewLabel(0, 0, vtui.Msg("IntChecker.ChecksumFile"), d.editFile)
	d.editDir = vtui.NewEdit(0, 0, width-6, dirText)
	lblDir := vtui.NewLabel(0, 0, vtui.Msg("IntChecker.FilesDir"), d.editDir)
	d.encoding = newEncodingCombo(24, readEncodingChoices(), opts.encoding)
	lblEncoding := vtui.NewLabel(0, 0, vtui.Msg("IntChecker.FileEncoding"), d.encoding.box)
	d.ignoreMissing = vtui.NewCheckbox(0, 0, vtui.Msg("IntChecker.IgnoreMissing"), false)
	d.ignoreMissing.State = checkboxState(opts.ignoreMissing)
	d.stopOnMismatch = vtui.NewCheckbox(0, 0, vtui.Msg("IntChecker.StopOnMismatch"), false)
	d.stopOnMismatch.State = checkboxState(opts.stopOnMismatch)
	d.btnOK = vtui.NewButton(0, 0, vtui.Msg("vtui.Ok"))
	d.btnOK.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, vtui.Msg("vtui.Cancel"))
	btnCancel.OnClick = func() { d.win.Close() }

	vbox := vtui.NewVBoxLayout(d.win.X1+2, d.win.Y1+2, width-4, height-4)
	if note != "" {
		txtNote := vtui.NewText(0, 0, note, vtui.Palette[vtui.ColDialogText])
		d.win.AddItem(txtNote)
		vbox.Add(txtNote, vtui.Margins{}, vtui.AlignLeft)
	}
	for _, item := range []vtui.UIElement{lblFile, d.editFile, lblDir, d.editDir, lblEncoding, d.encoding.box, d.ignoreMissing, d.stopOnMismatch, d.btnOK, btnCancel} {
		d.win.AddItem(item)
	}
	top := 0
	if note != "" {
		top = 1
	}
	vbox.Add(lblFile, vtui.Margins{Top: top}, vtui.AlignLeft)
	vbox.Add(d.editFile, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(lblDir, vtui.Margins{Top: 1}, vtui.AlignLeft)
	vbox.Add(d.editDir, vtui.Margins{}, vtui.AlignFill)
	encodingRow := vtui.NewHBoxLayout(0, 0, width-4, 1)
	encodingRow.Spacing = 1
	encodingRow.Add(lblEncoding, vtui.Margins{}, vtui.AlignLeft)
	encodingRow.Add(d.encoding.box, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(encodingRow, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Add(d.ignoreMissing, vtui.Margins{Top: 1}, vtui.AlignLeft)
	vbox.Add(d.stopOnMismatch, vtui.Margins{}, vtui.AlignLeft)
	buttons := vtui.NewHBoxLayout(0, 0, width-4, 1)
	buttons.HorizontalAlign = vtui.AlignCenter
	buttons.Spacing = 2
	buttons.Add(d.btnOK, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(buttons, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()

	if note != "" {
		d.win.SetFocusedItem(d.editDir)
	}
	return d
}

// openValidateDialog shows the checksum file/directory/encoding/options
// dialog, preset to opts. On Ok it persists the "Ignore missing
// files"/"Stop on first mismatch" checkboxes through p.store, the same way
// submitGenerate persists the generate dialog's choices (f4#1623 point 2);
// the encoding choice is not persisted here on purpose -- the dialog's
// default always tracks whatever "Generate hashes" last used instead (see
// showValidate), which is the literal ask ("by default, the same encoding
// as for generation").
func (p *Plugin) openValidateDialog(app vfs.App, fs vfs.VFS, note, hashText, dirText string, opts validateOptions) {
	panelDir := fs.GetPath()
	d := newValidateDialog(note, hashText, dirText, opts)
	d.btnOK.OnClick = func() { p.submitValidate(app, fs, panelDir, d) }
	vtui.FrameManager.Push(d.win)
}

// submitValidate is openValidateDialog's Ok handler, split out so a test can
// drive it directly against a dialog it built and edited itself, without
// simulating a button click through vtui.FrameManager -- the same pattern
// submitGenerate already uses for "Generate hashes" (f4#1623 point 1). It
// resolves the typed paths, persists the "Ignore missing files"/"Stop on
// first mismatch" checkboxes through p.store (the encoding choice is not
// persisted here on purpose -- see saveValidateOptions) and starts the run.
func (p *Plugin) submitValidate(app vfs.App, fs vfs.VFS, panelDir string, d *validateDialog) {
	hashPath, dir, ok := resolveValidateInput(fs, panelDir, d.editFile.GetText(), d.editDir.GetText())
	if !ok {
		vtui.ShowMessage(vtui.Msg("IntChecker.Title"), vtui.Msg("IntChecker.EnterChecksumFile"), []string{vtui.Msg("vtui.Ok")})
		return
	}
	d.win.Close()
	chosen := validateOptions{
		encoding:       d.encoding.selected(),
		ignoreMissing:  d.ignoreMissing.State == 1,
		stopOnMismatch: d.stopOnMismatch.State == 1,
	}
	if err := p.saveValidateOptions(chosen); err != nil && p.api != nil {
		p.api.Log("integrity checker: " + err.Error())
	}
	go startValidate(app, fs, hashPath, dir, chosen, p.openValidateDialog)
}

// saveValidateOptions persists the "Ignore missing files"/"Stop on first
// mismatch" checkboxes (f4#1623 point 2) into the same settings file and
// store the generate dialog uses, without touching any of its own fields.
func (p *Plugin) saveValidateOptions(opts validateOptions) error {
	settings := p.store.snapshot()
	settings.ValidateIgnoreMissing = opts.ignoreMissing
	settings.ValidateStopOnMismatch = opts.stopOnMismatch
	return p.store.save(settings)
}

// resolveValidateInput turns the dialog fields into paths. Relative paths are
// taken from the panel directory, and an empty directory means the panel
// directory itself, as the reporter of f4#1623 asked.
func resolveValidateInput(fs vfs.VFS, panelDir, hashText, dirText string) (hashPath, dir string, ok bool) {
	hashText, dirText = strings.TrimSpace(hashText), strings.TrimSpace(dirText)
	if hashText == "" {
		return "", "", false
	}
	resolve := func(p string) string {
		if fs.IsAbs(p) {
			return p
		}
		return fs.Join(panelDir, p)
	}
	dir = panelDir
	if dirText != "" {
		dir = resolve(dirText)
	}
	return resolve(hashText), dir, true
}

// selectionTokenFor captures the panel selection of the checksum file at
// path, when it sits directly in the panel directory fs is rooted at
// (fs.GetPath()) -- the only case where an entry name in that panel and path
// name the same file. It is nil when the checksum file comes from elsewhere
// (a different directory typed into the dialog) or the host does not support
// clearing selection (vfs.SelectionClearHost); clearing it later, once
// validation succeeds, is what f4#1623 asked for.
func selectionTokenFor(app vfs.App, fs vfs.VFS, path string) vfs.SelectionToken {
	host, ok := app.(vfs.SelectionClearHost)
	if !ok {
		return nil
	}
	name := fs.Base(path)
	if name == "" || name == "." || name == ".." || fs.Join(fs.GetPath(), name) != path {
		return nil
	}
	token, exists := host.CaptureSelectionToken(name)
	if !exists {
		return nil
	}
	return token
}

// startValidate loads the checksum file, decodes it as opts.encoding says
// (see decodeChecksumFile) and runs the check with a progress dialog. It
// waits for message answers, so it must not run on the UI goroutine. reopen
// is what finishValidate calls when none of the listed files was found.
func startValidate(app vfs.App, fs vfs.VFS, hashPath, dir string, opts validateOptions, reopen validateReopener) {
	title := vtui.Msg("IntChecker.Title")
	ok := []string{vtui.Msg("vtui.Ok")}
	ctx := context.Background()
	// Captured before any I/O, from the panel state as it was when the
	// command started (f4#1623: cleared on success, left alone otherwise).
	token := selectionTokenFor(app, fs, hashPath)
	if item, err := fs.Stat(ctx, dir); err != nil || !item.IsDir {
		app.Message(title, fmt.Sprintf(vtui.Msg("IntChecker.DirNotFound"), dir), ok)
		return
	}
	data, err := readChecksumFile(ctx, fs, hashPath)
	if err != nil {
		app.Message(title, fmt.Sprintf(vtui.Msg("IntChecker.ReadChecksumError"), hashPath, err), ok)
		return
	}
	text, _, err := decodeChecksumFile(data, opts.encoding.Codepage)
	if err != nil {
		app.Message(title, fmt.Sprintf(vtui.Msg("IntChecker.ReadChecksumError"), hashPath, err), ok)
		return
	}
	file, err := ParseHashFile(fs.Base(hashPath), text)
	if err != nil {
		app.Message(title, parseErrorText(hashPath, err), ok)
		return
	}
	job := validateJob{
		fs: fs, hashPath: hashPath, dir: dir, file: file, encoding: opts.encoding,
		ignoreMissing:  opts.ignoreMissing,
		stopOnMismatch: opts.stopOnMismatch,
	}
	var res validateResult
	app.RunAdvancedProgressTask(vtui.Msg("IntChecker.ValidateTitle"), false, func(ctx context.Context, reporter vfs.TaskReporter) error {
		var err error
		res, err = runValidate(ctx, job, reporter)
		return err
	}, func(err error) {
		finishValidate(app, job, res, err, token, reopen)
	})
}

func parseErrorText(hashPath string, err error) string {
	switch {
	case errors.Is(err, errNoChecksums):
		return fmt.Sprintf(vtui.Msg("IntChecker.NoChecksums"), hashPath)
	case errors.Is(err, errUnknownHashLength):
		return fmt.Sprintf(vtui.Msg("IntChecker.UnknownHashLength"), hashPath)
	}
	return fmt.Sprintf(vtui.Msg("IntChecker.ReadChecksumError"), hashPath, err)
}

// finishValidate runs on the UI goroutine when the check ends: it shows the
// report, or asks for another directory when no listed file was found.
// token, captured by startValidate before the run, is cleared once the run
// is confirmed successful (f4#1623): cancelled, failed, or "files not found"
// runs (which re-ask for a directory, not a finished operation) leave the
// panel selection alone.
func finishValidate(app vfs.App, job validateJob, res validateResult, err error, token vfs.SelectionToken, reopen validateReopener) {
	title := vtui.Msg("IntChecker.Title")
	ok := []string{vtui.Msg("vtui.Ok")}
	switch {
	case errors.Is(err, context.Canceled):
		go app.Message(title, vtui.Msg("IntChecker.ValidateCancelled"), ok)
	case errors.Is(err, errAllMissing):
		opts := validateOptions{encoding: job.encoding, ignoreMissing: job.ignoreMissing, stopOnMismatch: job.stopOnMismatch}
		reopen(app, job.fs, vtui.Msg("IntChecker.FilesNotFound"), job.hashPath, job.dir, opts)
	case err != nil:
		go app.Message(title, err.Error(), ok)
	default:
		if token != nil {
			token.Clear()
		}
		go app.Message(title, validateReport(res, job.file.Malformed), ok)
	}
}
