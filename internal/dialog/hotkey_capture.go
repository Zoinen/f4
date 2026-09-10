package dialog

import (
	"fmt"
	"strings"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type HotkeyAssignFrame struct {
	*vtui.Window
	Hm         *keymap.HotkeyManager
	ActionName string
	Area       string
	OnComplete func()
}

func NewHotkeyAssignFrame(hm *keymap.HotkeyManager, actionName, area string, onComplete func()) *HotkeyAssignFrame {
	width, height := 42, 9
	base := vtui.NewCenteredDialog(width, height, i18n.Msg("Hotkeys.AssignTitle"))
	f := &HotkeyAssignFrame{
		Window:     base,
		Hm:         hm,
		ActionName: actionName,
		Area:       area,
		OnComplete: onComplete,
	}

	lblAction := vtui.NewText(0, 0, fmt.Sprintf(i18n.Msg("Hotkeys.AssignAction"), actionName), vtui.Palette[vtui.ColDialogText])
	lblArea := vtui.NewText(0, 0, fmt.Sprintf(i18n.Msg("Hotkeys.AssignArea"), area), vtui.Palette[vtui.ColDialogText])
	currentText := fmt.Sprintf(i18n.Msg("Hotkeys.AssignCurrent"), i18n.Msg("Hotkeys.AssignNone"))
	if _, currentKey := keymap.ConfiguredHotkeyBinding(hm, strings.SplitN(actionName, ":", 2)[0]); currentKey != "" {
		currentText = fmt.Sprintf(i18n.Msg("Hotkeys.AssignCurrent"), keymap.FormatKeyForUI(currentKey))
	}
	lblCurrent := vtui.NewText(0, 0, currentText, vtui.Palette[vtui.ColDialogText])
	prompt := vtui.NewText(0, 0, i18n.Msg("Hotkeys.AssignPrompt"), vtui.Palette[vtui.ColDialogText])
	cancelPrompt := vtui.NewText(0, 0, i18n.Msg("Hotkeys.AssignCancel"), vtui.Palette[vtui.ColDialogText])

	f.AddItem(lblAction)
	f.AddItem(lblArea)
	f.AddItem(lblCurrent)
	f.AddItem(prompt)
	f.AddItem(cancelPrompt)

	vbox := vtui.NewVBoxLayout(f.X1+2, f.Y1+2, width-4, height-4)
	vbox.Add(lblAction, vtui.Margins{}, vtui.AlignCenter)
	vbox.Add(lblArea, vtui.Margins{Top: 1}, vtui.AlignCenter)
	vbox.Add(lblCurrent, vtui.Margins{Top: 1}, vtui.AlignCenter)
	vbox.Add(prompt, vtui.Margins{Top: 1}, vtui.AlignCenter)
	vbox.Add(cancelPrompt, vtui.Margins{Top: 1}, vtui.AlignCenter)
	vbox.Apply()

	return f
}

func (f *HotkeyAssignFrame) ProcessKey(e *vtinput.InputEvent) bool {
	if e.Type == vtinput.FocusEventType {
		return f.Window.ProcessKey(e)
	}

	if !e.KeyDown {
		return false
	}

	if e.VirtualKeyCode == vtinput.VK_ESCAPE {
		f.Close()
		vtui.FrameManager.Redraw()
		return true
	}

	switch e.VirtualKeyCode {
	case vtinput.VK_SHIFT, vtinput.VK_LSHIFT, vtinput.VK_RSHIFT,
		vtinput.VK_CONTROL, vtinput.VK_LCONTROL, vtinput.VK_RCONTROL,
		vtinput.VK_MENU, vtinput.VK_LMENU, vtinput.VK_RMENU,
		vtinput.VK_CAPITAL, vtinput.VK_NUMLOCK, vtinput.VK_SCROLL:
		return false
	}

	keyStr := keymap.EventToHotkeyString(e)

	if f.Hm != nil {
		f.Hm.Bind(f.Area, keyStr, f.ActionName)
	}

	f.Close()
	if f.OnComplete != nil {
		f.OnComplete()
	}
	vtui.FrameManager.Redraw()
	return true
}

func (f *HotkeyAssignFrame) ProcessMouse(e *vtinput.InputEvent) bool {
	return true
}
func (f *HotkeyAssignFrame) GetType() vtui.FrameType { return vtui.TypeDialog }
func (f *HotkeyAssignFrame) IsModal() bool           { return true }
