package panel

import (
	"runtime"

	"github.com/unxed/f4/internal/cmdline"
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// CommandLineReady reports whether a typed command owns input rather than a
// panel search or a running terminal application.
func (pf *PanelsFrame) CommandLineReady() bool {
	if pf.CmdLine == nil || !pf.CmdLine.IsVisible() || pf.CmdLine.IsEmpty() {
		return false
	}
	if !pf.ShowPanels {
		return pf.hiddenConsoleCommandLineOwnsInput()
	}
	return !pf.SearchFirstMode() || pf.CommandLineFocused
}

// RunCommandInNewWorkspace clones the panels and submits the exact command
// through the normal Enter path in the newly activated workspace.
func (pf *PanelsFrame) RunCommandInNewWorkspace() bool {
	if vtui.FrameManager == nil || !pf.CommandLineReady() {
		return false
	}
	command := pf.CmdLine.Edit.GetText()
	if cmdline.CommandHasUnmatchedQuote(command, runtime.GOOS == "windows") {
		vtui.ShowMessage(" Error ", "Unmatched quote in command. Close the quote and press Enter again.", []string{"&Ok"})
		return true
	}
	clone := pf.forkPanelsClone()
	clone.CmdLine.Edit.SetText(command)
	clone.SetCommandLineFocus(true)
	vtui.FrameManager.AddScreen(clone)
	if clone.deferWorkspaceCommand(command) {
		return true
	}
	return clone.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN,
	})
}

func (pf *PanelsFrame) deferWorkspaceCommand(command string) bool {
	if !SpawnLocalShellPTY || pf.ShellMode == terminal.ShellModeSimpleInline ||
		pf.ShellMode == terminal.ShellModeSimpleCaptured {
		return false
	}
	panel := pf.GetActivePanel()
	if panel == nil || !fileops.IsLocalOSVFS(panel.Vfs) || pf.GetActivePTY() != nil {
		return false
	}
	pf.pendingWorkspaceCommand = command
	return true
}

// Called on the UI thread after the new local PTY has been published. Never
// submit a stale command if the user edited the prompt or closed the workspace.
func (pf *PanelsFrame) submitPendingWorkspaceCommand() {
	command := pf.pendingWorkspaceCommand
	pf.pendingWorkspaceCommand = ""
	if command == "" || pf.Closed || pf.CmdLine.Edit.GetText() != command {
		return
	}
	pf.SetCommandLineFocus(true)
	pf.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN,
	})
}
