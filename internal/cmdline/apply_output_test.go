package cmdline

import (
	"fmt"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"testing"
)

// The apply-output dialog answers about its own view set and transcript,
// both private to the model. These three tests read them, which is why they
// are here rather than beside the action that opens the dialog.

func TestApplyOutputDialogCanCloseWhileForegroundBatchRuns(t *testing.T) {
	model := NewApplyBatchViewModel(1)
	dlg := ShowApplyOutputDialog(nil, model, nil)
	defer vtui.FrameManager.RemoveFrame(dlg)

	model.mu.Lock()
	var view *ApplyOutputView
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

func TestApplyOutputDialogExpandsTranscript(t *testing.T) {
	model := NewApplyBatchViewModel(1)
	for i := 0; i < 30; i++ {
		model.Transcript.Add(fmt.Sprintf("line %d", i))
	}
	dlg := ShowApplyOutputDialog(nil, model, nil)
	defer vtui.FrameManager.RemoveFrame(dlg)
	if !dlg.ShowZoom {
		t.Fatal("Apply output dialog has no expand control")
	}

	model.mu.Lock()
	var view *ApplyOutputView
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
	if view.output.ColorTextIdx != theme.ColViewerText || view.output.ColorSelectedTextIdx != theme.ColViewerStatus {
		t.Fatalf("transcript colors = %d/%d, want themed Viewer colors %d/%d",
			view.output.ColorTextIdx, view.output.ColorSelectedTextIdx, theme.ColViewerText, theme.ColViewerStatus)
	}
	if view.output.ScrollBar.ColorIdx != theme.ColViewerScrollbar {
		t.Fatalf("transcript scrollbar color = %d, want themed Viewer scrollbar %d", view.output.ScrollBar.ColorIdx, theme.ColViewerScrollbar)
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
	model := NewApplyBatchViewModel(1)
	model.Transcript.Add("first line")
	model.Transcript.Add("second line")
	editor := NewApplyTranscriptEditor(model, 80, 25)
	if got := editor.Pt.String(); got != "first line\nsecond line\n" {
		t.Fatalf("editor transcript = %q", got)
	}
	if editor.DisplayTitle != i18n.Msg("ApplyCommand.OutputEditorTitle") {
		t.Fatalf("editor title = %q", editor.DisplayTitle)
	}
}
