package main

import (
	"fmt"
	"strings"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// driveMenuFrame adds the Far-style operation hint to a drive menu while
// keeping the embedded VMenu available to the menu's existing callers.
type driveMenuFrame struct {
	*vtui.VMenu
	bottomHint string
}

func (f *driveMenuFrame) Show(scr *vtui.ScreenBuf) {
	f.VMenu.Show(scr)
	if f.bottomHint == "" {
		return
	}
	x1, _, x2, y2 := f.GetPosition()
	vtui.NewPainter(scr).DrawTitle(x1, y2, x2, f.bottomHint, vtui.Palette[vtui.ColMenuTitle])
}

func (f *driveMenuFrame) ProcessKey(e *vtinput.InputEvent) bool {
	handled := f.VMenu.ProcessKey(e)
	// VMenu checks whether it is the exact top frame before reporting that
	// Esc/F10 was consumed. The wrapper is the top frame in this menu, so
	// finish that part of the contract here.
	if e != nil && e.KeyDown && f.IsDone() &&
		(e.VirtualKeyCode == vtinput.VK_ESCAPE || e.VirtualKeyCode == vtinput.VK_F10) {
		return true
	}
	return handled
}

type driveBookmarkEditDialog struct {
	*vtui.Window
	nameEdit   *vtui.Edit
	pathEdit   *vtui.Edit
	hotkeyEdit *vtui.Edit
	finished   bool
	onFinish   func(bool, DriveBookmark)
}

func newDriveBookmarkEditDialog(initial DriveBookmark, defaultPath string, onFinish func(bool, DriveBookmark)) *driveBookmarkEditDialog {
	const (
		width  = 64
		height = 14
	)
	titleKey := "DriveLink.CreateTitle"
	if initial.Name != "" || initial.Path != "" {
		titleKey = "DriveLink.EditTitle"
	}
	dialog := &driveBookmarkEditDialog{
		Window:     vtui.NewCenteredDialog(width, height, Msg(titleKey)),
		nameEdit:   vtui.NewEdit(0, 0, width-6, initial.Name),
		pathEdit:   vtui.NewEdit(0, 0, width-6, initial.Path),
		hotkeyEdit: vtui.NewEdit(0, 0, 1, initial.Hotkey),
		onFinish:   onFinish,
	}
	if dialog.pathEdit.GetText() == "" {
		dialog.pathEdit.SetText(defaultPath)
	}

	nameLabel := vtui.NewLabel(0, 0, Msg("DriveLink.Name"), dialog.nameEdit)
	pathLabel := vtui.NewLabel(0, 0, Msg("DriveLink.Path"), dialog.pathEdit)
	hotkeyLabel := vtui.NewLabel(0, 0, Msg("DriveLink.Hotkey"), dialog.hotkeyEdit)
	hotkeyHint := vtui.NewText(0, 0, Msg("DriveLink.HotkeyHint"), vtui.Palette[vtui.ColDialogText])
	createText := Msg("DriveLink.Create")
	if initial.Name != "" || initial.Path != "" {
		createText = Msg("DriveLink.Save")
	}
	createButton := vtui.NewButton(0, 0, createText)
	createButton.IsDefault = true
	cancelButton := vtui.NewButton(0, 0, Msg("DriveLink.Cancel"))
	createButton.OnClick = func() { dialog.submit() }
	cancelButton.OnClick = func() { dialog.finish(false, DriveBookmark{}) }

	dialog.AddItem(nameLabel)
	dialog.AddItem(dialog.nameEdit)
	dialog.AddItem(pathLabel)
	dialog.AddItem(dialog.pathEdit)
	dialog.AddItem(hotkeyLabel)
	dialog.AddItem(dialog.hotkeyEdit)
	dialog.AddItem(hotkeyHint)
	dialog.AddItem(createButton)
	dialog.AddItem(cancelButton)

	vbox := vtui.NewVBoxLayout(dialog.X1+2, dialog.Y1+2, width-4, height-4)
	vbox.Add(nameLabel, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(dialog.nameEdit, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(pathLabel, vtui.Margins{Top: 1}, vtui.AlignLeft)
	vbox.Add(dialog.pathEdit, vtui.Margins{}, vtui.AlignFill)
	hotkeyRow := vtui.NewHBoxLayout(0, 0, width-4, 1)
	hotkeyRow.Add(hotkeyLabel, vtui.Margins{Right: 1}, vtui.AlignLeft)
	hotkeyRow.Add(dialog.hotkeyEdit, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(hotkeyRow, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Add(hotkeyHint, vtui.Margins{}, vtui.AlignLeft)
	buttons := vtui.NewHBoxLayout(0, 0, width-4, 1)
	buttons.HorizontalAlign = vtui.AlignCenter
	buttons.Spacing = 2
	buttons.Add(createButton, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(cancelButton, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(buttons, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()

	dialog.SetFocusedItem(dialog.nameEdit)
	dialog.OnResult = func(code int) {
		if !dialog.finished {
			dialog.finished = true
			if dialog.onFinish != nil {
				dialog.onFinish(false, DriveBookmark{})
			}
		}
	}
	return dialog
}

func (d *driveBookmarkEditDialog) finish(saved bool, bookmark DriveBookmark) {
	if d.finished {
		return
	}
	d.finished = true
	d.SetExitCode(1)
	if d.onFinish != nil {
		d.onFinish(saved, bookmark)
	}
}

func (d *driveBookmarkEditDialog) submit() {
	bookmark := DriveBookmark{
		Name:   strings.TrimSpace(d.nameEdit.GetText()),
		Path:   strings.TrimSpace(d.pathEdit.GetText()),
		Hotkey: strings.TrimSpace(d.hotkeyEdit.GetText()),
	}
	if bookmark.Name == "" {
		vtui.ShowMessageOn(d, Msg("DriveLink.ErrorTitle"), Msg("DriveLink.NameRequired"), []string{"&Ok"})
		d.SetFocusedItem(d.nameEdit)
		return
	}
	if bookmark.Path == "" {
		vtui.ShowMessageOn(d, Msg("DriveLink.ErrorTitle"), Msg("DriveLink.PathRequired"), []string{"&Ok"})
		d.SetFocusedItem(d.pathEdit)
		return
	}
	d.finish(true, bookmark)
}

func (d *driveBookmarkEditDialog) ProcessKey(e *vtinput.InputEvent) bool {
	if e != nil && e.KeyDown && d.GetFocusedItem() == d.hotkeyEdit {
		if e.VirtualKeyCode == vtinput.VK_TAB || e.VirtualKeyCode == vtinput.VK_ESCAPE {
			return d.Window.ProcessKey(e)
		}
		if e.VirtualKeyCode == vtinput.VK_BACK || e.VirtualKeyCode == vtinput.VK_DELETE {
			d.hotkeyEdit.SetText("")
			return true
		}
		key := EventToHotkeyString(e)
		if key != "" && key != "VK_0" {
			d.hotkeyEdit.SetText(key)
			return true
		}
	}
	return d.Window.ProcessKey(e)
}

func (pf *PanelsFrame) driveBookmarkDefaultPath(panelIdx int) string {
	if panelIdx < 0 || panelIdx >= len(pf.panels) {
		return ""
	}
	fsp, ok := pf.panels[panelIdx].(*FileSystemPanel)
	if !ok || fsp.vfs == nil {
		return ""
	}
	return fsp.vfs.GetPath()
}

func (pf *PanelsFrame) openDriveBookmarkEditor(panelIdx int, menu *vtui.VMenu, bookmarks []DriveBookmark, index int, reopen func()) {
	var initial DriveBookmark
	if index >= 0 && index < len(bookmarks) {
		initial = bookmarks[index]
	}
	if menu != nil {
		menu.Close()
	}
	vtui.FrameManager.PostTask(func() {
		dialog := newDriveBookmarkEditDialog(initial, pf.driveBookmarkDefaultPath(panelIdx), func(saved bool, bookmark DriveBookmark) {
			if !saved {
				vtui.FrameManager.PostTask(reopen)
				return
			}
			current, err := LoadDriveBookmarks(DriveBookmarksFilePath())
			if err == nil {
				if index >= 0 && index < len(current) {
					current[index] = bookmark
				} else {
					current = append(current, bookmark)
				}
				err = SaveDriveBookmarks(DriveBookmarksFilePath(), current)
			}
			if err != nil {
				message := vtui.ShowMessage(Msg("DriveLink.ErrorTitle"), fmt.Sprintf(Msg("DriveLink.SaveError"), err), []string{"&Ok"})
				message.OnResult = func(int) { vtui.FrameManager.PostTask(reopen) }
				return
			}
			vtui.FrameManager.PostTask(reopen)
		})
		vtui.FrameManager.Push(dialog)
	})
}

func (pf *PanelsFrame) deleteDriveBookmark(menu *vtui.VMenu, bookmarks []DriveBookmark, index int, reopen func()) {
	if index < 0 || index >= len(bookmarks) {
		return
	}
	question := fmt.Sprintf(Msg("DriveLink.DeleteQuestion"), bookmarks[index].Name)
	vtui.ShowMessageOn(menu, Msg("DriveLink.DeleteTitle"), question, []string{"&Delete", Msg("DriveLink.Cancel")}).OnResult = func(choice int) {
		if choice != 0 {
			return
		}
		current, err := LoadDriveBookmarks(DriveBookmarksFilePath())
		if err == nil && index >= 0 && index < len(current) {
			current = append(current[:index], current[index+1:]...)
			err = SaveDriveBookmarks(DriveBookmarksFilePath(), current)
		}
		if err != nil {
			message := vtui.ShowMessage(Msg("DriveLink.ErrorTitle"), fmt.Sprintf(Msg("DriveLink.SaveError"), err), []string{"&Ok"})
			message.OnResult = func(int) { menu.Close(); vtui.FrameManager.PostTask(reopen) }
			return
		}
		menu.Close()
		vtui.FrameManager.PostTask(reopen)
	}
}
