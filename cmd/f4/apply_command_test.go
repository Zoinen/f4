package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type applyTestHistoryProvider struct{ values map[string][]string }

type unavailableApplyCommandRunner struct{ *vfs.OSVFS }

type invalidDialectApplyCommandRunner struct{ *vfs.OSVFS }

type panicPathApplyVFS struct {
	*vfs.OSVFS
	panicOnPath bool
}

func (v *panicPathApplyVFS) GetPath() string {
	if v.panicOnPath {
		panic("closed workspace VFS was touched")
	}
	return v.OSVFS.GetPath()
}

func (r *unavailableApplyCommandRunner) RunCommand(context.Context, string, string, func(string)) (int, error) {
	return 0, nil
}
func (r *unavailableApplyCommandRunner) CommandRunnerAvailable() bool { return false }

func (r *invalidDialectApplyCommandRunner) RunCommand(context.Context, string, string, func(string)) (int, error) {
	return 0, nil
}

func (r *invalidDialectApplyCommandRunner) CommandRunnerInfo() vfs.CommandRunnerInfo {
	return vfs.CommandRunnerInfo{Dialect: vfs.CommandDialect(99), MaxParallel: 8}
}

func (p *applyTestHistoryProvider) LoadHistory(id string) []string {
	return append([]string(nil), p.values[id]...)
}

func (p *applyTestHistoryProvider) SaveHistory(id string, history []string) {
	p.values[id] = append([]string(nil), history...)
}

func TestApplyCommandActionRegistered(t *testing.T) {
	action, ok := GetAction("File.ApplyCommand")
	if !ok {
		t.Fatal("File.ApplyCommand is not registered")
	}
	if action.Area != "Shell" || action.MenuPath != "Files" || action.LabelKey != "Action.File.ApplyCommand" || len(action.DefaultKeys) != 1 || action.DefaultKeys[0] != "CtrlG" {
		t.Fatalf("action = %+v", action)
	}
}

func TestApplyCommandActionUsesActivePanelWorkspace(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	pfA := setupMockPanelsFrame(t)
	pfB := setupMockPanelsFrame(t)
	defer pfA.Close()
	defer pfB.Close()
	pfA.ResizeConsole(80, 25)
	pfB.ResizeConsole(80, 25)
	panelA := pfA.getActivePanel()
	panelB := pfB.getActivePanel()
	panelA.vfs = vfs.NewNullVFS(0)
	panelB.vfs = vfs.NewOSVFS(t.TempDir())
	panelB.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "active.txt"}},
	}
	panelB.SetCursorIndex(1)

	vtui.FrameManager.Push(pfA)
	vtui.FrameManager.AddScreen(pfB)
	if got := findPanelsFrame(); got != pfB {
		t.Fatalf("active panels frame = %p, want workspace B %p", got, pfB)
	}
	if !panelCanApplyCommand() {
		t.Fatal("Apply visibility was taken from unsupported background workspace A")
	}

	panelA.vfs = vfs.NewOSVFS(t.TempDir())
	panelA.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "background.txt"}},
	}
	panelA.SetCursorIndex(1)
	action, ok := GetAction("File.ApplyCommand")
	if !ok || !action.Handler() {
		t.Fatal("Apply action was not dispatched")
	}
	if got := findPanelsFrame(); got != pfB {
		t.Fatalf("Apply switched to background workspace A; active frame = %p", got)
	}
	top, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok || top.GetTitle() != Msg("ApplyCommand.Title") {
		t.Fatalf("active workspace top frame = %T %q", vtui.FrameManager.GetTopFrame(), vtui.FrameManager.GetTopFrame().GetTitle())
	}
	top.Close()
	vtui.FrameManager.RemoveFrame(top)
}

func TestResolveApplyCommandRunnerHonorsNegotiatedAvailability(t *testing.T) {
	runner := &unavailableApplyCommandRunner{OSVFS: vfs.NewOSVFS(t.TempDir())}
	if _, _, ok := resolveApplyCommandRunner(runner); ok {
		t.Fatal("unavailable command provider was accepted")
	}
}

func TestResolveApplyCommandRunnerNormalizesInvalidDialect(t *testing.T) {
	runner := &invalidDialectApplyCommandRunner{OSVFS: vfs.NewOSVFS(t.TempDir())}
	_, info, ok := resolveApplyCommandRunner(runner)
	if !ok {
		t.Fatal("command provider was unexpectedly rejected")
	}
	if info.Dialect != vfs.CommandDialectUnknown {
		t.Fatalf("dialect = %d, want Unknown", info.Dialect)
	}
}

func TestEffectiveApplyCommandWorkers(t *testing.T) {
	tests := []struct {
		mode                        ApplyCommandMode
		configured, count, provider int
		want                        int
	}{
		{ApplyCommandSequential, runtime.NumCPU(), 20, 0, 1},
		{ApplyCommandQueued, 20, 20, 0, 1},
		{ApplyCommandParallel, 3, 20, 0, 3},
		{ApplyCommandParallel, 0, 7, 0, 7},
		{ApplyCommandParallel, 99, 7, 4, 4},
	}
	for _, tc := range tests {
		if got := effectiveApplyCommandWorkers(tc.mode, tc.configured, tc.count, tc.provider); got != tc.want {
			t.Errorf("effective(%d,%d,%d,%d) = %d, want %d", tc.mode, tc.configured, tc.count, tc.provider, got, tc.want)
		}
	}
}

func TestApplyQueuePreconditionUsesStatSemanticsForSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("target contents"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(filepath.Base(target), link); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	fs := vfs.NewOSVFS(dir)
	linkInfo, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	panel := &FileSystemPanel{
		vfs: fs,
		entries: []*fileEntry{{VFSItem: vfs.VFSItem{
			Name: linkInfo.Name(), Size: linkInfo.Size(), MTime: linkInfo.ModTime(), IsSymlink: true,
		}}},
	}
	session := &applyCommandSession{
		active:  applyPanelCapture{panel: panel, panelVFS: fs, vfs: fs, dir: dir},
		targets: []string{filepath.Base(link)},
	}
	conditions := session.queuePreconditions()
	if len(conditions) != 1 {
		t.Fatalf("preconditions = %d, want 1", len(conditions))
	}
	want, err := fs.Stat(context.Background(), link)
	if err != nil {
		t.Fatal(err)
	}
	got := conditions[0]
	if got.Size != want.Size || got.MTime != want.MTime || got.IsDir != want.IsDir {
		t.Fatalf("precondition = %+v, want Stat metadata %+v", got, want)
	}
}

func TestApplySelectionTokenDoesNotClearLaterRemark(t *testing.T) {
	pf := seedPanelForRestore(t, []string{"one.txt"})
	panel := pf.getActivePanel()
	panel.SetItemSelected(1, true)
	token, ok := panel.captureSelectionToken("one.txt")
	if !ok {
		t.Fatal("selection token not captured")
	}
	panel.SetItemSelected(1, false)
	panel.SetItemSelected(1, true)
	if panel.clearSelectionIfUnchanged(token) {
		t.Fatal("stale Apply completion cleared a later user selection")
	}
	if !panel.IsNameSelected("one.txt") {
		t.Fatal("later selection was lost")
	}
}

func TestApplySelectionTokenClearsUnchangedMark(t *testing.T) {
	pf := seedPanelForRestore(t, []string{"one.txt"})
	panel := pf.getActivePanel()
	panel.SetItemSelected(1, true)
	token, ok := panel.captureSelectionToken("one.txt")
	if !ok || !panel.clearSelectionIfUnchanged(token) || panel.IsNameSelected("one.txt") {
		t.Fatalf("unchanged mark was not cleared: token=%+v ok=%v", token, ok)
	}
}

func TestApplySelectionTokenDoesNotCrossVFSIdentity(t *testing.T) {
	pf := seedPanelForRestore(t, []string{"one.txt"})
	panel := pf.getActivePanel()
	dir := t.TempDir()
	panel.vfs = vfs.NewOSVFS(dir)
	panel.SetItemSelected(1, true)
	token, ok := panel.captureSelectionToken("one.txt")
	if !ok {
		t.Fatal("selection token not captured")
	}
	panel.vfs = vfs.NewOSVFS(dir)
	if panel.clearSelectionIfUnchanged(token) || !panel.IsNameSelected("one.txt") {
		t.Fatal("completion from the prior VFS cleared the new panel mark")
	}
}

func TestApplyFinalRefreshPreservesCtrlMSelection(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"one.txt", "two.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	pf := seedPanelForRestore(t, nil)
	panel := pf.getActivePanel()
	panel.vfs = vfs.NewOSVFS(dir)
	panel.ReadDirectory()
	waitForLoad(t, panel)
	if !panel.SetSelectedByName("one.txt", true) || !panel.SetSelectedByName("two.txt", true) {
		t.Fatalf("test files missing after initial load: %+v", panel.entries)
	}
	panel.SaveSelection()
	panel.SetSelectedByName("one.txt", false)
	panel.SetSelectedByName("two.txt", false)

	session := &applyCommandSession{
		pf: pf,
		active: applyPanelCapture{
			panel: panel, panelVFS: panel.vfs, vfs: panel.vfs, dir: dir,
		},
	}
	session.refreshCapturedPanels()
	waitForLoad(t, panel)
	if got := collectSelected(panel); len(got) != 0 {
		t.Fatalf("final refresh resurrected processed marks early: %v", got)
	}

	panel.RestoreSelection()
	if got := collectSelected(panel); len(got) != 2 || got[0] != "one.txt" || got[1] != "two.txt" {
		t.Fatalf("Ctrl+M after Apply refresh selected %v, want [one.txt two.txt]", got)
	}
}

func TestApplyCompletionDoesNotTouchClosedWorkspace(t *testing.T) {
	pf := seedPanelForRestore(t, []string{"one.txt"})
	panel := pf.getActivePanel()
	closedVFS := &panicPathApplyVFS{OSVFS: vfs.NewOSVFS(t.TempDir())}
	panel.vfs = closedVFS
	panel.SetItemSelected(1, true)
	token, ok := panel.captureSelectionToken("one.txt")
	if !ok {
		t.Fatal("selection token not captured")
	}
	closedVFS.panicOnPath = true
	pf.ptyMutex.Lock()
	pf.closed = true
	pf.ptyMutex.Unlock()
	session := &applyCommandSession{
		pf: pf, explicit: true, tokens: map[string]panelSelectionToken{"one.txt": token},
		active: applyPanelCapture{panel: panel, panelVFS: closedVFS, vfs: closedVFS, dir: "/captured"},
	}
	session.postItemFinished(applyBatchItemResult{AffectedNames: []string{"one.txt"}})
	drained := make(chan struct{})
	vtui.FrameManager.PostTask(func() { close(drained) })
	deadline := time.After(time.Second)
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-drained:
			if !panel.IsNameSelected("one.txt") {
				t.Fatal("completion cleared selection in a closed workspace")
			}
			session.refreshCapturedPanels()
			return
		case <-deadline:
			t.Fatal("timed out draining completion UI task")
		}
	}
}

func TestCancelAllForegroundApplyCommands(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	unregister := registerForegroundApplyCommand(cancel)
	defer unregister()
	cancelAllForegroundApplyCommands()
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("foreground context = %v", ctx.Err())
	}
}

func TestApplyOutputDialogCanCloseWhileForegroundBatchRuns(t *testing.T) {
	model := newApplyBatchViewModel(1)
	dlg := showApplyOutputDialog(nil, model, nil)
	defer vtui.FrameManager.RemoveFrame(dlg)

	model.mu.Lock()
	var view *applyOutputView
	for candidate := range model.views {
		view = candidate
		break
	}
	model.mu.Unlock()
	if view == nil || view.btnClose.IsDisabled() {
		t.Fatal("foreground Close button is disabled while running")
	}
	if !dlg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE}) || !dlg.IsDone() {
		t.Fatal("foreground output dialog did not close while the batch was running")
	}
	if model.IsDone() {
		t.Fatal("closing the output dialog completed the running batch")
	}
	model.mu.Lock()
	_, stillObserved := model.views[view]
	model.mu.Unlock()
	if stillObserved {
		t.Fatal("closed output dialog remained registered for refresh")
	}
}

func TestApplyQueueDetailsConsumesCtrlWAndClosesOnlyDialog(t *testing.T) {
	model := newApplyBatchViewModel(1)
	dlg := showApplyOutputDialog(nil, model, nil)
	defer vtui.FrameManager.RemoveFrame(dlg)
	if !dlg.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_W,
		ControlKeyState: vtinput.LeftCtrlPressed,
	}) {
		t.Fatal("queue Details did not consume Ctrl+W")
	}
	if !dlg.IsDone() {
		t.Fatal("queue Details did not close its own dialog on Ctrl+W")
	}
}

func TestApplyOutputDialogExpandsTranscript(t *testing.T) {
	model := newApplyBatchViewModel(1)
	for i := 0; i < 30; i++ {
		model.transcript.Add(fmt.Sprintf("line %d", i))
	}
	dlg := showApplyOutputDialog(nil, model, nil)
	defer vtui.FrameManager.RemoveFrame(dlg)
	if !dlg.ShowZoom {
		t.Fatal("Apply output dialog has no expand control")
	}

	model.mu.Lock()
	var view *applyOutputView
	for candidate := range model.views {
		view = candidate
		break
	}
	model.mu.Unlock()
	if view == nil {
		t.Fatal("Apply output view not registered")
	}
	if !view.output.ShowScrollBar || view.output.ScrollBar == nil {
		t.Fatal("Apply output transcript has no scrollbar")
	}
	if view.output.ItemCount <= view.output.ViewHeight {
		t.Fatalf("test transcript does not overflow: items=%d height=%d", view.output.ItemCount, view.output.ViewHeight)
	}
	if view.output.ColorTextIdx != ColViewerText || view.output.ColorSelectedTextIdx != ColViewerStatus {
		t.Fatalf("transcript colors = %d/%d, want themed Viewer colors %d/%d",
			view.output.ColorTextIdx, view.output.ColorSelectedTextIdx, ColViewerText, ColViewerStatus)
	}
	if view.output.ScrollBar.ColorIdx != ColViewerScrollbar {
		t.Fatalf("transcript scrollbar color = %d, want themed Viewer scrollbar %d", view.output.ScrollBar.ColorIdx, ColViewerScrollbar)
	}
	_, _, oldX2, oldY2 := view.output.GetPosition()
	dx1, dy1, dx2, dy2 := dlg.GetPosition()
	dlg.ChangeSize(dx2-dx1+11, dy2-dy1+6)
	_, _, newX2, newY2 := view.output.GetPosition()
	if newX2 <= oldX2 || newY2 <= oldY2 {
		t.Fatalf("transcript did not grow: delta = %dx%d", newX2-oldX2, newY2-oldY2)
	}
}

func TestApplyTranscriptCanBeForwardedToEditor(t *testing.T) {
	model := newApplyBatchViewModel(1)
	model.transcript.Add("first line")
	model.transcript.Add("second line")
	editor := newApplyTranscriptEditor(model, 80, 25)
	if got := editor.pt.String(); got != "first line\nsecond line\n" {
		t.Fatalf("editor transcript = %q", got)
	}
	if editor.DisplayTitle != Msg("ApplyCommand.OutputEditorTitle") {
		t.Fatalf("editor title = %q", editor.DisplayTitle)
	}
}

func TestApplyCommandCtrlGOpensHistoryDialogAndPreservesCommandLine(t *testing.T) {
	pf := seedPanelForRestore(t, []string{"one.txt"})
	panel := pf.getActivePanel()
	panel.vfs = vfs.NewOSVFS(t.TempDir())
	panel.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}, {VFSItem: vfs.VFSItem{Name: "one.txt"}}}
	panel.SetCursorIndex(1)
	pf.cmdLine.Edit.SetText("keep this command")

	handled := pressKey(pf, &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_G,
		ControlKeyState: vtinput.LeftCtrlPressed,
	})
	if !handled {
		t.Fatal("Ctrl+G was not handled")
	}
	top, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok || top.GetTitle() != Msg("ApplyCommand.Title") {
		t.Fatalf("top frame = %T %q", vtui.FrameManager.GetTopFrame(), vtui.FrameManager.GetTopFrame().GetTitle())
	}
	foundHistory := false
	for _, child := range top.GetChildren() {
		if edit, ok := child.(*vtui.Edit); ok && edit.HistoryID == "ApplyCmd" && edit.ShowHistoryButton {
			foundHistory = true
		}
	}
	if !foundHistory {
		t.Fatal("ApplyCmd history editor not found")
	}
	if got := pf.cmdLine.Edit.GetText(); got != "keep this command" {
		t.Fatalf("command line changed to %q", got)
	}
	top.Close()
}

func TestApplyCommandPromptCancellationKeepsHistoryAndSelection(t *testing.T) {
	pf := seedPanelForRestore(t, []string{"one.txt"})
	panel := pf.getActivePanel()
	panel.vfs = vfs.NewOSVFS(t.TempDir())
	panel.SetItemSelected(1, true)

	oldProvider := vtui.GlobalHistoryProvider
	provider := &applyTestHistoryProvider{values: make(map[string][]string)}
	vtui.GlobalHistoryProvider = provider
	defer func() { vtui.GlobalHistoryProvider = oldProvider }()
	oldTemplate := lastApplyCommandTemplate
	lastApplyCommandTemplate = ""
	defer func() { lastApplyCommandTemplate = oldTemplate }()

	actionApplyCommand(pf)
	applyDialog, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok {
		t.Fatalf("top frame = %T, want Apply dialog", vtui.FrameManager.GetTopFrame())
	}
	var commandEdit *vtui.Edit
	var runButton *vtui.Button
	for _, child := range applyDialog.GetChildren() {
		switch child := child.(type) {
		case *vtui.Edit:
			if child.HistoryID == "ApplyCmd" {
				commandEdit = child
			}
		case *vtui.Button:
			if child.IsDefault {
				runButton = child
			}
		}
	}
	if commandEdit == nil || runButton == nil {
		t.Fatal("Apply dialog command editor or Run button not found")
	}
	commandEdit.SetText("echo !?Value?default!")
	runButton.OnClick()

	promptDialog, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok || promptDialog == applyDialog {
		t.Fatalf("top frame = %T, want prompt dialog", vtui.FrameManager.GetTopFrame())
	}
	var cancelButton *vtui.Button
	for _, child := range promptDialog.GetChildren() {
		if button, ok := child.(*vtui.Button); ok && !button.IsDefault {
			cancelButton = button
			break
		}
	}
	if cancelButton == nil {
		t.Fatal("prompt Cancel button not found")
	}
	cancelButton.OnClick()
	vtui.FrameManager.RemoveFrame(promptDialog)

	if vtui.FrameManager.GetTopFrame() != applyDialog {
		t.Fatal("prompt cancellation did not return to Apply dialog")
	}
	if len(provider.values["ApplyCmd"]) != 0 || lastApplyCommandTemplate != "" {
		t.Fatalf("cancelled preflight changed history=%v template=%q", provider.values["ApplyCmd"], lastApplyCommandTemplate)
	}
	if !panel.IsNameSelected("one.txt") || panel.entries[1].PrevSelected {
		t.Fatal("cancelled preflight changed or saved the panel selection")
	}
	applyDialog.Close()
}

func TestApplyCommandPromptDialogPagesEveryField(t *testing.T) {
	prompts := make([]ApplyCommandResolvedPrompt, 12)
	for i := range prompts {
		prompts[i] = ApplyCommandResolvedPrompt{Index: i, Title: fmt.Sprintf("Field %d", i+1), Initial: fmt.Sprintf("default-%d", i+1)}
	}
	var accepted ApplyCommandPromptValues
	showApplyCommandPrompts(nil, prompts, func(values ApplyCommandPromptValues) { accepted = values })
	dlg, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok {
		t.Fatalf("top frame = %T, want prompt dialog", vtui.FrameManager.GetTopFrame())
	}
	defer vtui.FrameManager.RemoveFrame(dlg)

	var edits []*vtui.Edit
	buttons := make(map[string]*vtui.Button)
	for _, child := range dlg.GetChildren() {
		switch child := child.(type) {
		case *vtui.Edit:
			edits = append(edits, child)
		case *vtui.Button:
			buttons[child.GetCaption()] = child
		}
	}
	if len(edits) != len(prompts) {
		t.Fatalf("edit count = %d, want %d", len(edits), len(prompts))
	}
	cleanCaption := func(message string) string {
		clean, _, _ := vtui.ParseAmpersandString(message)
		return clean
	}
	next := buttons[cleanCaption(Msg("ApplyCommand.PromptNext"))]
	back := buttons[cleanCaption(Msg("ApplyCommand.PromptBack"))]
	okButton := buttons[cleanCaption(Msg("vtui.Ok"))]
	if next == nil || back == nil || okButton == nil {
		t.Fatalf("pagination buttons missing: %#v", buttons)
	}
	for i, edit := range edits {
		edit.SetText(fmt.Sprintf("answer-%d", i+1))
	}
	if !back.IsDisabled() || next.IsDisabled() || !next.IsDefault || !okButton.IsDisabled() {
		t.Fatal("first prompt page has incorrect navigation state")
	}
	enabled := 0
	for i, edit := range edits {
		if !edit.IsDisabled() {
			enabled++
		}
		if edit.IsVisible() != (i < 10) {
			t.Errorf("field %d initial visibility = %v", i+1, edit.IsVisible())
		}
	}
	if enabled != 10 {
		t.Fatalf("enabled fields on first page = %d, want 10", enabled)
	}

	next.OnClick()
	if back.IsDisabled() || !next.IsDisabled() || next.IsDefault || okButton.IsDisabled() || !okButton.IsDefault {
		t.Fatal("last prompt page has incorrect navigation state")
	}
	enabled = 0
	for i, edit := range edits {
		if !edit.IsDisabled() {
			enabled++
		}
		if edit.IsVisible() != (i >= 10) {
			t.Errorf("field %d last-page visibility = %v", i+1, edit.IsVisible())
		}
	}
	if enabled != 2 {
		t.Fatalf("enabled fields on last page = %d, want 2", enabled)
	}
	back.OnClick()
	for i, edit := range edits {
		if edit.IsVisible() != (i < 10) {
			t.Errorf("field %d visibility after Back = %v", i+1, edit.IsVisible())
		}
	}
	next.OnClick()
	okButton.OnClick()
	if len(accepted) != len(prompts) {
		t.Fatalf("accepted values = %d, want %d", len(accepted), len(prompts))
	}
	for i := range prompts {
		if got, want := accepted[i], fmt.Sprintf("answer-%d", i+1); got != want {
			t.Errorf("value %d = %q, want %q", i+1, got, want)
		}
	}
}
