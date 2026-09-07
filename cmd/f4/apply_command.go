package main

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

var lastApplyCommandTemplate string

var foregroundApplyCommands = struct {
	sync.Mutex
	next uint64
	runs map[uint64]foregroundApplyCommand
}{runs: make(map[uint64]foregroundApplyCommand)}

type foregroundApplyCommand struct {
	cancel context.CancelFunc
	done   chan struct{}
}

type applyPanelCapture struct {
	panel    *FileSystemPanel
	panelVFS vfs.VFS
	vfs      vfs.VFS
	dir      string
	snapshot ApplyCommandPanel
}

type applyCommandSession struct {
	pf         *PanelsFrame
	active     applyPanelCapture
	passive    applyPanelCapture
	targets    []string
	explicit   bool
	tokens     map[string]panelSelectionToken
	runner     vfs.CommandRunner
	info       vfs.CommandRunnerInfo
	template   *CompiledApplyCommand
	values     ApplyCommandPromptValues
	silent     bool
	mode       ApplyCommandMode
	workers    int
	activeSide ApplyCommandPanelSide
	items      []applyBatchItem
	ownedVFS   []vfs.VFS
	releaseVFS sync.Once
}

func registerForegroundApplyCommand(cancel context.CancelFunc) func() {
	foregroundApplyCommands.Lock()
	foregroundApplyCommands.next++
	id := foregroundApplyCommands.next
	entry := foregroundApplyCommand{cancel: cancel, done: make(chan struct{})}
	foregroundApplyCommands.runs[id] = entry
	foregroundApplyCommands.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			foregroundApplyCommands.Lock()
			delete(foregroundApplyCommands.runs, id)
			foregroundApplyCommands.Unlock()
			close(entry.done)
		})
	}
}

func cancelAllForegroundApplyCommands() {
	foregroundApplyCommands.Lock()
	cancels := make([]context.CancelFunc, 0, len(foregroundApplyCommands.runs))
	for _, run := range foregroundApplyCommands.runs {
		cancels = append(cancels, run.cancel)
	}
	foregroundApplyCommands.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func activeForegroundApplyCommandCount() int {
	foregroundApplyCommands.Lock()
	defer foregroundApplyCommands.Unlock()
	return len(foregroundApplyCommands.runs)
}

func panelCanApplyCommand() bool {
	pf := findPanelsFrame()
	if pf == nil {
		return false
	}
	panel := pf.getActivePanel()
	if panel == nil || panel.vfs == nil {
		return false
	}
	_, _, ok := resolveApplyCommandRunner(panel.vfs)
	return ok
}

func actionApplyCommand(pf *PanelsFrame) {
	if pf == nil || vtui.FrameManager == nil {
		return
	}
	active := pf.getActivePanel()
	passive := pf.getInactivePanel()
	if active == nil || active.vfs == nil {
		return
	}

	marked := active.GetMarkedNames()
	targets := append([]string(nil), marked...)
	explicit := len(targets) != 0
	if !explicit {
		targets = active.GetSelectedNames()
	}
	if len(targets) == 0 {
		vtui.ShowMessageOn(pf, Msg("ApplyCommand.NoTargetsTitle"), Msg("ApplyCommand.NoTargets"), []string{Msg("vtui.Ok")})
		return
	}
	runner, info, ok := resolveApplyCommandRunner(active.vfs)
	if !ok {
		vtui.ShowMessageOn(pf, Msg("ApplyCommand.UnsupportedTitle"), Msg("ApplyCommand.Unsupported"), []string{Msg("vtui.Ok")})
		return
	}

	tokens := make(map[string]panelSelectionToken, len(marked))
	for _, name := range marked {
		if token, exists := active.captureSelectionToken(name); exists {
			tokens[name] = token
		}
	}
	session := &applyCommandSession{
		pf: pf, active: captureApplyCommandPanel(active, targets), passive: captureApplyCommandPanel(passive, nil),
		targets: targets, explicit: explicit, tokens: tokens, runner: runner, info: info,
	}
	if pf.activeIdx == 1 {
		session.activeSide = ApplyCommandRightSide
	}
	showApplyCommandDialog(session)
}

func resolveApplyCommandRunner(target vfs.VFS) (vfs.CommandRunner, vfs.CommandRunnerInfo, bool) {
	if availability, ok := target.(vfs.CommandRunnerAvailabilityProvider); ok && !availability.CommandRunnerAvailable() {
		return nil, vfs.CommandRunnerInfo{}, false
	}
	if _, local := target.(*vfs.OSVFS); local {
		runner := NewLocalCommandRunner()
		return runner, runner.CommandRunnerInfo(), true
	}
	runner, ok := target.(vfs.CommandRunner)
	if !ok {
		return nil, vfs.CommandRunnerInfo{}, false
	}
	info := vfs.CommandRunnerInfo{Dialect: vfs.CommandDialectUnknown, MaxParallel: 1}
	if provider, hasInfo := target.(vfs.CommandRunnerInfoProvider); hasInfo {
		info = provider.CommandRunnerInfo()
		if info.MaxParallel < 0 {
			info.MaxParallel = 1
		}
	}
	switch info.Dialect {
	case vfs.CommandDialectUnknown, vfs.CommandDialectPOSIX, vfs.CommandDialectCmd, vfs.CommandDialectPowerShell:
	default:
		// Optional provider metadata is an external extension point. Treat
		// values added by a newer or faulty provider as unknown instead of
		// accidentally selecting the raw/no-quoting path.
		info.Dialect = vfs.CommandDialectUnknown
	}
	return runner, info, true
}

func captureApplyCommandPanel(panel *FileSystemPanel, selectedOverride []string) applyPanelCapture {
	if panel == nil || panel.vfs == nil {
		return applyPanelCapture{}
	}
	dir := panel.vfs.GetPath()
	current := panel.getRawSelectedName()
	if current == ".." {
		current = ""
	}
	selected := append([]string(nil), selectedOverride...)
	if selected == nil {
		selected = panel.GetMarkedNames()
		if len(selected) == 0 && current != "" {
			selected = []string{current}
		}
	}
	realDir := dir
	if abs, err := panel.vfs.Abs(dir); err == nil && abs != "" {
		realDir = abs
	}
	if _, local := panel.vfs.(*vfs.OSVFS); local {
		if resolved, err := filepath.EvalSymlinks(realDir); err == nil {
			realDir = resolved
		}
	}
	shortDir := applyCommandShortPath(dir)
	realShortDir := applyCommandShortPath(realDir)
	makeFile := func(name string) ApplyCommandFile {
		if name == "" {
			return ApplyCommandFile{}
		}
		short := name
		if _, local := panel.vfs.(*vfs.OSVFS); local {
			if shortPath := applyCommandShortPath(panel.vfs.Join(dir, name)); shortPath != "" {
				short = filepath.Base(shortPath)
			}
		}
		return ApplyCommandFile{Name: name, ShortName: short}
	}
	selectedFiles := make([]ApplyCommandFile, 0, len(selected))
	for _, name := range selected {
		if name != "" && name != ".." {
			selectedFiles = append(selectedFiles, makeFile(name))
		}
	}
	return applyPanelCapture{
		panel: panel, panelVFS: panel.vfs, vfs: panel.vfs, dir: dir,
		snapshot: ApplyCommandPanel{
			PathStyle: detectApplyCommandPathStyle(panel.vfs, dir),
			Directory: dir, ShortDirectory: shortDir, RealDirectory: realDir, RealShortDirectory: realShortDir,
			Current: makeFile(current), Selected: selectedFiles,
		},
	}
}

func detectApplyCommandPathStyle(filesystem vfs.VFS, directory string) ApplyCommandPathStyle {
	if filesystem != nil {
		joined := filesystem.Join("f4-apply-style-a", "f4-apply-style-b")
		if strings.Contains(joined, `\`) && !strings.Contains(joined, "/") {
			return ApplyCommandPathStyleWindows
		}
		if strings.Contains(joined, "/") {
			return ApplyCommandPathStylePOSIX
		}
	}
	return effectiveApplyCommandPathStyle(directory, ApplyCommandPathStyleUnknown)
}

func (s *applyCommandSession) contextFor(name string) ApplyCommandContext {
	active := s.active.snapshot
	active.Current = applyCommandFileForTarget(s.active, name)
	return ApplyCommandContext{
		Dialect: applyCommandDialect(s.info.Dialect), ActiveSide: s.activeSide,
		Active: active, Passive: s.passive.snapshot,
	}
}

func applyCommandFileForTarget(capture applyPanelCapture, name string) ApplyCommandFile {
	for _, file := range capture.snapshot.Selected {
		if file.Name == name {
			return file
		}
	}
	return ApplyCommandFile{Name: name, ShortName: name}
}

func applyCommandDialect(dialect vfs.CommandDialect) ApplyCommandDialect {
	switch dialect {
	case vfs.CommandDialectPOSIX:
		return ApplyCommandDialectPOSIX
	case vfs.CommandDialectCmd:
		return ApplyCommandDialectCMD
	case vfs.CommandDialectPowerShell:
		return ApplyCommandDialectPowerShell
	default:
		return ApplyCommandDialectRaw
	}
}

func showApplyCommandDialog(session *applyCommandSession) {
	const width, height = 72, 15
	dlg := vtui.NewCenteredDialog(width, height, Msg("ApplyCommand.Title"))
	dlg.ShowClose = true
	dlg.SetHelp("ApplyCmd")

	initial := lastApplyCommandTemplate
	editCommand := vtui.NewEdit(0, 0, width-4, initial)
	editCommand.HistoryID = "ApplyCmd"
	editCommand.ShowHistoryButton = true
	editCommand.DeduplicateHistory = true
	if vtui.GlobalHistoryProvider != nil {
		editCommand.History = vtui.GlobalHistoryProvider.LoadHistory(editCommand.HistoryID)
		if initial == "" && len(editCommand.History) > 0 {
			editCommand.SetText(editCommand.History[0])
		}
	}
	lblCommand := vtui.NewLabel(0, 0, Msg("ApplyCommand.Prompt"), editCommand)
	txtTargets := vtui.NewText(0, 0, fmt.Sprintf(Msg("ApplyCommand.TargetsFmt"), len(session.targets)), 0)

	modes := []string{Msg("ApplyCommand.ModeSequential"), Msg("ApplyCommand.ModeParallel"), Msg("ApplyCommand.ModeQueue")}
	comboMode := vtui.NewComboBox(0, 0, 24, modes)
	comboMode.DropdownOnly = true
	comboMode.Menu.SetSelectPos(0)
	comboMode.Edit.SetText(modes[0])
	lblMode := vtui.NewLabel(0, 0, Msg("ApplyCommand.Mode"), comboMode)

	workerDefault := AppConfig.ApplyCommandParallelism
	if workerDefault <= 0 {
		workerDefault = runtime.NumCPU()
	}
	editWorkers := vtui.NewEdit(0, 0, 10, strconv.Itoa(workerDefault))
	lblWorkers := vtui.NewLabel(0, 0, Msg("ApplyCommand.Workers"), editWorkers)
	chkUnlimited := vtui.NewCheckbox(0, 0, Msg("ApplyCommand.Unlimited"), false)
	if AppConfig.ApplyCommandParallelism == 0 {
		chkUnlimited.State = 1
	}

	btnRun := vtui.NewButton(0, 0, Msg("ApplyCommand.Run"))
	btnRun.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, Msg("vtui.Cancel"))
	items := []vtui.UIElement{lblCommand, editCommand, txtTargets, lblMode, comboMode, lblWorkers, editWorkers, chkUnlimited, btnRun, btnCancel}
	for _, item := range items {
		dlg.AddItem(item)
	}

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)
	vbox.Add(lblCommand, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(editCommand, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(txtTargets, vtui.Margins{Top: 1}, vtui.AlignLeft)
	rowMode := vtui.NewHBoxLayout(0, 0, width-4, 1)
	rowMode.Add(lblMode, vtui.Margins{Right: 1}, vtui.AlignLeft)
	rowMode.Add(comboMode, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(rowMode, vtui.Margins{Top: 1}, vtui.AlignFill)
	rowWorkers := vtui.NewHBoxLayout(0, 0, width-4, 1)
	rowWorkers.Add(lblWorkers, vtui.Margins{Right: 1}, vtui.AlignLeft)
	rowWorkers.Add(editWorkers, vtui.Margins{Right: 2}, vtui.AlignFill)
	rowWorkers.Add(chkUnlimited, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(rowWorkers, vtui.Margins{Top: 1}, vtui.AlignFill)
	buttons := vtui.NewHBoxLayout(0, 0, width-4, 1)
	buttons.HorizontalAlign = vtui.AlignCenter
	buttons.Spacing = 2
	buttons.Add(btnRun, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(buttons, vtui.Margins{Top: 2}, vtui.AlignFill)
	vbox.Apply()

	updateWorkers := func() {
		parallel := comboMode.Menu.SelectPos == int(ApplyCommandParallel)
		lblWorkers.SetDisabled(!parallel)
		chkUnlimited.SetDisabled(!parallel)
		editWorkers.SetDisabled(!parallel || chkUnlimited.State == 1)
	}
	comboMode.Menu.OnAction = func(index int) {
		if index < 0 || index >= len(modes) {
			return
		}
		comboMode.Menu.SetSelectPos(index)
		comboMode.Edit.SetText(modes[index])
		updateWorkers()
	}
	chkUnlimited.OnChange = func(int) { updateWorkers() }
	updateWorkers()

	btnCancel.OnClick = func() { dlg.Close() }
	btnRun.OnClick = func() {
		raw := editCommand.GetText()
		executable := strings.TrimLeftFunc(raw, unicode.IsSpace)
		if strings.TrimSpace(executable) == "" {
			vtui.ShowMessageOn(dlg, Msg("ApplyCommand.Title"), Msg("ApplyCommand.InvalidCommand"), []string{Msg("vtui.Ok")})
			return
		}
		silent := false
		if strings.HasPrefix(executable, "@") {
			silent = true
			executable = strings.TrimLeftFunc(strings.TrimPrefix(executable, "@"), unicode.IsSpace)
		}
		if strings.TrimSpace(executable) == "" {
			vtui.ShowMessageOn(dlg, Msg("ApplyCommand.Title"), Msg("ApplyCommand.InvalidCommand"), []string{Msg("vtui.Ok")})
			return
		}
		compiled, err := CompileApplyCommand(executable)
		if err != nil {
			vtui.ShowMessageOn(dlg, Msg("ApplyCommand.Title"), fmt.Sprintf(Msg("ApplyCommand.InvalidTemplateFmt"), err), []string{Msg("vtui.Ok")})
			return
		}
		if session.info.Dialect == vfs.CommandDialectUnknown && compiled.Metadata().RequiresDialect {
			vtui.ShowMessageOn(dlg, Msg("ApplyCommand.Title"), Msg("ApplyCommand.UnknownDialect"), []string{Msg("vtui.Ok")})
			return
		}
		mode := ApplyCommandSequential
		switch comboMode.Menu.SelectPos {
		case 1:
			mode = ApplyCommandParallel
		case 2:
			mode = ApplyCommandQueued
		}
		workers := 1
		if mode == ApplyCommandParallel {
			if chkUnlimited.State == 1 {
				workers = 0
			} else {
				workers, err = strconv.Atoi(strings.TrimSpace(editWorkers.GetText()))
				if err != nil || workers <= 0 {
					vtui.ShowMessageOn(dlg, Msg("ApplyCommand.InvalidWorkersTitle"), Msg("ApplyCommand.InvalidWorkers"), []string{Msg("vtui.Ok")})
					return
				}
			}
		}
		firstContext := session.contextFor(session.targets[0])
		prompts, err := compiled.ResolvePrompts(firstContext)
		if err != nil {
			vtui.ShowMessageOn(dlg, Msg("ApplyCommand.Title"), fmt.Sprintf(Msg("ApplyCommand.InvalidTemplateFmt"), err), []string{Msg("vtui.Ok")})
			return
		}
		accept := func(values ApplyCommandPromptValues) {
			session.template = compiled
			session.values = values
			session.silent = silent
			session.mode = mode
			session.workers = workers
			if err := session.prepareExecution(); err != nil {
				vtui.ShowMessageOn(dlg, Msg("ApplyCommand.Title"), fmt.Sprintf(Msg("ApplyCommand.InvalidTemplateFmt"), err), []string{Msg("vtui.Ok")})
				return
			}
			lastApplyCommandTemplate = raw
			editCommand.AddHistory(raw)
			if mode == ApplyCommandParallel {
				AppConfig.ApplyCommandParallelism = workers
				RequestSaveConfig()
			}
			session.active.panel.SaveSelection()
			dlg.Close()
			launchApplyCommandSession(session)
		}
		if len(prompts) == 0 {
			accept(ApplyCommandPromptValues{})
			return
		}
		showApplyCommandPrompts(dlg, prompts, accept)
	}

	vtui.FrameManager.PushToFrameScreen(session.pf, dlg)
}

func showApplyCommandPrompts(anchor vtui.Frame, prompts []ApplyCommandResolvedPrompt, accepted func(ApplyCommandPromptValues)) {
	const pageSize = 10

	width := 70
	visibleRows := min(len(prompts), pageSize)
	// Keep the dialog usable on a conventional 25-line terminal. Templates
	// can contain any number of fields; additional fields are paged within
	// this same preflight dialog and retain their values between pages.
	height := 7 + visibleRows
	dlg := vtui.NewCenteredDialog(width, height, Msg("ApplyCommand.PromptTitle"))
	dlg.ShowClose = true
	dlg.SetHelp("ApplyCmd")
	contentX := dlg.X1 + 2
	contentRight := dlg.X2 - 2
	rowY := dlg.Y1 + 2
	labelWidth := 26
	editX := contentX + labelWidth + 1
	editWidth := max(1, contentRight-editX+1)

	labels := make([]*vtui.Text, len(prompts))
	edits := make([]*vtui.Edit, len(prompts))
	fieldLocked := make([]bool, len(prompts))
	for i, prompt := range prompts {
		title := prompt.Title
		if title == "" {
			title = fmt.Sprintf(Msg("ApplyCommand.ValueFmt"), i+1)
		}
		title = escapeAmpersand(vtui.TruncateMiddle(title, labelWidth))
		y := rowY + i%pageSize
		edit := vtui.NewEdit(editX, y, editWidth, prompt.Initial)
		if prompt.History != "" {
			edit.HistoryID = prompt.History
			edit.ShowHistoryButton = true
			edit.DeduplicateHistory = true
			if vtui.GlobalHistoryProvider != nil {
				edit.History = vtui.GlobalHistoryProvider.LoadHistory(prompt.History)
			}
		}
		label := vtui.NewLabel(contentX, y, title, edit)
		label.SetPosition(contentX, y, editX-2, y)
		dlg.AddItem(label)
		dlg.AddItem(edit)
		label.SetDisabled(true)
		edit.SetDisabled(true)
		label.Lock()
		edit.Lock()
		fieldLocked[i] = true
		labels[i] = label
		edits[i] = edit
	}

	pageInfo := vtui.NewText(contentX, dlg.Y2-3, "", 0)
	pageInfo.SetPosition(contentX, dlg.Y2-3, contentRight, dlg.Y2-3)
	dlg.AddItem(pageInfo)
	btnBack := vtui.NewButton(0, 0, Msg("ApplyCommand.PromptBack"))
	btnNext := vtui.NewButton(0, 0, Msg("ApplyCommand.PromptNext"))
	btnOK := vtui.NewButton(0, 0, Msg("vtui.Ok"))
	btnCancel := vtui.NewButton(0, 0, Msg("vtui.Cancel"))
	dlg.AddItem(btnBack)
	dlg.AddItem(btnNext)
	dlg.AddItem(btnOK)
	dlg.AddItem(btnCancel)
	buttons := vtui.NewHBoxLayout(contentX, dlg.Y2-2, width-4, 1)
	buttons.HorizontalAlign = vtui.AlignCenter
	buttons.Spacing = 2
	buttons.Add(btnBack, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(btnNext, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(btnOK, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	buttons.Apply()

	page := 0
	renderPage := func(redraw bool) {
		start := page * pageSize
		end := min(start+pageSize, len(prompts))
		for i := range edits {
			visible := i >= start && i < end
			if visible && fieldLocked[i] {
				labels[i].Unlock()
				edits[i].Unlock()
				fieldLocked[i] = false
			} else if !visible && !fieldLocked[i] {
				labels[i].Lock()
				edits[i].Lock()
				fieldLocked[i] = true
			}
			labels[i].SetVisible(visible)
			edits[i].SetVisible(visible)
			labels[i].SetDisabled(!visible)
			edits[i].SetDisabled(!visible)
		}
		pageInfo.SetText(fmt.Sprintf(Msg("ApplyCommand.PromptPageFmt"), start+1, end, len(prompts)))
		btnBack.SetDisabled(page == 0)
		btnNext.SetDisabled(end == len(prompts))
		btnOK.SetDisabled(end != len(prompts))
		btnNext.IsDefault = end != len(prompts)
		btnOK.IsDefault = end == len(prompts)
		if start < len(edits) {
			dlg.SetFocusedItem(edits[start])
		}
		if redraw {
			vtui.FrameManager.Redraw()
		}
	}
	btnBack.OnClick = func() {
		if page > 0 {
			page--
			renderPage(true)
		}
	}
	btnNext.OnClick = func() {
		if (page+1)*pageSize < len(prompts) {
			page++
			renderPage(true)
		}
	}
	btnCancel.OnClick = func() { dlg.Close() }
	btnOK.OnClick = func() {
		values := make(ApplyCommandPromptValues, len(prompts))
		for i, prompt := range prompts {
			value := edits[i].GetText()
			values[prompt.Index] = value
			if edits[i].HistoryID != "" {
				edits[i].AddHistory(value)
			}
		}
		dlg.Close()
		accepted(values)
	}
	renderPage(false)
	vtui.FrameManager.PushToFrameScreen(anchor, dlg)
}

func launchApplyCommandSession(session *applyCommandSession) {
	items := session.items
	workers := effectiveApplyCommandWorkers(session.mode, session.workers, len(items), session.info.MaxParallel)
	model := newApplyBatchViewModel(len(items))
	observe := func(event applyBatchEvent) {
		model.Observe(session.mode == ApplyCommandParallel, event)
		if event.Kind == applyBatchItemFinished {
			session.postItemFinished(event.Result)
		}
	}
	request := applyBatchRequest{
		Dir: session.active.dir, Items: items, Runner: session.runner, Parallelism: workers,
		Expand: session.expandItem, Observe: observe,
	}
	if session.mode == ApplyCommandQueued {
		session.enqueue(request, model)
		return
	}

	runCtx, cancel := context.WithCancel(context.Background())
	unregister := registerForegroundApplyCommand(cancel)
	showApplyOutputDialog(session.pf, model, func() {
		cancel()
	})
	vtui.RunAsync(func(ctx *vtui.TaskContext) {
		defer unregister()
		defer cancel()
		defer session.releaseCapturedVFSes()
		result := runApplyCommandBatch(runCtx, request)
		ctx.RunOnUI(func() {
			model.Finish(result)
			session.refreshCapturedPanels()
		})
	})
}

func (s *applyCommandSession) prepareExecution() error {
	items, err := s.batchItems()
	if err != nil {
		return err
	}
	activeClone := s.active.panelVFS.Clone()
	if activeClone == nil {
		return fmt.Errorf("apply command: active file system could not be captured")
	}
	passiveClone := vfs.VFS(nil)
	if s.passive.panelVFS != nil {
		passiveClone = s.passive.panelVFS.Clone()
		if passiveClone == nil {
			if !sameVFSInstance(activeClone, s.active.panelVFS) {
				_ = activeClone.Close()
			}
			return fmt.Errorf("apply command: passive file system could not be captured")
		}
	}
	runner, info, ok := resolveApplyCommandRunner(activeClone)
	if !ok {
		if !sameVFSInstance(activeClone, s.active.panelVFS) {
			_ = activeClone.Close()
		}
		if passiveClone != nil && !sameVFSInstance(passiveClone, s.passive.panelVFS) {
			_ = passiveClone.Close()
		}
		return fmt.Errorf("%s", Msg("ApplyCommand.Unsupported"))
	}
	if info.Dialect == vfs.CommandDialectUnknown && s.template.Metadata().RequiresDialect {
		if !sameVFSInstance(activeClone, s.active.panelVFS) {
			_ = activeClone.Close()
		}
		if passiveClone != nil && !sameVFSInstance(passiveClone, s.passive.panelVFS) {
			_ = passiveClone.Close()
		}
		return fmt.Errorf("%s", Msg("ApplyCommand.UnknownDialect"))
	}
	s.active.vfs = activeClone
	s.passive.vfs = passiveClone
	s.runner, s.info, s.items = runner, info, items
	if !sameVFSInstance(activeClone, s.active.panelVFS) {
		s.ownedVFS = append(s.ownedVFS, activeClone)
	}
	if passiveClone != nil && !sameVFSInstance(passiveClone, s.passive.panelVFS) {
		s.ownedVFS = append(s.ownedVFS, passiveClone)
	}
	return nil
}

func (s *applyCommandSession) releaseCapturedVFSes() {
	if s == nil {
		return
	}
	s.releaseVFS.Do(func() {
		for i := len(s.ownedVFS) - 1; i >= 0; i-- {
			_ = s.ownedVFS[i].Close()
		}
	})
}

func (s *applyCommandSession) batchItems() ([]applyBatchItem, error) {
	first, err := s.template.Expand(s.contextFor(s.targets[0]), s.values)
	if err != nil {
		return nil, err
	}
	if first.Cardinality == ApplyCommandOnce {
		return []applyBatchItem{{Name: s.targets[0], AffectedNames: append([]string(nil), s.targets...)}}, nil
	}
	items := make([]applyBatchItem, len(s.targets))
	for i, name := range s.targets {
		items[i] = applyBatchItem{Name: name, AffectedNames: []string{name}}
	}
	return items, nil
}

func (s *applyCommandSession) expandItem(ctx context.Context, _ int, item applyBatchItem) (applyExpandedCommand, error) {
	if err := ctx.Err(); err != nil {
		return applyExpandedCommand{}, err
	}
	expansion, err := s.template.Expand(s.contextFor(item.Name), s.values)
	if err != nil {
		return applyExpandedCommand{}, err
	}
	paths, release, err := materializeApplyCommandResources(ctx, s.active.vfs, s.active.dir, s.info.Dialect, expansion.Resources)
	if err != nil {
		return applyExpandedCommand{}, err
	}
	command, err := expansion.Render(paths)
	if err != nil {
		if release != nil {
			release(false)
		}
		return applyExpandedCommand{}, err
	}
	return applyExpandedCommand{Command: command, Silent: s.silent, Cleanup: release}, nil
}

func effectiveApplyCommandWorkers(mode ApplyCommandMode, configured, count, providerCap int) int {
	if mode != ApplyCommandParallel {
		return 1
	}
	workers := configured
	if workers <= 0 || workers > count {
		workers = count
	}
	if providerCap > 0 && workers > providerCap {
		workers = providerCap
	}
	if workers < 1 {
		workers = 1
	}
	return workers
}

func (s *applyCommandSession) postItemFinished(result applyBatchItemResult) {
	if !s.explicit || len(result.AffectedNames) == 0 {
		return
	}
	clear := func() {
		if s.pf == nil || s.active.panel == nil {
			return
		}
		s.pf.ptyMutex.Lock()
		closed := s.pf.closed
		s.pf.ptyMutex.Unlock()
		if closed {
			return
		}
		for _, name := range result.AffectedNames {
			if token, ok := s.tokens[name]; ok {
				s.active.panel.clearSelectionIfUnchanged(token)
			}
		}
		if vtui.FrameManager != nil {
			vtui.FrameManager.Redraw()
		}
	}
	if vtui.FrameManager != nil {
		vtui.FrameManager.PostTask(clear)
	} else {
		clear()
	}
}

func (s *applyCommandSession) refreshCapturedPanels() {
	if s == nil || s.pf == nil {
		return
	}
	s.pf.ptyMutex.Lock()
	closed := s.pf.closed
	s.pf.ptyMutex.Unlock()
	if closed {
		return
	}
	seen := make(map[*FileSystemPanel]bool)
	for _, capture := range []applyPanelCapture{s.active, s.passive} {
		if capture.panel == nil || seen[capture.panel] || !sameVFSInstance(capture.panel.vfs, capture.panelVFS) || capture.panel.vfs.GetPath() != capture.dir {
			continue
		}
		seen[capture.panel] = true
		capture.panel.ReadDirectory()
	}
}

func (s *applyCommandSession) enqueue(request applyBatchRequest, model *applyBatchViewModel) {
	preconditions := s.queuePreconditions()
	task := &QueueTask{
		Type: Msg("ApplyCommand.QueueType"), Desc: fmt.Sprintf(Msg("ApplyCommand.QueueDescriptionFmt"), len(s.targets)),
		Preconditions: preconditions, ResKeys: []string{getResourceKey(s.active.panelVFS)},
	}
	task.Run = func(ctx context.Context, reporter TaskReporter, _ vtui.Frame) error {
		defer s.releaseCapturedVFSes()
		originalObserve := request.Observe
		request.Observe = func(event applyBatchEvent) {
			originalObserve(event)
			if event.Kind == applyBatchItemFinished {
				completed := event.Index + 1
				pct := completed * 100 / len(request.Items)
				reporter.UpdateTransfer(Msg("ApplyCommand.QueueType"), event.Name, pct, fmt.Sprintf("%d/%d", completed, len(request.Items)), pct, "")
			}
		}
		result := runApplyCommandBatch(ctx, request)
		model.Finish(result)
		if err := ctx.Err(); err != nil {
			return err
		}
		if result.Failed > 0 {
			return fmt.Errorf(Msg("ApplyCommand.QueueFailedFmt"), result.Failed)
		}
		return nil
	}
	task.OpenDetails = func(anchor vtui.Frame) {
		showApplyOutputDialog(anchor, model, func() { GlobalQueueManager.Cancel(task.ID) })
	}
	task.Finalize = s.releaseCapturedVFSes
	task.OnComplete = func() {
		s.releaseCapturedVFSes()
		if !model.IsDone() {
			task.mu.Lock()
			state, taskErr := task.State, task.ErrorMsg
			task.mu.Unlock()
			fallback := applyBatchResult{Items: make([]applyBatchItemResult, len(request.Items)), NotStarted: len(request.Items)}
			if state == "Cancelled" {
				model.transcript.Add(Msg("ApplyCommand.ResultCancelled"))
			} else {
				if taskErr != nil {
					model.transcript.Add(fmt.Sprintf(Msg("ApplyCommand.ResultFailedFmt"), taskErr))
				}
			}
			model.Finish(fallback)
		}
		s.refreshCapturedPanels()
		showToast(Msg("ApplyCommand.StatusFinishedToast"), 3*time.Second)
	}
	GlobalQueueManager.Enqueue(task)
	showToast(Msg("ApplyCommand.QueuedToast"), 3*time.Second)
}

func (s *applyCommandSession) queuePreconditions() []OpPrecondition {
	if s.active.panel == nil || s.active.vfs == nil {
		return nil
	}
	wanted := make(map[string]struct{}, len(s.targets))
	for _, name := range s.targets {
		wanted[name] = struct{}{}
	}
	_, local := s.active.panelVFS.(*vfs.OSVFS)
	conditions := make([]OpPrecondition, 0, len(wanted))
	for _, panelEntry := range s.active.panel.entries {
		if _, ok := wanted[panelEntry.Name]; !ok {
			continue
		}
		path := s.active.vfs.Join(s.active.dir, panelEntry.Name)
		entry := panelEntry.VFSItem
		if panelEntry.IsSymlink {
			// Remote Stat would turn Run into an unbounded UI-thread network
			// operation. Omit that precondition; local Stat is cheap and makes
			// its baseline match the queue's follow-symlink comparison.
			if !local {
				continue
			}
			statEntry, err := s.active.vfs.Stat(context.Background(), path)
			if err != nil {
				continue
			}
			entry = statEntry
		}
		conditions = append(conditions, OpPrecondition{
			Vfs: s.active.vfs, Path: path,
			MTime: entry.MTime, Size: entry.Size, IsDir: entry.IsDir,
		})
	}
	return conditions
}
