package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type applyBatchViewModel struct {
	mu             sync.Mutex
	transcript     *applyTranscript
	total          int
	completed      int
	failed         int
	cancelled      int
	runningName    string
	done           bool
	cancelling     bool
	refreshPending bool
	views          map[*applyOutputView]struct{}
}

func newApplyBatchViewModel(total int) *applyBatchViewModel {
	return &applyBatchViewModel{
		transcript: newApplyTranscript(),
		total:      total,
		views:      make(map[*applyOutputView]struct{}),
	}
}

func (m *applyBatchViewModel) Observe(parallel bool, ev applyBatchEvent) {
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
			m.transcript.Add(fmt.Sprintf("[%d/%d %s] $ %s", ev.Index+1, ev.Total, ev.Name, ev.Line))
		}
	case applyBatchOutput:
		if parallel {
			m.transcript.Add(fmt.Sprintf("[%d/%d %s] %s", ev.Index+1, ev.Total, ev.Name, ev.Line))
		} else {
			m.transcript.Add(ev.Line)
		}
	case applyBatchItemFinished:
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
			m.transcript.Add(fmt.Sprintf("[%d/%d %s] %s", ev.Index+1, ev.Total, ev.Name, applyResultSummary(ev.Result)))
		}
	}
	m.requestRefresh()
}

func applyResultSummary(result applyBatchItemResult) string {
	switch result.State {
	case applyItemSucceeded:
		return Msg("ApplyCommand.ResultSuccess")
	case applyItemCancelled:
		return Msg("ApplyCommand.ResultCancelled")
	case applyItemFailed:
		if result.Err != nil {
			return fmt.Sprintf(Msg("ApplyCommand.ResultFailedFmt"), result.Err)
		}
		return Msg("ApplyCommand.ResultFailed")
	default:
		return Msg("ApplyCommand.ResultPending")
	}
}

func (m *applyBatchViewModel) Finish(result applyBatchResult) {
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
	m.transcript.Add("")
	m.transcript.Add(fmt.Sprintf(Msg("ApplyCommand.SummaryFmt"), result.Succeeded, result.Failed, result.Cancelled, result.NotStarted))
	m.requestRefresh()
}

func (m *applyBatchViewModel) RequestCancel() {
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

func (m *applyBatchViewModel) requestRefresh() {
	m.mu.Lock()
	if m.refreshPending {
		m.mu.Unlock()
		return
	}
	m.refreshPending = true
	m.mu.Unlock()
	time.AfterFunc(50*time.Millisecond, func() {
		if vtui.FrameManager == nil {
			m.mu.Lock()
			m.refreshPending = false
			m.mu.Unlock()
			return
		}
		vtui.FrameManager.PostTask(func() {
			m.mu.Lock()
			m.refreshPending = false
			views := make([]*applyOutputView, 0, len(m.views))
			for view := range m.views {
				views = append(views, view)
			}
			m.mu.Unlock()
			for _, view := range views {
				view.refresh()
			}
			vtui.FrameManager.Redraw()
		})
	})
}

func (m *applyBatchViewModel) snapshotStatus() (status string, percent int, done bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.total > 0 {
		percent = m.completed * 100 / m.total
	}
	if m.done {
		status = fmt.Sprintf(Msg("ApplyCommand.StatusDoneFmt"), m.completed, m.total, m.failed, m.cancelled)
	} else if m.cancelling {
		status = Msg("ApplyCommand.StatusCancelling")
	} else {
		status = fmt.Sprintf(Msg("ApplyCommand.StatusRunningFmt"), m.completed, m.total, escapeAmpersand(m.runningName))
	}
	return status, percent, m.done
}

func (m *applyBatchViewModel) IsDone() bool {
	if m == nil {
		return true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.done
}

type applyOutputView struct {
	dlg       *applyOutputDialog
	model     *applyBatchViewModel
	status    *vtui.Text
	progress  *vtui.ProgressBar
	output    *vtui.ListBox
	btnCancel *vtui.Button
	btnEditor *vtui.Button
	btnClose  *vtui.Button
}

type applyOutputDialog struct {
	*vtui.Window
}

func (d *applyOutputDialog) ProcessKey(event *vtinput.InputEvent) bool {
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

func (v *applyOutputView) refresh() {
	if v == nil || v.model == nil {
		return
	}
	status, pct, done := v.model.snapshotStatus()
	v.status.SetText(status)
	v.progress.SetPercent(pct)
	v.output.Items = v.model.transcript.Snapshot()
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

func newApplyTranscriptEditor(model *applyBatchViewModel, width, height int) *EditorView {
	lines := model.transcript.Snapshot()
	text := strings.Join(lines, "\n")
	if len(lines) > 0 {
		text += "\n"
	}
	editor := NewEditorView(piecetable.New([]byte(text)), nil, "")
	editor.DisplayTitle = Msg("ApplyCommand.OutputEditorTitle")
	editor.ResizeConsole(width, height)
	return editor
}

// showApplyOutputDialog opens a live or completed transcript. Closing a live
// view only detaches the UI; the foreground or queued batch keeps running.
func showApplyOutputDialog(anchor vtui.Frame, model *applyBatchViewModel, cancel func()) *applyOutputDialog {
	const width, height = 86, 24
	dlg := &applyOutputDialog{Window: vtui.NewCenteredDialog(width, height, Msg("ApplyCommand.OutputTitle"))}
	dlg.ShowClose = true
	dlg.ShowZoom = true
	dlg.SetHelp("ApplyCmd")

	status := vtui.NewText(0, 0, "", 0)
	progress := vtui.NewProgressBar(0, 0, width-4)
	output := vtui.NewListBox(0, 0, width-4, height-9, nil)
	output.ShowScrollBar = true
	output.ColorTextIdx = ColViewerText
	output.ColorSelectedTextIdx = ColViewerStatus
	output.ColorItemSelectTextIdx = ColViewerText
	output.ColorItemSelectCursorIdx = ColViewerStatus
	if output.ScrollBar != nil {
		output.ScrollBar.ColorIdx = ColViewerScrollbar
	}
	btnCancel := vtui.NewButton(0, 0, Msg("ApplyCommand.CancelTask"))
	btnEditor := vtui.NewButton(0, 0, Msg("ApplyCommand.SendToEditor"))
	btnClose := vtui.NewButton(0, 0, Msg("ApplyCommand.Close"))

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

	view := &applyOutputView{
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
		editor := newApplyTranscriptEditor(model, consoleWidth, consoleHeight)
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
