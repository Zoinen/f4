package panel

import (
	"fmt"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// translatorElement is a short-lived UIElement used for controls that are
// rendered by f4 but are not represented as a vtui child element. It gives
// the shared translator the same text/owner/help information as a regular
// ScreenObject without changing the live widget tree.
// zoin-bot: f4's panels and menu rows need this adapter because their visible
// contents are backed by application data rather than standalone widgets.
type translatorElement struct {
	vtui.ScreenObject
}

func newTranslatorElement(text string, owner vtui.CommandHandler, help string, x1, y1, x2, y2 int) vtui.UIElement {
	e := &translatorElement{}
	e.SetText(text)
	e.SetHelp(help)
	e.SetPosition(x1, y1, x2, y2)
	e.SetVisible(true)
	if owner != nil {
		e.SetOwner(owner)
	}
	return e
}

func IsTranslatorMouseEvent(e *vtinput.InputEvent) bool {
	if e == nil || e.Type != vtinput.MouseEventType || !e.KeyDown {
		return false
	}
	if e.ButtonState&vtinput.RightmostButtonPressed == 0 ||
		e.MouseEventFlags&vtinput.MouseMoved != 0 {
		return false
	}
	ctrl := e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0
	alt := e.ControlKeyState&(vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0
	return ctrl && alt
}

// HandleTranslatorMouseEvent is installed as an f4 event-filter fallback.
// vtui handles ordinary Window children before invoking EventFilter; this
// path fills the gap for f4's custom PanelsFrame, VMenu rows, and global menu
// bar, whose visible targets are not exposed as UIElement children.
func HandleTranslatorMouseEvent(e *vtinput.InputEvent) bool {
	if !IsTranslatorMouseEvent(e) {
		return false
	}
	target := translatorTargetAt(int(e.MouseX), int(e.MouseY))
	if target == nil {
		return false
	}

	terminal.SetF4Clipboard(FormatTranslatorReport(target))
	vtui.ShowToast("Translator info copied to clipboard", 3*time.Second)
	return true
}

func FormatTranslatorReport(target vtui.UIElement) string {
	text := ""
	if txtObj, ok := target.(interface{ GetText() string }); ok {
		text = txtObj.GetText()
	}

	key := ""
	if text != "" {
		key = vtui.ReverseLookup(text)
	}

	var contexts []string
	addContext := func(help string, prepend bool) {
		if help == "" {
			return
		}
		for _, existing := range contexts {
			if existing == help {
				return
			}
		}
		if prepend {
			contexts = append([]string{help}, contexts...)
		} else {
			contexts = append(contexts, help)
		}
	}
	addContext(target.GetHelp(), false)
	owner := target.GetOwner()
	for owner != nil {
		addContext(owner.GetHelp(), true)
		if obj, ok := owner.(interface{ GetOwner() vtui.CommandHandler }); ok {
			owner = obj.GetOwner()
		} else {
			break
		}
	}

	report := "--- f4 Translator Tool ---\n"
	if key != "" {
		report += fmt.Sprintf("Key:  %s\nText: %s\n", key, text)
	} else if text != "" {
		report += fmt.Sprintf("Key:  <HARDCODED>\nText: %s\n", text)
	} else {
		report += "Key:  <NO TEXT>\n"
	}

	context := "None"
	if len(contexts) > 0 {
		context = strings.Join(contexts, " -> ")
	}
	return report + fmt.Sprintf("Help Context: %s\n", context)
}

func translatorTargetAt(x, y int) vtui.UIElement {
	fm := vtui.FrameManager
	if fm == nil {
		return nil
	}

	frames := fm.GetActiveFrames(fm.ActiveIdx)
	if len(frames) == 0 {
		return nil
	}
	top := frames[len(frames)-1]

	// zoin-bot: match FrameManager's normal global-component priority. A
	// visible menu bar is painted over the frame below it and must win there.
	menuVisible := func(menu *vtui.MenuBar) bool {
		if menu == nil || !menu.IsVisible() {
			return false
		}
		return menu.Active || config.App.AlwaysShowMenuBar
	}
	if menu := fm.GetActiveMenuBar(); menuVisible(menu) && menu.HitTest(x, y) {
		canUseMenu := !top.IsModal() || top.GetType() == vtui.TypeMenu || top.GetMenuBar() == menu
		if canUseMenu {
			if target := TranslatorMenuBarTarget(menu, x, y); target != nil {
				return target
			}
		}
	}

	for i := len(frames) - 1; i >= 0; i-- {
		frame := frames[i]
		if !frame.HitTest(x, y) {
			continue
		}
		if target := TranslatorFrameTarget(frame, x, y); target != nil {
			return target
		}
		// The regular dispatcher stops at the first modal or hit frame even
		// when that frame declines the event. Do not inspect frames behind it.
		return nil
	}
	return nil
}

func TranslatorFrameTarget(frame vtui.Frame, x, y int) vtui.UIElement {
	if provider, ok := frame.(interface{ GetElementAt(x, y int) vtui.UIElement }); ok {
		if target := provider.GetElementAt(x, y); target != nil {
			return target
		}
	}

	switch frame := frame.(type) {
	case *vtui.VMenu:
		return translatorVMenuTarget(frame, x, y)
	case *PanelsFrame:
		return frame.TranslatorElementAt(x, y)
	default:
		return nil
	}
}

func translatorVMenuTarget(menu *vtui.VMenu, x, y int) vtui.UIElement {
	idx := menu.GetClickIndex(y)
	if idx < 0 || idx >= len(menu.Items) || menu.Items[idx].Separator {
		return nil
	}
	item := menu.Items[idx]
	x1, y1, x2, y2 := menu.GetPosition()
	rowY := menu.Y1 + menu.MarginTop + idx - menu.TopPos
	if rowY < y1 || rowY > y2 {
		return nil
	}
	return newTranslatorElement(item.Text, menu, "", x1+1, rowY, x2-1, rowY)
}

func TranslatorMenuBarTarget(menu *vtui.MenuBar, x, y int) vtui.UIElement {
	for i, item := range menu.Items {
		x1 := menu.GetItemX(i)
		x2 := x1
		if i < len(menu.Items)-1 {
			x2 = menu.GetItemX(i+1) - 1
		} else {
			clean, _, _ := vtui.ParseAmpersandString(item.Label)
			x2 += runewidth.StringWidth("  "+clean+"  ") - 1
		}
		if x >= x1 && x <= x2 && y >= menu.Y1 && y <= menu.Y2 {
			return newTranslatorElement(item.Label, menu, "", x1, menu.Y1, x2, menu.Y2)
		}
	}
	return nil
}

func (pf *PanelsFrame) TranslatorElementAt(x, y int) vtui.UIElement {
	if pf == nil {
		return nil
	}

	if pf.CmdLine != nil && pf.CmdLine.IsVisible() && pf.CmdLine.Edit != nil && pf.CmdLine.Edit.HitTest(x, y) {
		return newTranslatorElement(pf.CmdLine.Edit.GetText(), pf, "", pf.CmdLine.Edit.X1, pf.CmdLine.Edit.Y1, pf.CmdLine.Edit.X2, pf.CmdLine.Edit.Y2)
	}
	if !pf.ShowPanels {
		return nil
	}

	if i := pf.hitAltPanel(x, y); i >= 0 {
		x1, y1, x2, y2 := pf.AltPanels[i].GetPosition()
		return newTranslatorElement("", pf, "", x1, y1, x2, y2)
	}

	for i, panel := range pf.Panels {
		if panel == nil || (pf.Wide && i != pf.WidePanel) ||
			(!pf.Wide && i == 0 && !pf.ShowLeftPanel) ||
			(!pf.Wide && i == 1 && !pf.ShowRightPanel) {
			continue
		}
		x1, y1, x2, y2 := panel.GetPosition()
		if x < x1 || x > x2 || y < y1 || y > y2 {
			continue
		}
		if fsp, ok := panel.(*FileSystemPanel); ok {
			return fsp.TranslatorElementAt(x, y, pf)
		}
		return newTranslatorElement("", pf, "", x1, y1, x2, y2)
	}
	return nil
}

func (fp *FileSystemPanel) TranslatorElementAt(x, y int, owner vtui.CommandHandler) vtui.UIElement {
	if fp == nil {
		return nil
	}
	if fp.pathTitleHitTest(x, y) {
		return newTranslatorElement(fp.currentTitle, owner, "", fp.X1+1, fp.Y1, fp.X2-1, fp.Y1)
	}
	if fp.Table == nil || !fp.Table.HitTest(x, y) {
		return nil
	}

	if fp.Table.ShowHeader && y == fp.Table.Y1 {
		if column := panelTranslatorColumn(fp.Table, x); column >= 0 && column < len(fp.Table.Columns) {
			c := fp.Table.Columns[column]
			return newTranslatorElement(c.Title, owner, "", fp.Table.X1, y, fp.Table.X2, y)
		}
		return nil
	}

	rowOffset := y - (fp.Table.Y1 + fp.Table.MarginTop)
	if rowOffset < 0 || rowOffset >= fp.Table.ViewHeight {
		return nil
	}
	column := panelTranslatorColumn(fp.Table, x)
	if column < 0 {
		return nil
	}
	row := fp.Table.TopPos + rowOffset
	if fp.entryIndex(row, column) < 0 || fp.entryIndex(row, column) >= len(fp.Entries) {
		return nil
	}
	return newTranslatorElement(fp.GetCellText(row, column), owner, "", fp.Table.X1, y, fp.Table.X2, y)
}

func panelTranslatorColumn(table *vtui.Table, x int) int {
	if table == nil {
		return -1
	}
	columnX := table.X1
	for i, column := range table.Columns {
		width := column.Width
		if width <= 0 {
			return -1
		}
		if x >= columnX && x < columnX+width {
			return i
		}
		columnX += width
		if i < len(table.Columns)-1 {
			columnX++
		}
	}
	return -1
}
