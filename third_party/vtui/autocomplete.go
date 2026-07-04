package vtui

import (
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/vtinput"
)

type AutoCompleteMenu struct {
	Window
	Edit    *Edit
	lb      *ListBox
	Matches []string
}

func NewAutoCompleteMenu(edit *Edit) *AutoCompleteMenu {
	ac := &AutoCompleteMenu{
		Window: *NewWindow(0, 0, 10, 10, ""),
		Edit:   edit,
	}
	ac.ShowClose = false
	ac.ShowZoom = false
	ac.Modal = true
	ac.frame.boxType = SingleBox

	ac.lb = NewListBox(0, 0, 10, 10, nil)
	ac.lb.ColorTextIdx = ColDialogText
	ac.lb.ColorSelectedTextIdx = ColDialogSelectedButton
	ac.lb.OnAction = func(idx int) {
		if idx >= 0 && idx < len(ac.Matches) {
			ac.Edit.SetText(ac.Matches[idx])
			ac.Edit.curPos = len(ac.Edit.text)
			ac.Edit.clearFlag = false
			ac.Edit.HistoryPos = -1
			// Inject Enter to execute chosen command immediately
			if FrameManager != nil {
				FrameManager.InjectEvents([]*vtinput.InputEvent{
					{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN},
				})
			}
		}
		ac.Close()
	}
	ac.AddItem(ac.lb)

	ac.UpdateMatches()
	return ac
}

func (ac *AutoCompleteMenu) HasShadow() bool {
	return false
}

func (ac *AutoCompleteMenu) SetPosition(x1, y1, x2, y2 int) {
	ac.Window.SetPosition(x1, y1, x2, y2)
	ac.X1, ac.Y1, ac.X2, ac.Y2 = x1, y1, x2, y2
	if ac.frame != nil {
		ac.frame.SetPosition(x1, y1, x2, y2)
	}
	if ac.rootGroup != nil {
		ac.rootGroup.SetPosition(x1+1, y1+1, x2-1, y2-1)
	}
	if ac.lb != nil {
		ac.lb.SetPosition(x1+1, y1+1, x2-1, y2-1)
	}
}

func (ac *AutoCompleteMenu) HasMatches() bool {
	return len(ac.Matches) > 0
}

func (ac *AutoCompleteMenu) UpdateMatches() {
	text := ac.Edit.GetText()
	ac.Matches = nil
	if text == "" {
		return
	}
	textLower := strings.ToLower(text)

	seen := make(map[string]bool)

	for _, h := range ac.Edit.History {
		if strings.HasPrefix(strings.ToLower(h), textLower) {
			if !seen[h] {
				ac.Matches = append(ac.Matches, h)
				seen[h] = true
			}
		}
	}

	if len(ac.Matches) == 0 {
		return
	}

	ac.lb.Items = ac.Matches
	ac.lb.UpdateRows()

	if ac.lb.SelectPos >= len(ac.Matches) {
		ac.lb.SetSelectPos(len(ac.Matches) - 1)
	} else if ac.lb.SelectPos < 0 {
		ac.lb.SetSelectPos(0)
	}

	scrW := 80
	if FrameManager != nil && FrameManager.scr != nil {
		scrW = FrameManager.scr.width
	}

	h := len(ac.Matches) + 2
	if h > 12 {
		h = 12
	}

	x1 := 0
	x2 := scrW - 1

	y2 := ac.Edit.Y1 - 1
	y1 := y2 - h + 1

	if y1 < 0 {
		y1 = 0
		h = y2 - y1 + 1
	}

	ac.SetPosition(x1, y1, x2, y2)
}

func (ac *AutoCompleteMenu) Show(scr *ScreenBuf) {
	ac.Window.Show(scr)

	footer := " Up/Down Enter Esc Tab Shift+Del "
	p := NewPainter(scr)
	p.DrawTitle(ac.X1, ac.Y2, ac.X2, footer, Palette[ColDialogBoxTitle])

	if ac.Edit.curPos > len(ac.Edit.text) {
		ac.Edit.curPos = len(ac.Edit.text)
	}
	if ac.Edit.leftPos > ac.Edit.curPos {
		ac.Edit.leftPos = ac.Edit.curPos
	}

	if ac.IsFocused() {
		headText := string(ac.Edit.text[ac.Edit.leftPos:ac.Edit.curPos])
		vOffset := runewidth.StringWidth(headText)
		scr.SetCursorPos(ac.Edit.X1+vOffset, ac.Edit.Y1)
		scr.SetCursorVisible(true)
	}
}

func (ac *AutoCompleteMenu) ProcessKey(e *vtinput.InputEvent) bool {
	if e.Type == vtinput.FocusEventType {
		return ac.Window.ProcessKey(e)
	}
	if !e.KeyDown {
		return false
	}

	switch e.VirtualKeyCode {
	case vtinput.VK_UP, vtinput.VK_DOWN, vtinput.VK_PRIOR, vtinput.VK_NEXT:
		return ac.lb.ProcessKey(e)
	case vtinput.VK_ESCAPE:
		ac.Close()
		return true
	case vtinput.VK_TAB:
		if ac.lb.SelectPos >= 0 && ac.lb.SelectPos < len(ac.Matches) {
			ac.Edit.SetText(ac.Matches[ac.lb.SelectPos])
			ac.Edit.curPos = len(ac.Edit.text)
			ac.Edit.clearFlag = false
			ac.Edit.HistoryPos = -1
		}
		ac.Close()
		return true
	case vtinput.VK_RETURN:
		if ac.lb.SelectPos >= 0 && ac.lb.SelectPos < len(ac.Matches) {
			ac.Edit.SetText(ac.Matches[ac.lb.SelectPos])
			ac.Edit.curPos = len(ac.Edit.text)
			ac.Edit.clearFlag = false
			ac.Edit.HistoryPos = -1
		}
		ac.Close()
		if FrameManager != nil {
			// Only inject if not Shift+Enter (Shift+Enter just fills the line)
			if (e.ControlKeyState & vtinput.ShiftPressed) == 0 {
				FrameManager.InjectEvents([]*vtinput.InputEvent{e})
			}
		}
		return true
	case vtinput.VK_DELETE:
		if (e.ControlKeyState & vtinput.ShiftPressed) != 0 {
			if ac.lb.SelectPos >= 0 && ac.lb.SelectPos < len(ac.Matches) {
				itemToRemove := ac.Matches[ac.lb.SelectPos]
				newHist := []string{}
				for _, h := range ac.Edit.History {
					if h != itemToRemove {
						newHist = append(newHist, h)
					}
				}
				ac.Edit.History = newHist
				if ac.Edit.HistoryID != "" && GlobalHistoryProvider != nil {
					GlobalHistoryProvider.SaveHistory(ac.Edit.HistoryID, newHist)
				}
				ac.UpdateMatches()
				if !ac.HasMatches() {
					ac.Close()
				}
			}
			return true
		}
	}

	oldText := ac.Edit.GetText()
	handled := ac.Edit.ProcessKey(e)
	if handled {
		newText := ac.Edit.GetText()
		if newText != oldText {
			if newText == "" {
				ac.Close()
			} else {
				ac.UpdateMatches()
				if !ac.HasMatches() {
					ac.Close()
				}
			}
		}
	}
	return handled
}

func (ac *AutoCompleteMenu) ProcessMouse(e *vtinput.InputEvent) bool {
	if ac.lb.ProcessMouse(e) {
		return true
	}
	// Consume all mouse events within the menu bounds to prevent
	// the parent Window class from initiating a drag or resize operation.
	return true
}
