package settings

import (
	"github.com/unxed/f4/internal/dialog"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/vtui"
)

type settingsChord struct {
	*vtui.Group
	edit    *vtui.Edit
	capture *vtui.Button
}

func newSettingsChord(value string, change func(string)) *settingsChord {
	c := &settingsChord{Group: vtui.NewGroup(0, 0, 30, 1), edit: vtui.NewEdit(0, 0, 16, value), capture: vtui.NewButton(0, 0, settingsText("Capture", "Capture"))}
	c.edit.OnTextChange = change
	c.AddItem(c.edit)
	c.AddItem(c.capture)
	c.capture.OnClick = func() {
		hm := &keymap.HotkeyManager{Bindings: map[string]map[string]string{}, Defaults: map[string]map[string]string{}}
		frame := dialog.NewHotkeyAssignFrame(hm, "Settings.Capture", "Common", func() {
			for key := range hm.Bindings["Common"] {
				c.edit.SetText(key)
				change(key)
				break
			}
		})
		vtui.FrameManager.Push(frame)
	}
	return c
}
func (c *settingsChord) SetPosition(x1, y1, x2, y2 int) {
	c.Group.SetPosition(x1, y1, x2, y2)
	width := vtui.StringWidth(c.capture.GetCaption()) + 4
	c.edit.SetPosition(x1, y1, max(x1, x2-width-1), y1)
	c.capture.SetPosition(max(x1, x2-width+1), y1, x2, y1)
}
