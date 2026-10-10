package app

import (
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/macro"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// A held Ctrl+W queues its repeats in the event channel while the exit
// confirmation is up. The filter must swallow them: the close-active-screen
// fallback below it would re-emit CmQuit and stack a duplicate dialog (one
// Cancel per queued repeat), and with several workspaces it would close one
// behind the modal dialog.
func TestMacroFilter_SwallowsCtrlWWhileQuitDialogOpen(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	oldConfirm := config.App.ConfirmExit
	config.App.ConfirmExit = true
	t.Cleanup(func() { config.App.ConfirmExit = oldConfirm })

	pf := panel.NewPanelsFrame()
	defer pf.Close()
	vtui.FrameManager.Push(pf)

	mgr := macro.NewMacroManager("")
	ctrlW := &vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_W,
		ControlKeyState: vtinput.LeftCtrlPressed,
	}

	pf.HandleCommand(vtui.CmQuit, nil)
	if !panel.QuitConfirmationOpen() {
		t.Fatal("Quit dialog didn't appear")
	}
	if !macroFilter(mgr, ctrlW) {
		t.Error("Ctrl+W must be swallowed while the quit dialog is open")
	}
}
