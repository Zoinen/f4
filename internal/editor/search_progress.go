package editor

import (
	"fmt"
	"time"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/vtui"
)

// newSearchProgressDialog prepares a popup without changing focus or frames.
func newSearchProgressDialog(pattern string) (dlg *vtui.Window, btnCancel *vtui.Button) {
	dlg = vtui.NewCenteredDialog(50, 8, i18n.Msg("Search.Searching"))
	lbl := vtui.NewLabel(0, 0, fmt.Sprintf(i18n.Msg("Search.LookingFor"), pattern), nil)
	dlg.AddItem(lbl)
	btnCancel = vtui.NewButton(0, 0, i18n.Msg("vtui.Cancel"))
	dlg.AddItem(btnCancel)

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, 50-4, 8-4)
	vbox.Add(lbl, vtui.Margins{}, vtui.AlignCenter)
	vbox.Add(btnCancel, vtui.Margins{Top: 1}, vtui.AlignCenter)
	vbox.Apply()

	return dlg, btnCancel
}

// RunSearchWithProgress delays the progress popup and runs worker in the
// background. The cancel wiring lives here, on the UI thread: the Cancel
// button and any other close of the dialog (Esc, F10, a border click) all
// cancel the task, so a dismissed search can never resurface its result.
// Wiring after RunAsync is race-free because queued UI tasks only run once
// the current one returns. Must be called on the UI thread; workers close
// dlg from their RunOnUI callback and must check ctx.Err() before acting.
func RunSearchWithProgress(
	pattern string,
	worker func(ctx *vtui.TaskContext, dlg *vtui.Window),
) (*vtui.Window, *vtui.TaskContext) {
	return RunSearchWithProgressOn(vtui.FrameManager.GetTopFrame(), pattern, worker)
}

// RunSearchWithProgressOn keeps progress in the document's workspace even when
// a queued search starts after the user has switched to another tab.
// Searches completed within 150 ms never expose a modal progress frame.
func RunSearchWithProgressOn(
	anchor vtui.Frame,
	pattern string,
	worker func(ctx *vtui.TaskContext, dlg *vtui.Window),
) (*vtui.Window, *vtui.TaskContext) {
	dlg, btnCancel := newSearchProgressDialog(pattern)
	frames := vtui.FrameManager
	ctx := vtui.RunAsync(func(c *vtui.TaskContext) { worker(c, dlg) })
	// Completion closes the prepared window before this deadline on the fast
	// path. The queued callback checks again because it may already be posted
	// when completion or cancellation stops the timer.
	timer := time.AfterFunc(150*time.Millisecond, func() {
		ctx.RunOnUIWithRedrawDecision(func() bool {
			if dlg.IsDone() || ctx.Err() != nil {
				return false
			}
			frames.PushToFrameScreen(anchor, dlg)
			return true
		})
	})
	btnCancel.OnClick = func() { ctx.Cancel(); dlg.Close() }
	dlg.OnResult = func(int) { timer.Stop(); ctx.Cancel() }
	return dlg, ctx
}
