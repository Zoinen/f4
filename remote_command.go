package main

import (
	"fmt"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// panelCanRunCommand reports whether the active panel's file system can run
// a command where its files are. A local panel has a shell of its own and
// does not need this one.
func panelCanRunCommand() bool {
	pf := findPanelsFrameAnyScreen()
	if pf == nil {
		return false
	}
	fsp := pf.getActivePanel()
	if fsp == nil {
		return false
	}
	_, ok := fsp.vfs.(vfs.CommandRunner)
	return ok
}

// actionRunRemoteCommand asks for a command line and runs it in the
// directory the panel is showing, on the host that owns it. The output
// arrives while the command is still running, because a command that takes
// a while is exactly the one worth watching.
func actionRunRemoteCommand(pf *PanelsFrame) {
	fsp := pf.getActivePanel()
	if fsp == nil {
		return
	}
	runner, ok := fsp.vfs.(vfs.CommandRunner)
	if !ok {
		vtui.ShowMessage(Msg("RemoteCmd.Title"),
			"This file system cannot run commands.", []string{"&Ok"})
		return
	}
	dir := fsp.vfs.GetPath()

	vtui.InputBoxOn(pf, Msg("RemoteCmd.Title"), Msg("RemoteCmd.Prompt"), "", func(command string) {
		if command == "" {
			return
		}
		showRemoteCommandOutput(pf, runner, dir, command)
	})
}

func showRemoteCommandOutput(pf *PanelsFrame, runner vfs.CommandRunner, dir, command string) {
	width, height := 76, 20
	dlg := vtui.NewCenteredDialog(width, height, Msg("RemoteCmd.Title")+": "+command)
	dlg.ShowClose = true

	lb := vtui.NewListBox(0, 0, width-4, height-6, nil)
	btnClose := vtui.NewButton(0, 0, Msg("RemoteCmd.BtnClose"))
	dlg.AddItem(lb)
	dlg.AddItem(btnClose)

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)
	vbox.Add(lb, vtui.Margins{Bottom: 1}, vtui.AlignFill)
	hbox := vtui.NewHBoxLayout(0, 0, width-4, 1)
	hbox.HorizontalAlign = vtui.AlignCenter
	hbox.Add(btnClose, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(hbox, vtui.Margins{}, vtui.AlignFill)
	vbox.Apply()

	var taskCtx *vtui.TaskContext
	// Closing the window stops the command. A command whose output nobody
	// is reading is not the same as a job worth keeping: it was started to
	// be watched, and leaving it running on somebody's server with its
	// output going nowhere would be the wrong kind of thrift.
	btnClose.OnClick = func() {
		if taskCtx != nil {
			taskCtx.Cancel()
		}
		dlg.Close()
	}
	vtui.FrameManager.Push(dlg)

	taskCtx = vtui.RunAsync(func(ctx *vtui.TaskContext) {
		code, err := runner.RunCommand(ctx.Context, dir, command, func(line string) {
			ctx.RunOnUI(func() {
				lb.Items = append(lb.Items, line)
				lb.UpdateRows()
				vtui.FrameManager.Redraw()
			})
		})
		ctx.RunOnUI(func() {
			if ctx.Err() != nil {
				return
			}
			tail := fmt.Sprintf("[exit status %d]", code)
			if err != nil {
				tail = fmt.Sprintf("[failed: %v]", err)
			}
			lb.Items = append(lb.Items, "", tail)
			lb.UpdateRows()
			vtui.FrameManager.Redraw()
		})
	})
}
