package settings

import (
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func settingsUseRadios(f f4settings.Field) bool {
	if f.AllowCustom || f.ChoicePresentation == "dropdown" {
		return false
	}
	return len(f.Choices) > 0 && (f.ChoicePresentation == "radio" || len(f.Choices) <= 5)
}

type settingsRadioOption struct {
	*vtui.RadioButton
	label  string
	lines  []string
	dx, dy int
}

func (b *settingsRadioOption) Show(scr *vtui.ScreenBuf) {
	b.RadioButton.Show(scr)
	n, _ := b.GetStateAttrs(vtui.ColDialogText, vtui.ColDialogSelectedButton, vtui.ColDialogHighlightText, vtui.ColDialogHighlightSelectedButton)
	for i := 1; i < len(b.lines); i++ {
		scr.Write(b.X1+4, b.Y1+i, vtui.StringToCharInfo(b.lines[i], n))
	}
}

type settingsRadios struct {
	*vtui.Group
	buttons   []*settingsRadioOption
	hover     int
	onExplain func()
}

func newSettingsRadios(labels []string, selected int, change func(int)) *settingsRadios {
	r := &settingsRadios{Group: vtui.NewGroup(0, 0, 20, 1), hover: -1}
	for i, label := range labels {
		b := &settingsRadioOption{RadioButton: vtui.NewRadioButton(0, 0, label, i == selected), label: label}
		b.OnChange = func(on bool) {
			if !on {
				return
			}
			for j, other := range r.buttons {
				other.Selected = j == i
			}
			change(i)
		}
		r.buttons = append(r.buttons, b)
		r.AddItem(b)
	}
	r.SetFocusedItem(r.buttons[selected])
	return r
}

// Fit the entire choice set on one line; otherwise stack and wrap labels.
// Recalculate from translated display widths on every resize.
func (r *settingsRadios) inlineWidth() int {
	total := -2
	for _, b := range r.buttons {
		total += 6 + vtui.StringWidth(b.label)
	}
	return max(0, total)
}

func (r *settingsRadios) layout(width int) int {
	inline := r.inlineWidth() <= width
	x, y := 0, 0
	for _, b := range r.buttons {
		b.lines = settingsWrap(b.label, max(1, width-4))
		b.SetText(b.lines[0])
		b.dx, b.dy = x, y
		if inline {
			x += 6 + vtui.StringWidth(b.label)
		} else {
			y += len(b.lines)
		}
	}
	if inline {
		return 1
	}
	return y
}
func (r *settingsRadios) SetPosition(x1, y1, x2, y2 int) {
	r.Group.SetPosition(x1, y1, x2, y2)
	for _, b := range r.buttons {
		width := 4
		for _, line := range b.lines {
			width = max(width, 4+vtui.StringWidth(line))
		}
		b.SetPosition(x1+b.dx, y1+b.dy, min(x2, x1+b.dx+width-1), y1+b.dy+len(b.lines)-1)
	}
}
func (r *settingsRadios) helpIndex() int {
	if r.hover >= 0 {
		return r.hover
	}
	for i, b := range r.buttons {
		if r.GetFocusedItem() == b {
			return i
		}
	}
	return -1
}
func (r *settingsRadios) ProcessKey(e *vtinput.InputEvent) bool {
	r.hover = -1
	// Left/Right retain the Settings Center's category/content pane navigation.
	// Up/Down browse visible choices; Space selects without changing on focus.
	if e.VirtualKeyCode == vtinput.VK_LEFT || e.VirtualKeyCode == vtinput.VK_RIGHT {
		return false
	}
	return r.Group.ProcessKey(e)
}
func (r *settingsRadios) ProcessMouse(e *vtinput.InputEvent) bool {
	r.hover = -1
	for i, b := range r.buttons {
		if b.HitTest(int(e.MouseX), int(e.MouseY)) {
			r.hover = i
			break
		}
	}
	handled := r.Group.ProcessMouse(e)
	if r.onExplain != nil {
		r.onExplain()
	}
	return handled
}
