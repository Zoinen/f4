package intchecker

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// hashListWindow is the "Display" output: the checksum list in a window,
// with buttons to copy it to the clipboard or save it to a file.
type hashListWindow struct {
	win     *vtui.Window
	list    *vtui.ListBox
	btnCopy *vtui.Button
	btnSave *vtui.Button
}

// hashListLines splits the checksum list into the lines the window shows.
func hashListLines(text string) []string {
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

// newHashListWindow builds the window for text, sized to the longest line
// within the screen.
func newHashListWindow(text string) *hashListWindow {
	lines := hashListLines(text)
	width := 66 // room for the three buttons in every language
	for _, line := range lines {
		if w := vtui.StringWidth(line) + 6; w > width {
			width = w
		}
	}
	height := len(lines) + 7
	if height < 10 {
		height = 10
	}
	if vtui.FrameManager != nil {
		if maximum := vtui.FrameManager.GetScreenSize() - 2; maximum > 20 && width > maximum {
			width = maximum
		}
		if maximum := vtui.FrameManager.GetScreenHeight() - 2; maximum > 8 && height > maximum {
			height = maximum
		}
	}
	w := &hashListWindow{win: vtui.NewCenteredDialog(width, height, vtui.Msg("IntChecker.HashListTitle"))}
	w.win.ShowClose = true

	w.list = vtui.NewListBox(0, 0, width-4, height-6, lines)
	w.btnCopy = vtui.NewButton(0, 0, vtui.Msg("IntChecker.CopyToClipboard"))
	w.btnSave = vtui.NewButton(0, 0, vtui.Msg("IntChecker.SaveToFile"))
	btnClose := vtui.NewButton(0, 0, vtui.Msg("IntChecker.Close"))
	btnClose.IsDefault = true
	btnClose.OnClick = func() { w.win.Close() }
	for _, item := range []vtui.UIElement{w.list, w.btnCopy, w.btnSave, btnClose} {
		w.win.AddItem(item)
	}

	vbox := vtui.NewVBoxLayout(w.win.X1+2, w.win.Y1+2, width-4, height-4)
	vbox.Add(w.list, vtui.Margins{Bottom: 1}, vtui.AlignFill)
	buttons := vtui.NewHBoxLayout(0, 0, width-4, 1)
	buttons.HorizontalAlign = vtui.AlignCenter
	buttons.Spacing = 1
	buttons.Add(w.btnCopy, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(w.btnSave, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(btnClose, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(buttons, vtui.Margins{}, vtui.AlignFill)
	vbox.Apply()
	w.win.SetFocusedItem(w.list)
	return w
}

// showHashList opens the list window for a finished "Display" run. report,
// when not empty, tells about files that could not be read. It runs on the
// UI goroutine.
func showHashList(app vfs.App, job generateJob, text, report string) {
	w := newHashListWindow(text)
	w.btnCopy.OnClick = func() {
		go vtui.SetClipboard(text)
	}
	w.btnSave.OnClick = func() {
		vtui.InputBoxOn(w.win, vtui.Msg("IntChecker.SaveToFileTitle"), vtui.Msg("IntChecker.OutputFileName"),
			defaultOutputName(job.fs.Base(job.dir), job.algorithm), func(answer string) {
				target, ok := resolveSavePath(job.fs, job.dir, answer)
				if !ok {
					return
				}
				go saveHashList(app, w.win, job, target, []byte(text))
			})
	}
	if anchor, ok := app.(vtui.Frame); ok {
		vtui.FrameManager.PushToFrameScreen(anchor, w.win)
	} else {
		vtui.FrameManager.Push(w.win)
	}
	if report != "" {
		vtui.ShowMessageOn(w.win, vtui.Msg("IntChecker.Title"), report, []string{vtui.Msg("vtui.Ok")})
	}
}

// resolveSavePath turns the typed file name into a path: a relative name is
// taken from the panel directory. An empty name means "do not save".
func resolveSavePath(fs vfs.VFS, dir, text string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" || text == "." || text == ".." {
		return "", false
	}
	if fs.IsAbs(text) {
		return text, true
	}
	return fs.Join(dir, text), true
}

// saveHashList writes the displayed list to target in the job's encoding,
// asking before it replaces an existing file. It waits for the answer, so it
// runs off the UI goroutine; the dialogs it opens sit on top of the list
// window.
func saveHashList(app vfs.App, win *vtui.Window, job generateJob, target string, data []byte) {
	title := vtui.Msg("IntChecker.Title")
	ctx := context.Background()
	overwrite := false
	if _, err := job.fs.Stat(ctx, target); err == nil {
		answers := make(chan int, 1)
		vtui.FrameManager.PostTask(func() {
			dlg := vtui.ShowMessageOn(win, title, fmt.Sprintf(vtui.Msg("IntChecker.OverwriteQuestion"), target),
				[]string{vtui.Msg("IntChecker.Overwrite"), vtui.Msg("vtui.Cancel")})
			dlg.OnResult = func(code int) { answers <- code }
		})
		if <-answers != 0 {
			return
		}
		overwrite = true
	}
	encoded, err := job.encoding.encode(string(data))
	if err == nil {
		err = writeFile(ctx, job.fs, target, encoded, overwrite)
	}
	vtui.FrameManager.PostTask(func() {
		if errors.Is(err, errUnencodableName) {
			vtui.ShowMessageOn(win, title, fmt.Sprintf(vtui.Msg("IntChecker.CannotEncodeList"), job.encoding.name()), []string{vtui.Msg("vtui.Ok")})
			return
		}
		if err != nil {
			vtui.ShowMessageOn(win, title, fmt.Sprintf(vtui.Msg("IntChecker.WriteError"), target, err), []string{vtui.Msg("vtui.Ok")})
			return
		}
		if job.fs.Dir(target) == job.dir {
			app.SetPendingSelection(job.fs.Base(target))
		}
		app.RefreshAll()
		vtui.ShowMessageOn(win, title, fmt.Sprintf(vtui.Msg("IntChecker.Saved"), target), []string{vtui.Msg("vtui.Ok")})
	})
}
