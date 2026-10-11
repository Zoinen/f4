package intchecker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// Command IDs for the plugin's host registrations. The menu command keeps
// its original ID so existing key bindings and macros do not shift; the
// generate/validate commands are separate IDs so each gets its own entry in
// the Files menu and its own assignable hotkey (f4#1623 part 4).
const (
	menuCommandID     = "intchecker.menu"
	generateCommandID = "intchecker.generate"
	validateCommandID = "intchecker.validate"
)

// Plugin is the integrity checker's host registration.
type Plugin struct {
	api           vfs.HostAPI
	registrations []vfs.Registration
	configDir     string
	store         *settingsStore
}

// NewPlugin returns the integrity checker plugin. configDir is where its
// generate settings (Settings, see settings.go) persist between runs; an
// empty string falls back to vfs.CustomConfigDir or os.UserConfigDir(), the
// same order plugins/mediainfo and plugins/proclist use.
func NewPlugin(configDir string) *Plugin { return &Plugin{configDir: configDir} }

func (p *Plugin) GetName() string { return "Integrity Checker" }

func (p *Plugin) Init(api vfs.HostAPI) error {
	p.api = api
	store, loadErr := newSettingsStore(p.settingsDirectory())
	p.store = store
	if loadErr != nil {
		api.Log("integrity checker: " + loadErr.Error() + "; using defaults")
	}
	contributions, ok := api.(vfs.ContributionHost)
	if !ok {
		api.RegisterPluginMenuItem(vtui.Msg("IntChecker.Menu"), p.showMenu)
		return nil
	}
	commands := []vfs.PluginCommand{
		{
			ID:             menuCommandID,
			Location:       vfs.PluginCommandPanel,
			Label:          "Integrity &checker",
			LabelKey:       "IntChecker.Menu",
			Description:    "Generate and verify checksum files (SFV, MD5, SHA)",
			DescriptionKey: "IntChecker.Command.Desc",
			SearchKeys:     []string{"IntChecker.Generate", "IntChecker.Validate"},
			SearchTerms:    []string{"checksum", "hash", "crc32", "md5", "sha1", "sha256", "sfv", "verify"},
			Enabled:        canRun,
			Run:            p.showMenu,
		},
		{
			// Direct Files-menu entries, by the same pattern as
			// plugins/archive's "Add to archive"/"Extract files": a
			// panel command with MenuPath "Files" gets a Files-menu
			// row and, through the host's generic plugin-command
			// keymap (internal/panel/plugin_hotkeys.go), a hotkey the
			// user can assign and reassign with F4, instead of a
			// hotkey hardcoded by the plugin.
			ID:             generateCommandID,
			Location:       vfs.PluginCommandPanel,
			Label:          "Calculate files checksum",
			LabelKey:       "IntChecker.Command.Generate",
			MenuPath:       "Files",
			Description:    "Generate checksums for the selected files",
			DescriptionKey: "IntChecker.Command.Generate.Desc",
			SearchKeys:     []string{"IntChecker.Generate"},
			SearchTerms:    []string{"checksum", "hash", "crc32", "md5", "sha1", "sha256", "sfv"},
			Enabled:        canWorkOnSelection,
			Run:            p.showGenerateDialog,
		},
		{
			ID:             validateCommandID,
			Location:       vfs.PluginCommandPanel,
			Label:          "Verify files checksum",
			LabelKey:       "IntChecker.Command.Validate",
			MenuPath:       "Files",
			Description:    "Verify files against a checksum file",
			DescriptionKey: "IntChecker.Command.Validate.Desc",
			SearchKeys:     []string{"IntChecker.Validate"},
			SearchTerms:    []string{"checksum", "hash", "verify", "check", "sfv"},
			Enabled:        canWorkOnSelection,
			Run:            p.showValidate,
		},
	}
	for _, command := range commands {
		registration, err := contributions.RegisterPluginCommand(command)
		if err != nil {
			for _, done := range p.registrations {
				done.Unregister()
			}
			p.registrations = nil
			p.api = nil
			return fmt.Errorf("integrity checker: register %s command: %w", command.ID, err)
		}
		p.registrations = append(p.registrations, registration)
	}
	return nil
}

func (p *Plugin) Close() error {
	for _, registration := range p.registrations {
		registration.Unregister()
	}
	p.registrations = nil
	p.api = nil
	return nil
}

// settingsDirectory is where the generate settings file lives, in the same
// order plugins/mediainfo's plugin.go resolves its own settings directory.
func (p *Plugin) settingsDirectory() string {
	if strings.TrimSpace(p.configDir) != "" {
		return p.configDir
	}
	if strings.TrimSpace(vfs.CustomConfigDir) != "" {
		return vfs.CustomConfigDir
	}
	if directory, err := os.UserConfigDir(); err == nil {
		return filepath.Join(directory, "f4")
	}
	return "."
}

// canRun dims the command when there is no panel filesystem to work on.
func canRun(app vfs.App) bool {
	return app.GetActivePanelVFS() != nil
}

// canWorkOnSelection dims the Files-menu commands when nothing but ".." is
// under the cursor and nothing is marked (f4#1356).
func canWorkOnSelection(app vfs.App) bool {
	return canRun(app) && len(selectedFileNames(app)) > 0
}

// Menu items, in the order showMenu lists them.
const (
	menuGenerate = iota
	menuValidate
)

func (p *Plugin) showMenu(app vfs.App) {
	app.Menu(vtui.Msg("IntChecker.Title"), []string{
		vtui.Msg("IntChecker.Generate"),
		vtui.Msg("IntChecker.Validate"),
	}, func(idx int) {
		switch idx {
		case menuGenerate:
			p.showGenerateDialog(app)
		case menuValidate:
			p.showValidate(app)
		}
	})
}

// selectedFileNames is what "Generate hashes" works on: the marked panel
// items, or the one under the cursor.
func selectedFileNames(app vfs.App) []string {
	var names []string
	for _, name := range app.GetSelectedNames() {
		if name != "" && name != ".." {
			names = append(names, name)
		}
	}
	return names
}

// captureSelectionTokens snapshots names' current panel selection, when the
// host supports it (vfs.SelectionClearHost). Clearing them later, once the
// operation that used them succeeds, is what f4#1623 asked for: files and
// folders that were just hashed or verified stop being marked. Hosts (and
// every test double in this package) that don't implement the optional
// interface simply get no tokens back, and clearSelectionTokens is then a
// no-op -- the same graceful fallback vfs.SelectedIsDirHost already uses.
func captureSelectionTokens(app vfs.App, names []string) map[string]vfs.SelectionToken {
	host, ok := app.(vfs.SelectionClearHost)
	if !ok {
		return nil
	}
	var tokens map[string]vfs.SelectionToken
	for _, name := range names {
		token, exists := host.CaptureSelectionToken(name)
		if !exists {
			continue
		}
		if tokens == nil {
			tokens = make(map[string]vfs.SelectionToken, len(names))
		}
		tokens[name] = token
	}
	return tokens
}

// clearSelectionTokens drops every captured token's selection, if it is
// still exactly what was captured (SelectionToken.Clear).
func clearSelectionTokens(tokens map[string]vfs.SelectionToken) {
	for _, token := range tokens {
		token.Clear()
	}
}

// generateDialog asks how to generate the hashes.
type generateDialog struct {
	win        *vtui.Window
	algorithm  *vtui.RadioGroup
	output     *vtui.RadioGroup
	editOutput *vtui.Edit
	recursive  *vtui.Checkbox
	absolute   *vtui.Checkbox
	editMask   *vtui.Edit
	encoding   *encodingCombo
	btnOK      *vtui.Button
}

// defaultMask is the file mask the dialog offers: every file.
const defaultMask = "*"

// outputModeNames lists the "Output to" choices in radio button order.
func outputModeNames() []string {
	return []string{
		vtui.Msg("IntChecker.OutputSingle"),
		vtui.Msg("IntChecker.OutputSeparate"),
		vtui.Msg("IntChecker.OutputDirectory"),
		vtui.Msg("IntChecker.OutputDisplay"),
	}
}

// newGenerateDialog builds the dialog for a panel directory named dirBase,
// preset to settings -- the previous run's choices (settings.go), everything
// but the output file name and the mask: the file name field always starts
// from dirBase and the mask field always starts at defaultMask, since the
// author asked (f4#1623) to keep exactly those two fresh on every run. The
// file name field belongs to the "Single file" output and is disabled for
// the others. The file encoding applies to every output, including "Save to
// file" of the display window.
func newGenerateDialog(dirBase string, settings Settings) *generateDialog {
	width, height := 60, 24
	d := &generateDialog{win: vtui.NewCenteredDialog(width, height, vtui.Msg("IntChecker.GenerateTitle"))}
	d.win.ShowClose = true

	d.algorithm = vtui.NewRadioGroup(0, 0, 2, algorithmNames())
	d.algorithm.Selected = int(settings.Algorithm)
	lblAlgorithm := vtui.NewLabel(0, 0, vtui.Msg("IntChecker.Algorithm"), d.algorithm)

	d.output = vtui.NewRadioGroup(0, 0, 1, outputModeNames())
	d.output.Selected = int(settings.Output)
	lblOutput := vtui.NewLabel(0, 0, vtui.Msg("IntChecker.OutputTo"), d.output)

	d.editOutput = vtui.NewEdit(0, 0, width-6, defaultOutputName(dirBase, settings.Algorithm))
	lblOutputEdit := vtui.NewLabel(0, 0, vtui.Msg("IntChecker.OutputFileName"), d.editOutput)

	current := settings.Algorithm
	d.algorithm.OnChange = func(idx int) {
		next := Algorithm(idx)
		if !next.valid() {
			return
		}
		d.editOutput.SetText(switchExtension(d.editOutput.GetText(), current, next))
		current = next
	}
	d.output.OnChange = func(idx int) {
		d.editOutput.SetDisabled(outputMode(idx) != outputSingle)
	}

	d.recursive = vtui.NewCheckbox(0, 0, vtui.Msg("IntChecker.Recursive"), false)
	d.recursive.State = checkboxState(settings.Recursive)
	d.absolute = vtui.NewCheckbox(0, 0, vtui.Msg("IntChecker.AbsolutePaths"), false)
	d.absolute.State = checkboxState(settings.Absolute)
	d.editMask = vtui.NewEdit(0, 0, 10, defaultMask)
	lblMask := vtui.NewLabel(0, 0, vtui.Msg("IntChecker.FileMask"), d.editMask)
	// The mask field takes the rest of its row.
	lx1, _, lx2, _ := lblMask.GetPosition()
	maskWidth := width - 4 - (lx2 - lx1 + 1) - 1
	d.editMask.SetPosition(0, 0, maskWidth-1, 0)
	d.encoding = newEncodingCombo(24, writeEncodingChoices(), settings.Encoding)
	lblEncoding := vtui.NewLabel(0, 0, vtui.Msg("IntChecker.FileEncoding"), d.encoding.box)

	d.btnOK = vtui.NewButton(0, 0, vtui.Msg("vtui.Ok"))
	d.btnOK.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, vtui.Msg("vtui.Cancel"))
	btnCancel.OnClick = func() { d.win.Close() }

	for _, item := range []vtui.UIElement{lblAlgorithm, d.algorithm, lblOutput, d.output, lblOutputEdit, d.editOutput, d.recursive, d.absolute, lblMask, d.editMask, lblEncoding, d.encoding.box, d.btnOK, btnCancel} {
		d.win.AddItem(item)
	}

	vbox := vtui.NewVBoxLayout(d.win.X1+2, d.win.Y1+2, width-4, height-4)
	vbox.Add(lblAlgorithm, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(d.algorithm, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(lblOutput, vtui.Margins{Top: 1}, vtui.AlignLeft)
	vbox.Add(d.output, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(lblOutputEdit, vtui.Margins{Top: 1}, vtui.AlignLeft)
	vbox.Add(d.editOutput, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(d.recursive, vtui.Margins{Top: 1}, vtui.AlignLeft)
	vbox.Add(d.absolute, vtui.Margins{}, vtui.AlignLeft)
	maskRow := vtui.NewHBoxLayout(0, 0, width-4, 1)
	maskRow.Add(lblMask, vtui.Margins{}, vtui.AlignLeft)
	maskRow.Add(d.editMask, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(maskRow, vtui.Margins{}, vtui.AlignFill)
	encodingRow := vtui.NewHBoxLayout(0, 0, width-4, 1)
	encodingRow.Spacing = 1
	encodingRow.Add(lblEncoding, vtui.Margins{}, vtui.AlignLeft)
	encodingRow.Add(d.encoding.box, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(encodingRow, vtui.Margins{}, vtui.AlignFill)
	buttons := vtui.NewHBoxLayout(0, 0, width-4, 1)
	buttons.HorizontalAlign = vtui.AlignCenter
	buttons.Spacing = 2
	buttons.Add(d.btnOK, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(buttons, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()
	return d
}

// checkboxState turns a persisted bool into the 0/1 vtui.Checkbox uses.
func checkboxState(on bool) int {
	if on {
		return 1
	}
	return 0
}

func (p *Plugin) showGenerateDialog(app vfs.App) {
	fs := app.GetActivePanelVFS()
	if fs == nil {
		return
	}
	names := selectedFileNames(app)
	if len(names) == 0 {
		vtui.ShowMessage(vtui.Msg("IntChecker.Title"), vtui.Msg("IntChecker.NothingSelected"), []string{vtui.Msg("vtui.Ok")})
		return
	}
	dir := fs.GetPath()
	d := newGenerateDialog(fs.Base(dir), p.store.snapshot())
	d.btnOK.OnClick = func() { p.submitGenerate(app, fs, dir, names, d) }
	vtui.FrameManager.Push(d.win)
}

// submitGenerate validates the dialog, persists the settings it remembers
// (f4#1623 point 1: every generate setting except the output file name and
// the mask, which the dialog always asks for fresh), closes the dialog and
// starts the job. Split out of showGenerateDialog's OnClick so a test can
// drive it directly, against a dialog it built and edited itself, without
// simulating a button click through vtui.FrameManager.
func (p *Plugin) submitGenerate(app vfs.App, fs vfs.VFS, dir string, names []string, d *generateDialog) {
	algorithm := Algorithm(d.algorithm.Selected)
	mode := outputMode(d.output.Selected)
	if !algorithm.valid() || !mode.valid() {
		return
	}
	output := strings.TrimSpace(d.editOutput.GetText())
	if mode == outputSingle && !validOutputName(output) {
		vtui.ShowMessage(vtui.Msg("IntChecker.Title"), vtui.Msg("IntChecker.BadOutputName"), []string{vtui.Msg("vtui.Ok")})
		return
	}
	d.win.Close()
	recursive := d.recursive.State == 1
	absolute := d.absolute.State == 1
	encoding := d.encoding.selected()
	// Start from the current snapshot, not a bare Settings{}: f4#1623 point 2
	// added the ValidateIgnoreMissing/ValidateStopOnMismatch fields to the
	// same struct, and a literal listing only the generate fields would
	// silently reset them to false on every "Generate hashes" run.
	toSave := p.store.snapshot()
	toSave.Algorithm = algorithm
	toSave.Output = mode
	toSave.Recursive = recursive
	toSave.Absolute = absolute
	toSave.Encoding = encoding
	if err := p.store.save(toSave); err != nil && p.api != nil {
		p.api.Log("integrity checker: " + err.Error())
	}
	job := generateJob{
		fs: fs, dir: dir, names: names, algorithm: algorithm, mode: mode, output: output,
		recursive: recursive,
		absolute:  absolute,
		mask:      strings.TrimSpace(d.editMask.GetText()),
		encoding:  encoding,
	}
	startGenerate(app, job)
}

// validOutputName accepts a plain file name in the current directory. Other
// directories come with the later output modes.
func validOutputName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, `/\`+"\x00")
}

// confirmOverwrite asks what to do with checksum files that already exist and
// updates the job. It returns false when the user cancelled. It waits for the
// answer, so it must not run on the UI goroutine.
func confirmOverwrite(app vfs.App, job *generateJob, existing []string) bool {
	if len(existing) == 0 {
		return true
	}
	title := vtui.Msg("IntChecker.Title")
	if !job.mode.writesManyFiles() {
		answer := app.Message(title,
			fmt.Sprintf(vtui.Msg("IntChecker.OverwriteQuestion"), existing[0]),
			[]string{vtui.Msg("IntChecker.Overwrite"), vtui.Msg("vtui.Cancel")})
		if answer != 0 {
			return false
		}
		job.overwrite = true
		return true
	}
	text := fmt.Sprintf(vtui.Msg("IntChecker.OverwriteQuestion"), existing[0])
	if len(existing) > 1 {
		text = fmt.Sprintf(vtui.Msg("IntChecker.OverwriteManyQuestion"), len(existing), existing[0])
	}
	switch app.Message(title, text, []string{vtui.Msg("IntChecker.Overwrite"), vtui.Msg("IntChecker.Skip"), vtui.Msg("vtui.Cancel")}) {
	case 0:
		job.overwrite = true
	case 1:
		job.skipExisting = true
	default:
		return false
	}
	return true
}

// startGenerate runs a job in two steps, each with its own progress and
// cancellation: first the selected files are collected (walking the selected
// directories when the job is recursive) and the existing checksum files are
// found, then, once the user has answered the overwrite question, everything
// is hashed and written.
func startGenerate(app vfs.App, job generateJob) {
	var (
		res      generateResult
		inputs   []hashInput
		existing []string
	)
	// Captured before anything runs, so it reflects what the user marked at
	// the moment they confirmed the dialog; cleared on success (f4#1623),
	// left untouched otherwise (cancelled, failed, or the user re-marked
	// something while the job ran -- ClearSelectionIfUnchanged then no-ops).
	tokens := captureSelectionTokens(app, job.names)
	title := vtui.Msg("IntChecker.GenerateTitle")
	hash := func(job generateJob) {
		app.RunAdvancedProgressTask(title, false, func(ctx context.Context, reporter vfs.TaskReporter) error {
			var err error
			res, err = hashInputs(ctx, job, inputs, res, reporter)
			return err
		}, func(err error) {
			finishGenerate(app, job, res, err, tokens)
		})
	}
	app.RunAdvancedProgressTask(title, false, func(ctx context.Context, reporter vfs.TaskReporter) error {
		var err error
		inputs, err = collectInputs(ctx, job, &res, func(dir string, files, dirs int64) {
			reporter.UpdateScan(dir, files, dirs)
		})
		if err != nil {
			return err
		}
		existing, err = existingOutputs(ctx, job, inputs)
		return err
	}, func(err error) {
		if err != nil {
			finishGenerate(app, job, res, err, tokens)
			return
		}
		if len(existing) == 0 || len(inputs) == 0 {
			hash(job)
			return
		}
		// app.Message waits for the answer, so the overwrite question
		// runs off the UI goroutine.
		go func() {
			if confirmOverwrite(app, &job, existing) {
				hash(job)
			}
		}()
	})
}

// finishGenerate tells the user how the run ended, puts the panel cursor on
// the new checksum file and, for the display mode, opens the list window. It
// runs on the UI goroutine. tokens, captured by startGenerate before the run,
// are cleared once the run is confirmed successful (f4#1623): cancelled or
// failed runs leave the panel selection alone.
func finishGenerate(app vfs.App, job generateJob, res generateResult, err error, tokens map[string]vfs.SelectionToken) {
	title := vtui.Msg("IntChecker.Title")
	ok := []string{vtui.Msg("vtui.Ok")}
	if len(res.Outputs) > 0 {
		if !strings.Contains(res.Outputs[0], "/") {
			app.SetPendingSelection(res.Outputs[0])
		}
		app.RefreshAll()
	}
	switch {
	case errors.Is(err, context.Canceled):
		go app.Message(title, vtui.Msg("IntChecker.Cancelled"), ok)
		return
	case errors.Is(err, errNothingToHash):
		go app.Message(title, vtui.Msg("IntChecker.NoFiles"), ok)
		return
	case err != nil && job.mode == outputSingle:
		go app.Message(title, fmt.Sprintf(vtui.Msg("IntChecker.WriteError"), job.output, err), ok)
		return
	case err != nil:
		go app.Message(title, err.Error(), ok)
		return
	}
	clearSelectionTokens(tokens)
	report := generateReport(job, res)
	if job.mode == outputDisplay && res.Text != "" {
		showHashList(app, job, res.Text, report)
		return
	}
	if report != "" {
		go app.Message(title, report, ok)
	}
}

// generateReport describes what went wrong in a finished run: checksum files
// kept or not written and files that could not be read. It is empty when
// everything went fine.
func generateReport(job generateJob, res generateResult) string {
	var lines []string
	const shown = 10
	appendFailures := func(header string, failures []fileFailure) {
		if len(failures) == 0 {
			return
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, fmt.Sprintf(header, len(failures)))
		for i, failure := range failures {
			if i == shown {
				lines = append(lines, fmt.Sprintf(vtui.Msg("IntChecker.MoreErrors"), len(failures)-shown))
				break
			}
			lines = append(lines, fmt.Sprintf("%s: %v", failure.Name, failure.Err))
		}
	}
	if job.mode.writesManyFiles() && (len(res.SkippedExisting) > 0 || len(res.WriteFailures) > 0) {
		lines = append(lines, fmt.Sprintf(vtui.Msg("IntChecker.FilesWritten"), len(res.Outputs)))
		if len(res.SkippedExisting) > 0 {
			lines = append(lines, fmt.Sprintf(vtui.Msg("IntChecker.SkippedExisting"), len(res.SkippedExisting)))
		}
	}
	appendFailures(vtui.Msg("IntChecker.WriteErrors"), res.WriteFailures)
	appendFailures(vtui.Msg("IntChecker.ReadErrors"), res.Failures)
	if len(res.Unencodable) > 0 {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, fmt.Sprintf(vtui.Msg("IntChecker.UnencodableNames"), len(res.Unencodable), job.encoding.name()))
		for i, name := range res.Unencodable {
			if i == shown {
				lines = append(lines, fmt.Sprintf(vtui.Msg("IntChecker.MoreErrors"), len(res.Unencodable)-shown))
				break
			}
			lines = append(lines, name)
		}
	}
	return strings.Join(lines, "\n")
}
