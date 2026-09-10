package cmdline

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/unxed/f4/internal/dialog"
	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type ApplyBatchViewModel struct {
	mu             sync.Mutex
	Transcript     *applyTranscript
	total          int
	completed      int
	failed         int
	cancelled      int
	runningName    string
	done           bool
	cancelling     bool
	refreshPending bool
	views          map[*ApplyOutputView]struct{}
}

func NewApplyBatchViewModel(total int) *ApplyBatchViewModel {
	return &ApplyBatchViewModel{
		Transcript: newApplyTranscript(),
		total:      total,
		views:      make(map[*ApplyOutputView]struct{}),
	}
}

func (m *ApplyBatchViewModel) Observe(parallel bool, ev ApplyBatchEvent) {
	if m == nil {
		return
	}
	switch ev.Kind {
	case applyBatchItemStarted:
		m.mu.Lock()
		m.runningName = ev.Name
		m.mu.Unlock()
	case applyBatchCommandReady:
		if !ev.Silent {
			m.Transcript.Add(fmt.Sprintf("[%d/%d %s] $ %s", ev.Index+1, ev.Total, ev.Name, ev.Line))
		}
	case applyBatchOutput:
		if parallel {
			m.Transcript.Add(fmt.Sprintf("[%d/%d %s] %s", ev.Index+1, ev.Total, ev.Name, ev.Line))
		} else {
			m.Transcript.Add(ev.Line)
		}
	case ApplyBatchItemFinished:
		m.mu.Lock()
		m.completed++
		switch ev.Result.State {
		case applyItemFailed:
			m.failed++
		case applyItemCancelled:
			m.cancelled++
		}
		m.mu.Unlock()
		if ev.Result.State == applyItemFailed {
			m.Transcript.Add(fmt.Sprintf("[%d/%d %s] %s", ev.Index+1, ev.Total, ev.Name, applyResultSummary(ev.Result)))
		}
	}
	m.requestRefresh()
}

func applyResultSummary(result ApplyBatchItemResult) string {
	switch result.State {
	case applyItemSucceeded:
		return i18n.Msg("ApplyCommand.ResultSuccess")
	case applyItemCancelled:
		return i18n.Msg("ApplyCommand.ResultCancelled")
	case applyItemFailed:
		if result.Err != nil {
			return fmt.Sprintf(i18n.Msg("ApplyCommand.ResultFailedFmt"), result.Err)
		}
		return i18n.Msg("ApplyCommand.ResultFailed")
	default:
		return i18n.Msg("ApplyCommand.ResultPending")
	}
}

func (m *ApplyBatchViewModel) Finish(result ApplyBatchResult) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.done = true
	m.cancelling = false
	m.completed = result.Completed
	m.failed = result.Failed
	m.cancelled = result.Cancelled
	m.runningName = ""
	m.mu.Unlock()
	m.Transcript.Add("")
	m.Transcript.Add(fmt.Sprintf(i18n.Msg("ApplyCommand.SummaryFmt"), result.Succeeded, result.Failed, result.Cancelled, result.NotStarted))
	m.requestRefresh()
}

func (m *ApplyBatchViewModel) RequestCancel() {
	if m == nil {
		return
	}
	m.mu.Lock()
	if !m.done {
		m.cancelling = true
	}
	m.mu.Unlock()
	m.requestRefresh()
}

func (m *ApplyBatchViewModel) requestRefresh() {
	m.mu.Lock()
	if m.refreshPending {
		m.mu.Unlock()
		return
	}
	m.refreshPending = true
	m.mu.Unlock()
	// Read on the goroutine that starts this work, not inside it: the
	// work outlives the call, and reading the global from it races
	// anything that reassigns vtui.FrameManager meanwhile.
	frames := vtui.FrameManager
	time.AfterFunc(50*time.Millisecond, func() {
		if frames == nil {
			m.mu.Lock()
			m.refreshPending = false
			m.mu.Unlock()
			return
		}
		frames.PostTask(func() {
			m.mu.Lock()
			m.refreshPending = false
			views := make([]*ApplyOutputView, 0, len(m.views))
			for view := range m.views {
				views = append(views, view)
			}
			m.mu.Unlock()
			for _, view := range views {
				view.refresh()
			}
			frames.Redraw()
		})
	})
}

func (m *ApplyBatchViewModel) snapshotStatus() (status string, percent int, done bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.total > 0 {
		percent = m.completed * 100 / m.total
	}
	if m.done {
		status = fmt.Sprintf(i18n.Msg("ApplyCommand.StatusDoneFmt"), m.completed, m.total, m.failed, m.cancelled)
	} else if m.cancelling {
		status = i18n.Msg("ApplyCommand.StatusCancelling")
	} else {
		status = fmt.Sprintf(i18n.Msg("ApplyCommand.StatusRunningFmt"), m.completed, m.total, dialog.EscapeAmpersand(m.runningName))
	}
	return status, percent, m.done
}

func (m *ApplyBatchViewModel) IsDone() bool {
	if m == nil {
		return true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.done
}

type ApplyOutputView struct {
	dlg       *ApplyOutputDialog
	model     *ApplyBatchViewModel
	status    *vtui.Text
	progress  *vtui.ProgressBar
	output    *vtui.ListBox
	btnCancel *vtui.Button
	btnEditor *vtui.Button
	btnClose  *vtui.Button
}

type ApplyOutputDialog struct {
	*vtui.Window
}

func (d *ApplyOutputDialog) ProcessKey(event *vtinput.InputEvent) bool {
	if event != nil && event.KeyDown {
		ctrlW := event.VirtualKeyCode == vtinput.VK_W &&
			(event.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed)) != 0
		if ctrlW {
			d.Close()
			return true
		}
	}
	return d.Window.ProcessKey(event)
}

func (v *ApplyOutputView) refresh() {
	if v == nil || v.model == nil {
		return
	}
	status, pct, done := v.model.snapshotStatus()
	v.status.SetText(status)
	v.progress.SetPercent(pct)
	v.output.Items = v.model.Transcript.Snapshot()
	v.output.UpdateRows()
	if len(v.output.Items) > 0 {
		v.output.SetSelectPos(len(v.output.Items) - 1)
	}
	v.model.mu.Lock()
	cancelling := v.model.cancelling
	v.model.mu.Unlock()
	v.btnCancel.SetDisabled(done || cancelling)
	v.btnEditor.SetDisabled(len(v.output.Items) == 0)
	v.btnClose.SetDisabled(false)
}

func NewApplyTranscriptEditor(model *ApplyBatchViewModel, width, height int) *editor.EditorView {
	lines := model.Transcript.Snapshot()
	text := strings.Join(lines, "\n")
	if len(lines) > 0 {
		text += "\n"
	}
	editor := editor.NewEditorView(piecetable.New([]byte(text)), nil, "")
	editor.DisplayTitle = i18n.Msg("ApplyCommand.OutputEditorTitle")
	editor.ResizeConsole(width, height)
	return editor
}

// showApplyOutputDialog opens a live or completed transcript. Closing a live
// view only detaches the UI; the foreground or queued batch keeps running.
func ShowApplyOutputDialog(anchor vtui.Frame, model *ApplyBatchViewModel, cancel func()) *ApplyOutputDialog {
	const width, height = 86, 24
	dlg := &ApplyOutputDialog{Window: vtui.NewCenteredDialog(width, height, i18n.Msg("ApplyCommand.OutputTitle"))}
	dlg.ShowClose = true
	dlg.ShowZoom = true
	dlg.SetHelp("ApplyCmd")

	status := vtui.NewText(0, 0, "", 0)
	progress := vtui.NewProgressBar(0, 0, width-4)
	output := vtui.NewListBox(0, 0, width-4, height-9, nil)
	output.ShowScrollBar = true
	output.ColorTextIdx = theme.ColViewerText
	output.ColorSelectedTextIdx = theme.ColViewerStatus
	output.ColorItemSelectTextIdx = theme.ColViewerText
	output.ColorItemSelectCursorIdx = theme.ColViewerStatus
	if output.ScrollBar != nil {
		output.ScrollBar.ColorIdx = theme.ColViewerScrollbar
	}
	btnCancel := vtui.NewButton(0, 0, i18n.Msg("ApplyCommand.CancelTask"))
	btnEditor := vtui.NewButton(0, 0, i18n.Msg("ApplyCommand.SendToEditor"))
	btnClose := vtui.NewButton(0, 0, i18n.Msg("ApplyCommand.Close"))

	for _, item := range []vtui.UIElement{status, progress, output, btnCancel, btnEditor, btnClose} {
		dlg.AddItem(item)
	}
	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)
	vbox.Add(status, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(progress, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Add(output, vtui.Margins{Top: 1, Bottom: 1}, vtui.AlignFill)
	hbox := vtui.NewHBoxLayout(0, 0, width-4, 1)
	hbox.HorizontalAlign = vtui.AlignCenter
	hbox.Spacing = 2
	hbox.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	hbox.Add(btnEditor, vtui.Margins{}, vtui.AlignTop)
	hbox.Add(btnClose, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(hbox, vtui.Margins{}, vtui.AlignFill)
	vbox.Apply()
	status.SetGrowMode(vtui.GrowHiX)
	progress.SetGrowMode(vtui.GrowHiX)
	output.SetGrowMode(vtui.GrowHiX | vtui.GrowHiY)
	for _, button := range []*vtui.Button{btnCancel, btnEditor, btnClose} {
		button.SetGrowMode(vtui.GrowLoY | vtui.GrowHiY)
	}

	view := &ApplyOutputView{
		dlg: dlg, model: model, status: status, progress: progress, output: output,
		btnCancel: btnCancel, btnEditor: btnEditor, btnClose: btnClose,
	}
	model.mu.Lock()
	model.views[view] = struct{}{}
	model.mu.Unlock()
	removeView := func() {
		model.mu.Lock()
		delete(model.views, view)
		model.mu.Unlock()
	}
	dlg.OnResult = func(int) { removeView() }
	btnCancel.OnClick = func() {
		model.RequestCancel()
		if cancel != nil {
			cancel()
		}
	}
	btnEditor.OnClick = func() {
		consoleWidth, consoleHeight := 80, 25
		if anchor != nil {
			x1, y1, x2, y2 := anchor.GetPosition()
			consoleWidth, consoleHeight = x2-x1+1, y2-y1+1
		}
		editor := NewApplyTranscriptEditor(model, consoleWidth, consoleHeight)
		editor.StartIndexing()
		vtui.FrameManager.AddScreen(editor)
	}
	btnClose.OnClick = func() {
		removeView()
		dlg.Close()
	}
	view.refresh()
	if anchor != nil {
		vtui.FrameManager.PushToFrameScreen(anchor, dlg)
	} else {
		vtui.FrameManager.Push(dlg)
	}
	return dlg
}
